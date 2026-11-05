const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../src/strategy.js'), 'utf8');
const adapterSource = fs.readFileSync(path.join(__dirname, '../src/strategy-contract-adapter.js'), 'utf8');
const copy = value => JSON.parse(JSON.stringify(value));
const ref = (scope, name, field, offset) => ({
  kind: 'reference',
  ref: { scope, name, field, ...(offset === undefined ? {} : { offset }) },
});
const price = offset => ref('market', 'bar', 'close', offset);
const number = value => ({ kind: 'number', number: value });
const operator = (op, ...args) => ({ kind: 'operator', op, args });

class Element {
  constructor() {
    this.value = '';
    this.innerHTML = '';
    this.dataset = {};
    this.listeners = new Map();
    this.selectors = new Map();
    this.classList = { add() {}, remove() {} };
  }
  addEventListener(type, fn) { this.listeners.set(type, fn); }
  emit(type) { return this.listeners.get(type)?.({ target: this }); }
  querySelectorAll(selector) { return this.selectors.get(selector) || []; }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
  focus() {}
}

function workbench() {
  const elements = new Map();
  const document = {
    getElementById(id) {
      if (!elements.has(id)) elements.set(id, new Element());
      return elements.get(id);
    },
    querySelectorAll() { return []; },
  };
  const context = { window: {}, document };
  vm.createContext(context);
  vm.runInContext(adapterSource, context);
  for (const file of ['strategy-store.js', 'capability-catalog.js', 'strategy-editors.js']) {
    vm.runInContext(fs.readFileSync(path.join(__dirname, '../src', file), 'utf8'), context);
  }
  vm.runInContext(source, context);
  const wb = Object.create(context.window.StrategyWorkbench.prototype);
  wb.catalog = [
    { type: 'ema', outputs: [{ name: 'value', semantic: 'price_series', cross_group: 'moving_average' }] },
    { type: 'rsi', outputs: [{ name: 'value', semantic: 'oscillator_series', cross_group: 'rsi' }] },
    { type: 'boll', outputs: ['middle', 'upper', 'lower'].map(name => ({ name, semantic: 'price_series' })) },
    { type: 'supertrend', outputs: [
      { name: 'value', semantic: 'trend_level' },
      { name: 'direction', semantic: 'trend_direction' },
    ] },
  ];
  wb.currentStrategy = wb.getDefaultStrategyTemplate();
  wb.currentStrategy.indicators.push({ id: 'supertrend', type: 'supertrend' });
  wb.showAlert = () => {};
  wb.activeMode = 'visual';
  return { wb, document, context };
}

const conditions = {
  false: { kind: 'boolean', bool: false },
  true: { kind: 'boolean', bool: true },
  position: operator('lt', ref('position', 'current', 'entry_count', 0), number(3)),
  history: operator('gt', price(-2), number(100)),
  not: operator('not', operator('gt', price(), number(100))),
  nested: operator('and',
    operator('gt', price(), number(100)),
    operator('or',
      operator('lt', ref('position', 'current', 'unrealized_pnl_pct'), number(-0.02)),
      operator('gt', price(-1), number(105)),
    )),
  unknown: operator('future_operator', price(), number(100)),
  missingIndicator: operator('gt', ref('indicator', 'removed_indicator', 'value', -1), number(100)),
  sameSeriesHistory: operator('gt', price(), price(-1)),
  cross: operator('cross_up',
    ref('indicator', 'ema_fast_15m', 'value', -1),
    ref('indicator', 'ema_slow_15m', 'value', 0)),
  trend: operator('eq', ref('indicator', 'supertrend', 'direction', -1), number(-1)),
  trendTurn: operator('turn_bullish', ref('indicator', 'supertrend', 'direction', -1)),
  support: operator('lte', ref('indicator', 'supertrend', 'value', -1), price(-2)),
};

for (const [name, when] of Object.entries(conditions)) {
  test(`render and condition round trip preserve ${name}`, () => {
    const { wb } = workbench();
    wb.currentStrategy.rules = [{
      id: name, priority: 7, disabled: false, when: copy(when),
      action: { type: 'open', side: 'long', size_pct: 0.0123456789 },
    }];
    const before = copy(wb.currentStrategy.rules);
    wb.renderRulesEditor(wb.currentStrategy);
    wb.renderRulesEditor(wb.currentStrategy);
    assert.deepEqual(copy(wb.currentStrategy.rules), before);
    const parsed = wb.parseAstWhen(when);
    const rebuilt = wb.compileAstWhen(parsed.gate, parsed.items, parsed.sourceWhen);
    assert.deepEqual(copy(rebuilt), when);
  });
}

test('editing one condition preserves a nested sibling and both historical references', () => {
  const { wb, document } = workbench();
  const when = operator('and', operator('gt', price(-2), number(100)), conditions.nested);
  wb.currentStrategy.rules = [{ id: 'rule', when: copy(when), action: { type: 'open', side: 'long' } }];
  const input = new Element();
  input.dataset = { ridx: '0', cidx: '0' };
  input.value = '125.25';
  document.getElementById('rulesTreeContainer').selectors.set('.cond-right-num', [input]);
  wb.bindRulesEditorEvents(wb.currentStrategy);
  input.emit('input');
  const result = copy(wb.currentStrategy.rules[0].when);
  assert.deepEqual(result.args[0].args[0], when.args[0].args[0]);
  assert.equal(result.args[0].args[1].number, 125.25);
  assert.deepEqual(result.args[1], when.args[1]);
});

test('changing a group gate preserves unsupported descendants', () => {
  const { wb } = workbench();
  const parsed = wb.parseAstWhen(conditions.nested);
  const result = wb.compileAstWhen('or', parsed.items, parsed.sourceWhen);
  assert.equal(result.op, 'or');
  assert.deepEqual(copy(result.args), conditions.nested.args);
});

