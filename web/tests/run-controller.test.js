const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const copy = value => JSON.parse(JSON.stringify(value));
function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
function environment() {
  const context = { window: {} };
  vm.createContext(context);
  for (const name of ['strategy-contract-adapter', 'strategy-store', 'capability-catalog', 'backtest-run-controller', 'backtest-run-panel']) {
    vm.runInContext(fs.readFileSync(path.join(__dirname, `../src/${name}.js`), 'utf8'), context);
  }
  return context;
}
function snapshot(id = 'A', revision = 3) {
  return {
    definition: { id, schema_version: 1, indicators: [], rules: [] },
    strategy_version: { revision, definition_sha256: 'a'.repeat(64) },
    default_run_config: {
      data: { dataset_id: '__sample__', symbols: ['BTC-USDT'], timeframe: '15m',
        start: '2026-10-01T00:00:00Z', end: '2026-10-02T00:00:00Z' },
      account: { initial_cash: 10000, market_type: 'perpetual', exchange_leverage: 10 },
      execution: { fee: { taker_rate: 0.0004 }, slippage: { type: 'none', value: 0 } },
      risk: { max_total_exposure_pct: 10, max_symbol_equity_pct: 10, max_drawdown_halt_pct: 0.2 },
    },
  };
}

test('out-of-order strategy fetches cannot overwrite store selection', async () => {
  const context = environment();
  const first = deferred(), second = deferred();
  const store = new context.window.StrategyStore({
    fetchStrategySnapshot: id => id === 'A' ? first.promise : second.promise,
  });
  const a = store.select('A'), b = store.select('B');
  second.resolve(snapshot('B'));
  await b;
  first.resolve(snapshot('A'));
  assert.equal(await a, null);
  assert.equal(store.draft.id, 'B');
  assert.equal(store.version.revision, 3);
  store.draft.id = 'edited';
  assert.equal(store.source.id, 'B');
});

test('creating a draft supersedes an in-flight saved selection', async () => {
  const context = environment(), fetch = deferred();
  const store = new context.window.StrategyStore({ fetchStrategySnapshot: () => fetch.promise });
  const pending = store.select('A');
  store.create(snapshot('new').definition);
  fetch.resolve(snapshot('A'));
  assert.equal(await pending, null);
  assert.equal(store.draft.id, 'new');
  assert.equal(store.version, null);
});

test('save owns submitted configuration while editing continues', async () => {
  const context = environment(), save = deferred();
  let submitted;
  const store = new context.window.StrategyStore({
    saveStrategy: value => { submitted = value; return save.promise; },
  });
  store.create(snapshot().definition);
  const pending = store.save(store.draft);
  store.draft.name = 'newer edit';
  assert.equal(submitted.name, undefined);
  save.resolve({});
  await pending;
  assert.equal(store.draft.name, 'newer edit');
});

test('run binding and request capture fix revision and own environment', async () => {
  const context = environment(), submit = deferred(), source = snapshot();
  let payload;
  const controller = new context.window.BacktestRunController({
    fetchStrategySnapshot: async () => source,
    startRun: value => { payload = value; return submit.promise; },
  });
  const selected = await controller.bindStrategy('A');
  source.strategy_version.revision = 4;
  const config = selected.default_run_config;
  config.account.initial_cash = 25000;
  const pending = controller.start(config, {});
  config.account.initial_cash = 50000;
  assert.equal(payload.strategy_revision, 3);
  assert.equal(payload.run_config.account.initial_cash, 25000);
  await assert.rejects(controller.start(config, {}), /尚未结束/);
  submit.resolve({ id: 'run', status: 'cancelled' });
  await pending;
  assert.equal(selected.definition.id, 'A');
});

test('stale submission after switching views remains archived without publishing', async () => {
  const context = environment(), submit = deferred();
  const published = [];
  const controller = new context.window.BacktestRunController({
    fetchStrategySnapshot: async id => snapshot(id),
    startRun: () => submit.promise,
  }, { onState: state => published.push(state.status) });
  await controller.bindStrategy('A');
  const pending = controller.start(snapshot().default_run_config, {});
  await controller.bindStrategy('B');
  submit.resolve({ id: 'old', status: 'running' });
  await pending;
  assert.equal(controller.record, null);
  assert.deepEqual(published, ['submitting']);
});

test('changing dataset while binding a revision does not strand the selection', async () => {
  const context = environment(), fetch = deferred();
  const controller = new context.window.BacktestRunController({ fetchStrategySnapshot: () => fetch.promise });
  const pending = controller.bindStrategy('A');
  controller.invalidate();
  fetch.resolve(snapshot('A'));
  await pending;
  assert.equal(controller.selection.definition.id, 'A');
});

test('latest run strategy selection wins out-of-order version responses', async () => {
  const context = environment(), first = deferred(), second = deferred();
  const controller = new context.window.BacktestRunController({
    fetchStrategySnapshot: id => id === 'A' ? first.promise : second.promise,
  });
  const a = controller.bindStrategy('A'), b = controller.bindStrategy('B');
  second.resolve(snapshot('B'));
  await b;
  first.resolve(snapshot('A'));
  assert.equal(await a, null);
  assert.equal(controller.selection.definition.id, 'B');
});

