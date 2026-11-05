const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');
const context = { window: {} };
vm.createContext(context);
vm.runInContext(fs.readFileSync(path.join(__dirname, '../src/margin-view.js'), 'utf8'), context);
const view = context.window.MarginView;

test('archived untested runs are not presented as having survived liquidation', () => {
  assert.equal(view.describe(undefined).label, '未检查');
  const result = view.describe({ maintenance_margin_rate: 0.005, liquidated: false, minimum_margin_ratio: null });
  assert.equal(result.label, '未爆仓');
  assert.ok(result.detail.includes('无有效持仓数据'));
});

test('margin summary displays rate and clearly qualifies intrabar timing', () => {
  const result = view.describe({ maintenance_margin_rate: 0.005, liquidated: true, liquidation_count: 1,
    liquidations: [{ bar_open_time_ms: 1800000, time_ms: 2700000, phase: 'intrabar', side: 'short',
      trigger_price: 119.4, fill_price: 119.5, cash_after: 188.4 }] });
  assert.equal(result.label, '已爆仓 · 1 次');
  assert.ok(result.detail.includes('0.500%'));
  assert.ok(result.detail.includes('K 线内'));
  assert.ok(result.detail.includes('空头强平'));
  assert.ok(result.detail.includes('188.40'));
});

test('current report minimum MMR is authoritative and not divided by rate again', () => {
  const report = { maintenance_margin_rate: 0.005, minimum_mmr: 1.25, minimum_margin_ratio: 0.2, liquidated: false };
  assert.equal(view.minimumMMR(report), 1.25);
  assert.ok(view.describe(report).detail.includes('最低 MMR（权益 / 维持保证金）125.000%'));
  assert.ok(view.describe(report).detail.includes('维持保证金率 0.500%'));
});

test('legacy archive minimum is converted from E/N using its fixed saved rate', () => {
  const report = Object.freeze({ maintenance_margin_rate: 0.005, minimum_margin_ratio: 0.02, liquidated: false });
  const before = JSON.stringify(report);
  assert.equal(view.minimumMMR(report), 4);
  assert.ok(view.describe(report).detail.includes('400.000%'));
  assert.equal(JSON.stringify(report), before);
});

test('undefined MMR is not turned into zero or inferred over an explicit null', () => {
  assert.equal(view.minimumMMR({ maintenance_margin_rate: 0.005, minimum_mmr: null, minimum_margin_ratio: 0.2 }), null);
  assert.equal(view.minimumMMR({ maintenance_margin_rate: 0, minimum_margin_ratio: 0.2 }), null);
  assert.equal(view.minimumMMR({ minimum_margin_ratio: 0.2 }), null);
});

test('replay uses current MMR or actual archived maintenance margin', () => {
  assert.equal(view.pointMMR({ mmr: 1.5, equity: 250, maintenance_margin: 50 }, {}), 1.5);
  assert.equal(view.pointMMR({ equity: 250, maintenance_margin: 50, position_qty: 1 }, {}), 5);
  assert.equal(view.pointMMR({ margin_ratio: 0.01, position_qty: 1 }, { maintenance_margin_rate: 0.005 }), 2);
  assert.equal(view.pointMMR({ equity: 250, maintenance_margin: 50, position_qty: 0 }, {}), null);
  assert.equal(view.pointMMR({ mmr: null, equity: 250, maintenance_margin: 50 }, {}), null);
});

test('minimum MMR badge displays 100 percent at the liquidation boundary', () => {
  const elements = new Map();
  context.document = { getElementById(id) {
    if (!elements.has(id)) elements.set(id, {});
    return elements.get(id);
  } };
  view.render({ maintenance_margin_rate: 0.005, minimum_mmr: 1, liquidated: false });
  assert.equal(elements.get('bmMinimumMMR').textContent, '100.000%');
  assert.ok(elements.get('bmMinimumMMR').className.includes('m-danger'));
  view.render(undefined);
  assert.equal(elements.get('bmMinimumMMR').textContent, '--');
});