test('removing the final condition disables the condition rather than making it unconditional', () => {
  const { wb } = workbench();
  const parsed = wb.parseAstWhen(conditions.false);
  assert.deepEqual(copy(wb.compileAstWhen(parsed.gate, [], parsed.sourceWhen)), conditions.false);
});

test('condition strings remain untouched through visual rendering and saving', async () => {
  const { wb, context } = workbench();
  const rule = {
    id: 'chain', condition: 'and(position.flat(), ema_fast_15m.gt(100))',
    action: { type: 'enter', side: 'long', size_pct: 0.25 },
  };
  wb.currentStrategy.rules = [rule];
  const before = copy(rule);
  wb.renderRulesEditor(wb.currentStrategy);
  let payload;
  context.Api = { saveStrategy: async value => { payload = copy(value); return {}; } };
  wb.loadStrategies = async () => {};
  wb.selectStrategy = async () => {};
  await wb.handleSave();
  assert.deepEqual(payload.rules[0], before);
  assert.equal(Object.hasOwn(payload.rules[0], 'when'), false);
});

test('invalid JSON stays in the editor and does not replace the current strategy', () => {
  const { wb, document } = workbench();
  wb.activeMode = 'json';
  const current = wb.currentStrategy;
  document.getElementById('jsonEditorArea').value = '{"rules":';
  wb.switchMode('visual');
  assert.equal(wb.activeMode, 'json');
  assert.equal(wb.currentStrategy, current);
  assert.equal(document.getElementById('jsonEditorArea').value, '{"rules":');
});

test('visual and JSON switches preserve complex conditions', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.rules[0].when = copy(conditions.nested);
  wb.renderOverviewForm = () => {};
  wb.renderIndicatorsSection = () => {};
  const before = copy(wb.currentStrategy.rules);
  wb.switchMode('json');
  wb.switchMode('visual');
  wb.switchMode('json');
  assert.deepEqual(JSON.parse(document.getElementById('jsonEditorArea').value).rules, before);
});

test('default and newly added entry rules explicitly declare their own sizing', () => {
  const { wb } = workbench();
  assert.deepEqual(copy(wb.currentStrategy.position), {
    position_leverage: 1,
    long_capital_decay_power: 0.5,
    short_capital_decay_power: 0.5,
  });
  assert.deepEqual(copy(wb.currentStrategy.rules[0].action), { type: 'enter', side: 'long', size_pct: 0.25 });
  wb.addNewRule();
  assert.deepEqual(copy(wb.currentStrategy.rules.at(-1).action), { type: 'enter', side: 'long', size_pct: 0.25 });
});

test('legacy zero size preserves inheritance and full-close semantics', () => {
  const { wb } = workbench();
  assert.equal(wb.ruleSizeMode({ type: 'open', size_pct: 0 }), 'inherit');
  assert.equal(wb.ruleSizeMode({ type: 'close', size_pct: 0 }), 'all');
  assert.equal(wb.ruleSizeMode({ type: 'close', size_pct: 1 }), 'all');
  assert.equal(wb.ruleSizeMode({ type: 'close', size_pct: 0.25 }), 'custom');
});

test('switching to inheritance removes the override and can restore it explicitly', () => {
  const { wb } = workbench();
  const rule = { action: { type: 'open', side: 'long', size_pct: 0.0025 } };
  wb.setRuleSizeMode(rule, 'inherit');
  assert.equal(Object.hasOwn(rule.action, 'size_pct'), false);
  wb.setRuleSizeMode(rule, 'custom');
  assert.equal(rule.action.size_pct, 0.0025);
  wb.setRuleSizeMode(rule, 'inherit');
  rule.action.type = 'close';
  wb.setRuleSizeMode(rule, 'all');
  assert.equal(rule.action.size_pct, 1);
});

test('mode selection changes only sizing and preserves the entire condition', () => {
  const { wb, document } = workbench();
  const rule = { id: 'rule', when: copy(conditions.nested), action: { type: 'open', side: 'long', size_pct: 0.2 } };
  wb.currentStrategy.rules = [rule];
  const select = new Element();
  select.dataset.ridx = '0';
  select.value = 'inherit';
  document.getElementById('rulesTreeContainer').selectors.set('.rule-size-mode', [select]);
  wb.bindRulesEditorEvents(wb.currentStrategy);
  select.emit('change');
  assert.equal(Object.hasOwn(rule.action, 'size_pct'), false);
  assert.deepEqual(rule.when, conditions.nested);
});

test('custom sizing accepts tiny proportions and does not treat zero as an override', () => {
  const { wb, document } = workbench();
  const rule = { id: 'rule', when: copy(conditions.false), action: { type: 'open', side: 'long', size_pct: 0.2 } };
  wb.currentStrategy.rules = [rule];
  const input = new Element();
  input.dataset.ridx = '0';
  input.value = '0.000012345';
  document.getElementById('rulesTreeContainer').selectors.set('.rule-size-input', [input]);
  wb.bindRulesEditorEvents(wb.currentStrategy);
  input.emit('change');
  const size = rule.action.size_pct;
  assert.equal(size, 0.000012345 / 100);
  input.value = '0';
  input.emit('change');
  assert.equal(rule.action.size_pct, size);
  assert.deepEqual(rule.when, conditions.false);
});

test('preserved JSON previews escape markup', () => {
  const { wb } = workbench();
  const item = wb.parseConditionItem({ kind: 'future', text: '<script>run()</script>' });
  const html = wb.renderConditionRowHtml(item, 0, 0, wb.getAvailableOperands());
  assert.ok(html.includes('&lt;script&gt;'));
  assert.ok(!html.includes('<script>'));
});