test('stale result fetch cannot overwrite another task or dataset view', async () => {
  const context = environment(), result = deferred();
  const published = [];
  const controller = new context.window.BacktestRunController({
    fetchRun: async () => ({ id: 'old', status: 'completed' }),
    fetchRunResult: () => result.promise,
    fetchRunSnapshot: async () => ({ data: { bars: ['old-data'] } }),
  }, { onResult: response => published.push(response) });
  await controller.open('old');
  controller.invalidate();
  result.resolve({ run_id: 'old' });
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(published, []);
});

test('cancellation stops scheduled polling and terminal result publication', async () => {
  const context = environment(), scheduled = [], states = [];
  let polls = 0;
  const controller = new context.window.BacktestRunController({
    fetchRun: async () => { polls++; return { id: 'run', status: 'running' }; },
    cancelRun: async () => ({ id: 'run', status: 'cancelled', progress: 10 }),
  }, { schedule: fn => scheduled.push(fn), onState: state => states.push(state.status) });
  await controller.open('run');
  await controller.cancel();
  await scheduled[0]();
  assert.equal(polls, 1);
  assert.deepEqual(states, ['running', 'cancelled']);
  assert.equal(controller.busy, false);
});

test('late cancellation of completed task still displays archived result once', async () => {
  const context = environment(), results = [];
  const controller = new context.window.BacktestRunController({
    fetchRun: async () => ({ id: 'run', status: 'running' }),
    cancelRun: async () => ({ id: 'run', status: 'completed', progress: 100 }),
    fetchRunResult: async () => ({ run_id: 'run' }),
    fetchRunSnapshot: async () => ({ data: { bars: [1] } }),
  }, { schedule: () => {}, onResult: (response, input) => results.push([response.run_id, input.data.bars]) });
  await controller.open('run');
  await controller.cancel();
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(copy(results), [['run', [1]]]);
});

test('disconnected polling preserves server task identity for reconnecting', async () => {
  const context = environment(), scheduled = [], states = [];
  const controller = new context.window.BacktestRunController({
    fetchRun: async () => {
      if (controller.record) throw new Error('offline');
      return { id: 'run', status: 'running' };
    },
  }, { schedule: fn => scheduled.push(fn), onState: value => states.push(value) });
  await controller.open('run');
  await scheduled[0]();
  assert.equal(states.at(-1).status, 'disconnected');
  assert.equal(controller.record.id, 'run');
});

test('run panel edits experiment values without modifying saved defaults', () => {
  const context = environment(), elements = new Map();
  context.document = {
    getElementById(id) {
      if (!elements.has(id)) elements.set(id, { value: '', closest: () => null });
      return elements.get(id);
    },
  };
  const panel = Object.create(context.window.BacktestRunPanel.prototype);
  panel.config = snapshot().default_run_config;
  const before = copy(panel.config);
  panel.getDataSelection = () => ({ datasetID: 'real', symbol: 'ETH-USDT' });
  panel.renderConfig();
  elements.get('btInitialCash').value = '25000';
  elements.get('btExposureLimit').value = '2';
  elements.get('btSymbolLimit').value = '1.5';
  elements.get('btDrawdownLimit').value = '0';
  elements.get('btMaintenanceMarginRate').value = '0.5';
  for (const element of elements.values()) element.value = String(element.value);
  const config = panel.readConfig();
  assert.equal(config.account.initial_cash, 25000);
  assert.equal(config.risk.max_total_exposure_pct, 2);
  assert.equal(config.risk.max_symbol_equity_pct, 1.5);
  assert.equal(config.risk.max_drawdown_halt_pct, 0);
  assert.equal(config.account.maintenance_margin_rate, 0.005);
  assert.equal(config.data.symbols[0], 'ETH-USDT');
  assert.deepEqual(copy(panel.config), before);
  elements.get('btInitialCash').value = '';
  assert.throws(() => panel.readConfig(), /无效/);
});

test('maintenance-rate controls preserve disabled legacy runs and reject invalid percentages', () => {
  const context = environment(), elements = new Map();
  context.document = { getElementById(id) {
    if (!elements.has(id)) elements.set(id, { value: '', closest: () => null });
    return elements.get(id);
  } };
  const panel = Object.create(context.window.BacktestRunPanel.prototype);
  panel.config = snapshot().default_run_config;
  panel.getDataSelection = () => ({ datasetID: '__sample__', symbol: 'BTC-USDT' });
  panel.renderConfig();
  for (const element of elements.values()) element.value = String(element.value);
  assert.equal(Object.hasOwn(panel.readConfig().account, 'maintenance_margin_rate'), false);
  for (const value of ['100', '-1', 'NaN', '']) {
    elements.get('btMaintenanceMarginRate').value = value;
    assert.throws(() => panel.readConfig(), /维持保证金率|无效/);
  }
  panel.config.account.market_type = 'spot';
  assert.equal(Object.hasOwn(panel.readConfig().account, 'maintenance_margin_rate'), false);
});

test('capability catalog owns metadata and escapes rendered labels', () => {
  const context = environment(), source = { indicators: [{ type: 'ema', category: '<unsafe>', params: [] }],
    capabilities: { archived_runs: true, position_modes: ['net'] } };
  const catalog = new context.window.CapabilityCatalog().accept(source);
  source.capabilities.archived_runs = false;
  assert.equal(catalog.supports('archived_runs'), true);
  assert.equal(catalog.supports('hedge'), false);
  const container = {};
  catalog.render(container);
  assert.ok(container.innerHTML.includes('&lt;unsafe&gt;'));
});