function legacyStrategy(wb, position = {}, action = {}) {
  const def = wb.getDefaultStrategyTemplate();
  def.position = { position_leverage: 1, initial_size_pct: 0.0025, max_additions: 2, ...position };
  def.behavior.allow_pyramiding = true;
  delete def.behavior.allow_additions;
  def.rules = [{
    id: 'old', priority: 13, disabled: false, when: copy(conditions.nested),
    action: { type: 'open', side: 'long', ...action },
  }];
  return def;
}

function deepFreeze(value) {
  if (value && typeof value === 'object') {
    Object.values(value).forEach(deepFreeze);
    Object.freeze(value);
  }
  return value;
}

test('all visual sections read deeply frozen legacy definitions without migration', () => {
  const { wb } = workbench();
  const def = legacyStrategy(wb, { add_size_pct: 0.01, target_leverage: 3 });
  def.exit_policies = [{
    id: 'invalid', anchor: { type: 'avg_entry_price', indicator: 'retain_me' },
    distance: { type: 'percent', value: -1, indicator: 'retain_me' }, close_pct: 0,
  }, { id: 'missing', anchor: { type: 'indicator' }, distance: { type: 'atr_multiple' } }];
  const before = copy(def);
  wb.currentStrategy = deepFreeze(def);
  wb.renderVisualDetail(def);
  wb.renderVisualDetail(def);
  assert.deepEqual(copy(def), before);
  assert.equal(def.rules[0].action.type, 'open');
});

test('rendered defaults do not create fields until the user edits them', () => {
  const { wb, document } = workbench();
  wb.currentStrategy = { id: 'minimal' };
  wb.renderVisualDetail(wb.currentStrategy);
  assert.deepEqual(copy(wb.currentStrategy), { id: 'minimal' });
  assert.ok(document.getElementById('positionCards').innerHTML.includes('value="0.5"'));
  const input = document.getElementById('formLongCapitalDecayPower');
  input.value = '0.75';
  input.emit('change');
  assert.deepEqual(copy(wb.currentStrategy), { id: 'minimal', position: { long_capital_decay_power: 0.75 } });
});

test('loading keeps the fetched revision and the migrated draft independently owned', async () => {
  const { wb, context } = workbench();
  const source = deepFreeze(legacyStrategy(wb, { add_size_multiplier: 2 }));
  const before = copy(source);
  context.Api = { fetchStrategyById: async () => source };
  await wb.selectStrategy(source.id);
  assert.deepEqual(copy(source), before);
  assert.deepEqual(copy(wb.sourceStrategy), before);
  assert.notEqual(wb.sourceStrategy, source);
  assert.equal(wb.currentStrategy.rules[0].action.type, 'enter');
  wb.currentStrategy.rules[0].when.args[0].args[1].number = 999;
  assert.deepEqual(copy(wb.sourceStrategy), before);
  assert.deepEqual(copy(source), before);
});

test('adapter preparation is idempotent, preserves unknown fields, and shares no descendants', () => {
  const { wb, context } = workbench();
  const adapter = context.window.StrategyContractAdapter;
  const source = legacyStrategy(wb, { target_leverage: 3 });
  delete source.position.position_leverage;
  source.custom_extension = { values: [1, 2] };
  source.rules[0].custom_extension = { retained: true };
  const before = copy(source);
  const draft = adapter.prepareDraft(deepFreeze(source));
  assert.deepEqual(copy(adapter.prepareDraft(draft)), copy(draft));
  assert.deepEqual(copy(adapter.prepareSave(draft)), copy(draft));
  assert.equal(draft.position.position_leverage, 3);
  assert.equal(draft.rules[0].custom_extension.retained, true);
  draft.custom_extension.values[0] = 99;
  assert.deepEqual(copy(source), before);
});

test('explicit invalid scalar values remain visible for backend validation', () => {
  const { wb, context } = workbench();
  const source = wb.getDefaultStrategyTemplate();
  source.account.exchange_leverage = 5;
  source.position.position_leverage = -2;
  source.position.long_capital_decay_power = 0;
  source.position.short_capital_decay_power = null;
  source.behavior.allow_additions = null;
  source.exit_policies = [{ id: 'invalid', close_pct: 0, distance: { type: 'percent', value: -1 } }];
  const draft = context.window.StrategyContractAdapter.prepareDraft(source);
  assert.equal(draft.account.exchange_leverage, 5);
  assert.equal(draft.position.position_leverage, -2);
  assert.equal(draft.position.long_capital_decay_power, 0);
  assert.equal(draft.position.short_capital_decay_power, null);
  assert.equal(draft.behavior.allow_additions, null);
  const view = wb.normalizeExitPolicy(draft.exit_policies[0]);
  assert.equal(view.close_pct, 0);
  assert.equal(view.distance.value, -1);
});

test('JSON adapter failures retain the previous draft, source, editor and mode', () => {
  const { wb, document } = workbench();
  wb.sourceStrategy = copy(wb.currentStrategy);
  const source = wb.sourceStrategy;
  const draft = wb.currentStrategy;
  for (const invalid of [
    legacyStrategy(wb, { add_size_multiplier: 1e308 }),
    { rules: 'invalid' }, { exit_policies: [{ anchor: 'invalid' }] }, { account: null },
  ]) {
    wb.activeMode = 'json';
    const json = JSON.stringify(invalid);
    document.getElementById('jsonEditorArea').value = json;
    wb.switchMode('visual');
    assert.equal(wb.activeMode, 'json');
    assert.equal(wb.currentStrategy, draft);
    assert.equal(wb.sourceStrategy, source);
    assert.equal(document.getElementById('jsonEditorArea').value, json);
  }
});

test('a visual rendering failure rolls back the parsed JSON draft', () => {
  const { wb, document } = workbench();
  const current = wb.currentStrategy;
  wb.activeMode = 'json';
  const json = JSON.stringify({ id: 'unsupported' });
  document.getElementById('jsonEditorArea').value = json;
  wb.renderVisualDetail = () => { throw new Error('unsupported visual shape'); };
  wb.switchMode('visual');
  assert.equal(wb.currentStrategy, current);
  assert.equal(wb.activeMode, 'json');
  assert.equal(document.getElementById('jsonEditorArea').value, json);
});

test('an unconvertible fetched revision opens as JSON without losing its raw configuration', async () => {
  const { wb, document, context } = workbench();
  const source = legacyStrategy(wb, { add_size_multiplier: 1e308 });
  const before = copy(source);
  context.Api = { fetchStrategyById: async () => deepFreeze(source) };
  await wb.selectStrategy(source.id);
  assert.equal(wb.activeMode, 'json');
  assert.deepEqual(copy(wb.sourceStrategy), before);
  assert.deepEqual(JSON.parse(document.getElementById('jsonEditorArea').value), before);
});

test('a fetched revision with unsupported visual shape remains available in JSON', async () => {
  const { wb, context, document } = workbench();
  const source = wb.getDefaultStrategyTemplate();
  source.indicators = [{ id: 'broken' }];
  context.Api = { fetchStrategyById: async () => source };
  await wb.selectStrategy(source.id);
  assert.equal(wb.activeMode, 'json');
  assert.deepEqual(copy(wb.sourceStrategy), copy(source));
  assert.deepEqual(JSON.parse(document.getElementById('jsonEditorArea').value).indicators, source.indicators);
});

test('failed saves leave visual and JSON drafts, raw source and editor unchanged', async () => {
  for (const mode of ['visual', 'json']) {
    const { wb, context, document } = workbench();
    wb.currentStrategy = legacyStrategy(wb);
    wb.sourceStrategy = copy(wb.currentStrategy);
    wb.activeMode = mode;
    const draft = wb.currentStrategy;
    const source = wb.sourceStrategy;
    const before = copy(draft);
    const json = JSON.stringify(draft);
    document.getElementById('jsonEditorArea').value = json;
    context.Api = { saveStrategy: async payload => {
      assert.equal(payload.rules[0].action.type, 'enter');
      payload.rules[0].when.args.length = 0;
      throw new Error('backend rejected');
    } };
    await wb.handleSave();
    assert.equal(wb.currentStrategy, draft);
    assert.equal(wb.sourceStrategy, source);
    assert.deepEqual(copy(draft), before);
    assert.deepEqual(copy(source), before);
    assert.equal(document.getElementById('jsonEditorArea').value, json);
  }
});

test('editing an exit ID preserves missing references and invalid unrelated values', () => {
  const { wb, document } = workbench();
  const policy = { id: 'before', close_pct: 0, distance: { type: 'atr_multiple', value: -1 } };
  wb.currentStrategy.exit_policies = [policy];
  wb.currentStrategy.indicators.push({ id: 'atr', type: 'atr' });
  const field = new Element();
  field.dataset = { exitIdx: '0', exitField: 'id' };
  field.value = 'after';
  document.getElementById('exitPoliciesCards').selectors.set('.exit-policy-field', [field]);
  wb.renderExitPoliciesSection(wb.currentStrategy);
  assert.ok(document.getElementById('exitPoliciesCards').innerHTML.includes('请选择 ATR 指标'));
  field.emit('change');
  assert.deepEqual(copy(policy), { id: 'after', close_pct: 0, distance: { type: 'atr_multiple', value: -1 } });
});

test('explicit exit distance selection creates only its fields and removes incompatible ATR settings', () => {
  const { wb, document } = workbench();
  const policy = { id: 'minimal' };
  wb.currentStrategy.exit_policies = [policy];
  wb.currentStrategy.indicators.push({ id: 'atr', type: 'atr' });
  const field = new Element();
  field.dataset = { exitIdx: '0', exitField: 'distance_type' };
  field.value = 'atr_multiple';
  document.getElementById('exitPoliciesCards').selectors.set('.exit-policy-field', [field]);
  wb.renderExitPoliciesSection(wb.currentStrategy);
  field.emit('change');
  assert.deepEqual(copy(policy), { id: 'minimal', distance: {
    type: 'atr_multiple', value: 2, indicator: 'atr', field: 'value', sampling: 'live',
  } });
  field.value = 'fixed_amount';
  field.emit('change');
  assert.deepEqual(copy(policy), { id: 'minimal', distance: { type: 'fixed_amount', value: 100 } });
  field.dataset.exitField = 'purpose';
  field.value = 'take_profit';
  field.emit('change');
  assert.deepEqual(copy(policy), {
    id: 'minimal', distance: { type: 'fixed_amount', value: 100 },
    purpose: 'take_profit', trigger_price: 'high',
  });
});

test('legacy fixed sizing moves into each action and preserves the original condition and priority', () => {
  const { wb } = workbench();
  const original = legacyStrategy(wb, { add_size_pct: 0.01 });
  const before = copy(original);
  const def = wb.migrateRuleSizing(original);
  assert.deepEqual(copy(original), before);
  assert.deepEqual(copy(def.position), { position_leverage: 1 });
  assert.equal(def.behavior.allow_pyramiding, false);
  assert.equal(def.behavior.allow_additions, true);
  assert.deepEqual(copy(def.rules.map(rule => rule.action)), [
    { type: 'enter', side: 'long', size_pct: 0.0025 },
    { type: 'add', side: 'long', addition_index: 1, size_pct: 0.01 },
    { type: 'add', side: 'long', addition_index: 2, size_pct: 0.01 },
  ]);
  for (const rule of def.rules) {
    assert.equal(rule.priority, 13);
    assert.equal(rule.disabled, false);
    assert.equal(rule.source_rule_id, 'old');
    assert.deepEqual(copy(rule.when), conditions.nested);
  }
  const once = copy(def);
  assert.deepEqual(copy(wb.migrateRuleSizing(def)), once);
});

test('legacy explicit size overrides both first entry and addition modes', () => {
  const { wb } = workbench();
  const def = wb.migrateRuleSizing(legacyStrategy(wb, { add_size_multiplier: 2 }, { size_pct: 0.00345 }));
  assert.deepEqual(copy(def.rules.map(rule => rule.action.size_pct)), [0.00345, 0.00345, 0.00345]);
  assert.ok(def.rules.every(rule => !rule.action.size_multiplier));
});

test('legacy exponential sizing becomes a direct first-fill multiplier for each addition', () => {
  const { wb } = workbench();
  const def = wb.migrateRuleSizing(legacyStrategy(wb, { add_size_multiplier: 2 }, { size_pct: 0 }));
  assert.equal(def.rules[0].action.size_pct, 0.0025);
  assert.deepEqual(copy(def.rules.slice(1).map(rule => rule.action.size_multiplier)), [2, 4]);
});

test('legacy default additions use one times the actual first notional', () => {
  const { wb } = workbench();
  const def = wb.migrateRuleSizing(legacyStrategy(wb));
  assert.deepEqual(copy(def.rules.slice(1).map(rule => rule.action.size_multiplier)), [1, 1]);
});

test('legacy position pyramiding alias and chain conditions migrate without AST rewriting', () => {
  const { wb } = workbench();
  let def = legacyStrategy(wb, { allow_pyramiding: true });
  def.behavior.allow_pyramiding = false;
  delete def.rules[0].when;
  def.rules[0].condition = 'or(position.flat(), position.avg_entry_price.gte(market.close))';
  def.rules[0].disabled = true;
  def = wb.migrateRuleSizing(def);
  assert.equal(def.rules.length, 3);
  for (const rule of def.rules) {
    assert.equal(rule.condition, 'or(position.flat(), position.avg_entry_price.gte(market.close))');
    assert.equal(Object.hasOwn(rule, 'when'), false);
    assert.equal(rule.disabled, true);
  }
});

test('migration generates collision-free IDs and keeps provenance for legacy tie ordering', () => {
  const { wb } = workbench();
  let def = legacyStrategy(wb);
  const close = { id: 'old__add_1', when: copy(conditions.false), action: { type: 'close', side: 'long', size_pct: 1 } };
  def.rules.push(close);
  def = wb.migrateRuleSizing(def);
  assert.equal(new Set(def.rules.map(rule => rule.id)).size, 4);
  assert.equal(def.rules[1].id, 'old__add_1_2');
  assert.equal(def.rules[1].source_rule_id, 'old');
  assert.deepEqual(copy(def.rules.at(-1)), close);
});

test('invalid or overflowing legacy migration leaves the full definition untouched', () => {
  const { wb } = workbench();
  for (const position of [
    { max_additions: 1.5 }, { initial_size_pct: 0 }, { add_size_multiplier: 1e308 },
    { add_size_pct: 0.1, add_size_multiplier: 2 },
    { max_additions: null }, { add_size_pct: null }, { allow_pyramiding: 'true' },
  ]) {
    const def = legacyStrategy(wb, position);
    const before = copy(def);
    assert.throws(() => wb.migrateRuleSizing(def));
    assert.deepEqual(copy(def), before);
  }
});

test('overview separates leverage and capital decay from rule allocation', () => {
  const { wb, document } = workbench();
  wb.renderVisualDetail(wb.currentStrategy);
  const leverage = document.getElementById('leverageCards').innerHTML;
  assert.equal((leverage.match(/class="visual-card"/g) || []).length, 2);
  assert.ok(leverage.includes('formExchangeLeverage'));
  assert.ok(leverage.includes('formPositionLeverage'));
  const overview = document.getElementById('positionCards').innerHTML;
  assert.equal((overview.match(/class="visual-card"/g) || []).length, 2);
  assert.ok(overview.includes('formLongCapitalDecayPower'));
  assert.ok(overview.includes('formShortCapitalDecayPower'));
  assert.ok(!overview.includes('formInitialSizePct'));
  assert.ok(!overview.includes('formMaxAdditions'));
  assert.ok(!document.getElementById('rulesTreeContainer').innerHTML.includes('value="inherit"'));
});

test('maintenance-rate edits use percentage units and changing to spot clears the perpetual setting', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.account.market_type = 'perpetual';
  wb.sourceStrategy = copy(wb.currentStrategy);
  const source = copy(wb.sourceStrategy);
  wb.renderOverviewForm(wb.currentStrategy);
  assert.ok(document.getElementById('riskCards').innerHTML.includes('维持保证金率'));
  assert.ok(!document.getElementById('riskCards').innerHTML.includes('最低 MMR'));
  const field = document.getElementById('formMaintenanceMarginRate');
  field.value = '0.5';
  field.emit('change');
  assert.equal(wb.currentStrategy.account.maintenance_margin_rate, 0.005);
  assert.deepEqual(copy(wb.sourceStrategy), source);
  for (const value of ['100', '', 'Infinity', '-1']) {
    field.value = value;
    field.emit('change');
    assert.equal(wb.currentStrategy.account.maintenance_margin_rate, 0.005);
  }
  const market = document.getElementById('formMarketType');
  market.value = 'spot';
  market.emit('change');
  assert.equal(Object.hasOwn(wb.currentStrategy.account, 'maintenance_margin_rate'), false);
});

test('switching action types cleans incompatible fields and retains allocation and conditions', () => {
  const { wb } = workbench();
  const rule = wb.currentStrategy.rules[0];
  const when = copy(rule.when);
  rule.action.size_pct = 0.0025;
  wb.setRuleActionType(rule, 'add');
  assert.equal(rule.action.addition_index, 1);
  wb.setRuleSizeMode(rule, 'multiplier');
  rule.action.size_multiplier = 4;
  wb.setRuleSizeMode(rule, 'custom');
  assert.equal(rule.action.size_pct, 0.0025);
  assert.equal(Object.hasOwn(rule.action, 'size_multiplier'), false);
  wb.setRuleSizeMode(rule, 'multiplier');
  assert.equal(rule.action.size_multiplier, 4);
  wb.setRuleActionType(rule, 'close');
  assert.deepEqual(copy(rule.action), { type: 'close', side: 'long', size_pct: 1 });
  wb.setRuleActionType(rule, 'enter');
  assert.deepEqual(copy(rule.action), { type: 'enter', side: 'long', size_pct: 0.0025 });
  assert.deepEqual(copy(rule.when), when);
});

test('addition index and multiplier events update only the corresponding action', () => {
  const { wb, document } = workbench();
  const rule = { id: 'add', when: copy(conditions.history),
    action: { type: 'add', side: 'long', addition_index: 1, size_multiplier: 2 } };
  wb.currentStrategy.rules = [rule];
  wb.currentStrategy.behavior.allow_additions = true;
  const index = new Element();
  index.dataset.ridx = '0';
  index.value = '3';
  const multiplier = new Element();
  multiplier.dataset.ridx = '0';
  multiplier.value = '8';
  const container = document.getElementById('rulesTreeContainer');
  container.selectors.set('.rule-addition-index', [index]);
  container.selectors.set('.rule-multiplier-input', [multiplier]);
  wb.bindRulesEditorEvents(wb.currentStrategy);
  index.emit('change');
  multiplier.emit('change');
  assert.equal(rule.action.addition_index, 3);
  assert.equal(rule.action.size_multiplier, 8);
  index.value = '0';
  multiplier.value = '0';
  index.emit('change');
  multiplier.emit('change');
  assert.equal(rule.action.addition_index, 3);
  assert.equal(rule.action.size_multiplier, 8);
  assert.deepEqual(copy(rule.when), conditions.history);
});

test('saving legacy JSON sends explicit action sizing without global allocation fields', async () => {
  const { wb, document, context } = workbench();
  wb.activeMode = 'json';
  document.getElementById('jsonEditorArea').value = JSON.stringify(legacyStrategy(wb, { add_size_pct: 0.01 }));
  let payload;
  context.Api = { saveStrategy: async value => { payload = copy(value); return {}; } };
  wb.loadStrategies = async () => {};
  wb.selectStrategy = async () => {};
  await wb.handleSave();
  assert.deepEqual(payload.position, {
    position_leverage: 1,
    long_capital_decay_power: 0.5,
    short_capital_decay_power: 0.5,
  });
  assert.equal(payload.rules[0].action.type, 'enter');
  assert.equal(payload.rules[1].action.size_pct, 0.01);
  assert.deepEqual(payload.rules[2].when, conditions.nested);
});

test('allow additions switch enables configuration and preserves rules when disabled again', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.rules.push({
    id: 'add', when: copy(conditions.nested),
    action: { type: 'add', side: 'long', sizing_mode: 'current_position_multiple', addition_index: 1, size_multiplier: 2 },
  });
  wb.bindEvents();
  const before = copy(wb.currentStrategy.rules);
  const toggle = document.getElementById('formAllowAdditions');
  wb.renderRulesEditor(wb.currentStrategy);
  assert.equal(toggle.checked, false);
  let html = document.getElementById('rulesTreeContainer').innerHTML;
  assert.ok(html.includes('加仓已关闭'));
  assert.match(html, /rule-size-mode" data-ridx="2" disabled/);
  toggle.checked = true;
  toggle.emit('change');
  assert.equal(wb.currentStrategy.behavior.allow_additions, true);
  html = document.getElementById('rulesTreeContainer').innerHTML;
  assert.ok(html.includes('当前持仓倍增'));
  assert.ok(html.includes('首仓层级倍增'));
  assert.ok(!html.includes('加仓已关闭'));
  toggle.checked = false;
  toggle.emit('change');
  assert.equal(wb.currentStrategy.behavior.allow_additions, false);
  assert.deepEqual(copy(wb.currentStrategy.rules), before);
  const json = JSON.parse(document.getElementById('jsonEditorArea').value);
  assert.equal(json.behavior.allow_additions, false);
});

test('historical explicit add rules without the new switch remain enabled after migration', () => {
  const { wb } = workbench();
  let def = wb.getDefaultStrategyTemplate();
  delete def.behavior.allow_additions;
  def.rules.push({ id: 'add', when: copy(conditions.true),
    action: { type: 'add', side: 'long', addition_index: 1, size_multiplier: 4 } });
  const before = copy(def.rules);
  def = wb.migrateRuleSizing(def);
  assert.equal(def.behavior.allow_additions, true);
  assert.deepEqual(copy(def.rules), before);
  def = wb.migrateRuleSizing(def);
  assert.equal(def.rules.at(-1).action.sizing_mode, undefined);
});

test('new martingale modes keep their own factors and preserve rule conditions', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  const rule = { id: 'add', when: copy(conditions.nested),
    action: { type: 'add', side: 'long', addition_index: 2, size_multiplier: 4 } };
  wb.currentStrategy.rules = [rule];
  const select = new Element();
  select.dataset.ridx = '0';
  document.getElementById('rulesTreeContainer').selectors.set('.rule-size-mode', [select]);
  wb.bindRulesEditorEvents(wb.currentStrategy);
  select.value = 'current_position_multiple';
  select.emit('change');
  assert.equal(rule.action.sizing_mode, 'current_position_multiple');
  assert.equal(rule.action.size_multiplier, 2);
  rule.action.size_multiplier = 3;
  select.value = 'first_entry_geometric';
  select.emit('change');
  assert.equal(rule.action.sizing_mode, 'first_entry_geometric');
  assert.equal(rule.action.size_multiplier, 2);
  assert.ok(wb.ruleSizeHint(rule.action).includes('n = 本次加仓次数（2）'));
  select.value = 'multiplier';
  select.emit('change');
  assert.equal(rule.action.size_multiplier, 4);
  select.value = 'current_position_multiple';
  select.emit('change');
  assert.equal(rule.action.size_multiplier, 3);
  assert.deepEqual(copy(rule.when), conditions.nested);
  assert.equal(Object.hasOwn(rule.action, 'size_pct'), false);
});

test('disabled addition configuration cannot change sizing or addition count', () => {
  const { wb, document } = workbench();
  const rule = { id: 'add', when: copy(conditions.history),
    action: { type: 'add', side: 'long', addition_index: 1, sizing_mode: 'first_entry_geometric', size_multiplier: 2 } };
  wb.currentStrategy.rules = [rule];
  const before = copy(rule);
  const container = document.getElementById('rulesTreeContainer');
  for (const [selector, value] of [
    ['.rule-size-mode', 'current_position_multiple'],
    ['.rule-multiplier-input', '3'],
    ['.rule-addition-index', '2'],
  ]) {
    const input = new Element();
    input.dataset.ridx = '0';
    input.value = value;
    container.selectors.set(selector, [input]);
    wb.bindRulesEditorEvents(wb.currentStrategy);
    input.emit('change');
  }
  assert.deepEqual(copy(rule), before);
});

test('geometric input rejects numerical overflow without changing the action or condition', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  const rule = { id: 'add', when: copy(conditions.nested),
    action: { type: 'add', side: 'long', addition_index: 2, sizing_mode: 'first_entry_geometric', size_multiplier: 2 } };
  wb.currentStrategy.rules = [rule];
  const before = copy(rule);
  const multiplier = new Element();
  multiplier.dataset.ridx = '0';
  multiplier.value = '1e308';
  const index = new Element();
  index.dataset.ridx = '0';
  index.value = '2000';
  const container = document.getElementById('rulesTreeContainer');
  container.selectors.set('.rule-multiplier-input', [multiplier]);
  container.selectors.set('.rule-addition-index', [index]);
  wb.bindRulesEditorEvents(wb.currentStrategy);
  multiplier.emit('change');
  index.emit('change');
  assert.deepEqual(copy(rule), before);
});

test('saving new modes includes the master switch and selected formula', async () => {
  for (const mode of ['current_position_multiple', 'first_entry_geometric']) {
    const { wb, context } = workbench();
    wb.currentStrategy.behavior.allow_additions = true;
    wb.currentStrategy.rules.push({ id: 'add', when: copy(conditions.nested),
      action: { type: 'add', side: 'long', addition_index: 2, sizing_mode: mode, size_multiplier: 2 } });
    let payload;
    context.Api = { saveStrategy: async value => { payload = copy(value); return {}; } };
    wb.loadStrategies = async () => {};
    wb.selectStrategy = async () => {};
    await wb.handleSave();
    assert.equal(payload.behavior.allow_additions, true);
    assert.equal(payload.rules.at(-1).action.sizing_mode, mode);
    assert.equal(payload.rules.at(-1).action.size_multiplier, 2);
    assert.deepEqual(payload.rules.at(-1).when, conditions.nested);
  }
});

function changeAdditionField(wb, document, key, value, ridx = 0) {
  const field = new Element();
  field.dataset = { ridx: String(ridx), additionField: key };
  field.value = String(value);
  document.getElementById('rulesTreeContainer').selectors.set('.rule-addition-field', [field]);
  wb.bindRulesEditorEvents(wb.currentStrategy);
  field.emit('change');
}

test('dedicated addition button requires the switch and creates sequential price rules', () => {
  const { wb, document } = workbench();
  const count = wb.currentStrategy.rules.length;
  wb.addNewRule('add');
  assert.equal(wb.currentStrategy.rules.length, count);
  wb.currentStrategy.behavior.allow_additions = true;
  wb.addNewRule('add');
  wb.addNewRule('add');
  const rules = wb.currentStrategy.rules.slice(-2);
  assert.deepEqual(copy(rules.map(rule => rule.action.addition_index)), [1, 2]);
  assert.equal(rules[0].action.sizing_mode, 'current_position_multiple');
  assert.deepEqual(copy(rules[0].when), conditions.true);
  assert.equal(rules[0].addition_trigger.distance.value, 0.01);
  assert.equal(rules[0].addition_trigger.anchor.type, 'avg_entry_price');
  assert.ok(document.getElementById('rulesTreeContainer').innerHTML.includes('无附加限制'));
});

test('old add rules preserve custom conditions until price mode is selected', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  wb.currentStrategy.rules = [{
    id: 'add', when: copy(conditions.nested),
    action: { type: 'add', side: 'long', addition_index: 1, size_multiplier: 2 },
  }];
  wb.renderRulesEditor(wb.currentStrategy);
  const rule = wb.currentStrategy.rules[0];
  assert.equal(rule.addition_trigger, undefined);
  changeAdditionField(wb, document, 'mode', 'distance');
  assert.ok(rule.addition_trigger);
  assert.deepEqual(copy(rule.when), conditions.nested);
  changeAdditionField(wb, document, 'mode', 'condition');
  assert.equal(rule.addition_trigger, undefined);
  assert.deepEqual(copy(rule.when), conditions.nested);
});

test('percentage spacing uses human percentages and updates each rule independently', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  wb.currentStrategy.rules = [];
  wb.addNewRule('add');
  wb.addNewRule('add');
  const first = copy(wb.currentStrategy.rules[0]);
  changeAdditionField(wb, document, 'distance_value', 2, 1);
  changeAdditionField(wb, document, 'spacing_mode', 'linear', 1);
  changeAdditionField(wb, document, 'spacing_value', 0.5, 1);
  const second = wb.currentStrategy.rules[1];
  assert.equal(second.addition_trigger.distance.value, 0.02);
  assert.equal(second.addition_trigger.spacing.value, 0.005);
  assert.equal(wb.additionDistanceAt(second), 0.025);
  assert.ok(wb.additionTriggerHint(second).includes('第 2 次为 2.5%'));
  assert.deepEqual(copy(wb.currentStrategy.rules[0]), first);
  changeAdditionField(wb, document, 'spacing_mode', 'geometric', 1);
  changeAdditionField(wb, document, 'spacing_value', 2, 1);
  assert.equal(wb.additionDistanceAt(second), 0.04);
  changeAdditionField(wb, document, 'spacing_mode', 'fixed', 1);
  assert.equal(second.addition_trigger.spacing, undefined);
});

test('ATR trigger selects fixed entry, each fill and live sampling with geometric spacing', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  wb.currentStrategy.indicators.push({ id: 'atr_30m', type: 'atr', timeframe: '30m' });
  wb.currentStrategy.rules = [];
  wb.addNewRule('add');
  changeAdditionField(wb, document, 'distance_type', 'atr_multiple');
  const rule = wb.currentStrategy.rules[0];
  assert.deepEqual(copy(rule.addition_trigger.distance),
    { type: 'atr_multiple', value: 1, indicator: 'atr_30m', field: 'value', sampling: 'entry' });
  changeAdditionField(wb, document, 'distance_value', 2);
  changeAdditionField(wb, document, 'sampling', 'each_entry');
  assert.equal(rule.addition_trigger.distance.sampling, 'each_entry');
  changeAdditionField(wb, document, 'sampling', 'live');
  changeAdditionField(wb, document, 'spacing_mode', 'geometric');
  changeAdditionField(wb, document, 'spacing_value', 2);
  changeAdditionField(wb, document, 'anchor', 'last_entry_price');
  changeAdditionField(wb, document, 'direction', 'favorable');
  changeAdditionField(wb, document, 'trigger_price', 'high');
  assert.ok(wb.additionTriggerHint(rule).includes('最近一次开仓 / 加仓成交价 +'));
  assert.ok(document.getElementById('rulesTreeContainer').innerHTML.includes('首仓固定 ATR'));
  assert.equal(rule.addition_trigger.trigger_price, 'high');
  const before = copy(rule);
  wb.renderRulesEditor(wb.currentStrategy);
  assert.deepEqual(copy(rule), before);
});

test('distance type switches clear incompatible ATR fields and spacing units', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  wb.currentStrategy.rules = [];
  wb.addNewRule('add');
  changeAdditionField(wb, document, 'distance_type', 'atr_multiple');
  changeAdditionField(wb, document, 'spacing_mode', 'linear');
  changeAdditionField(wb, document, 'distance_type', 'fixed_amount');
  const rule = wb.currentStrategy.rules[0];
  assert.deepEqual(copy(rule.addition_trigger.distance), { type: 'fixed_amount', value: 1 });
  assert.equal(rule.addition_trigger.spacing, undefined);
  wb.setRuleActionType(rule, 'close');
  assert.equal(rule.addition_trigger, undefined);
});

test('invalid distance and excessive layer spacing retain the last valid configuration', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  wb.currentStrategy.rules = [];
  wb.addNewRule('add');
  const rule = wb.currentStrategy.rules[0];
  for (const value of ['', 0, -1, 100, 'Infinity']) {
    const before = copy(rule);
    changeAdditionField(wb, document, 'distance_value', value);
    assert.deepEqual(copy(rule), before);
  }
  changeAdditionField(wb, document, 'spacing_mode', 'geometric');
  changeAdditionField(wb, document, 'spacing_value', 2);
  const before = copy(rule);
  const index = new Element();
  index.dataset = { ridx: '0' };
  index.value = '100';
  document.getElementById('rulesTreeContainer').selectors.set('.rule-addition-index', [index]);
  wb.bindRulesEditorEvents(wb.currentStrategy);
  index.emit('change');
  assert.deepEqual(copy(rule), before);
});

test('paused additions preserve price settings and prevent edits', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  wb.currentStrategy.rules = [];
  wb.addNewRule('add');
  const before = copy(wb.currentStrategy.rules);
  wb.currentStrategy.behavior.allow_additions = false;
  wb.renderRulesEditor(wb.currentStrategy);
  assert.equal(document.getElementById('btnAddAdditionRule').disabled, true);
  changeAdditionField(wb, document, 'anchor', 'first_entry_price');
  assert.deepEqual(copy(wb.currentStrategy.rules), before);
});

test('adding an extra condition replaces the new rule unconditional expression', () => {
  const { wb, document } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  wb.currentStrategy.rules = [];
  wb.addNewRule('add');
  const button = new Element();
  button.dataset = { ridx: '0' };
  document.getElementById('rulesTreeContainer').selectors.set('.btn-add-condition', [button]);
  wb.bindRulesEditorEvents(wb.currentStrategy);
  button.emit('click');
  const when = wb.currentStrategy.rules[0].when;
  assert.equal(when.kind, 'operator');
  assert.equal(when.op, 'gt');
  assert.equal(when.args.some(arg => arg.kind === 'boolean'), false);
});

test('price triggers survive saving and JSON visual round trips with complex conditions', async () => {
  const { wb, document, context } = workbench();
  wb.currentStrategy.behavior.allow_additions = true;
  wb.currentStrategy.rules = [];
  wb.addNewRule('add');
  const rule = wb.currentStrategy.rules[0];
  rule.when = copy(conditions.nested);
  changeAdditionField(wb, document, 'spacing_mode', 'linear');
  const before = copy(rule);
  wb.switchMode('json');
  wb.switchMode('visual');
  assert.deepEqual(copy(wb.currentStrategy.rules[0]), before);
  let payload;
  context.Api = { saveStrategy: async value => { payload = copy(value); return {}; } };
  wb.loadStrategies = async () => {};
  wb.selectStrategy = async () => {};
  await wb.handleSave();
  assert.deepEqual(payload.rules[0], before);
});
