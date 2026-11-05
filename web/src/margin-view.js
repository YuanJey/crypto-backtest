/**
 * Describes the archived margin result independently of current form values.
 */
const MarginView = {
  minimumMMR(report) {
    if (!report) return null;
    // A new explicit null is not a legacy value and must not become zero.
    if (Object.hasOwn(report, 'minimum_mmr')) {
      return Number.isFinite(report.minimum_mmr) ? report.minimum_mmr : null;
    }
    // Older archives stored min(E/N). The rate was fixed for the entire run,
    // so min(E/M) = min(E/N) / r. Use the archived rate, not the form value.
    const rate = report.maintenance_margin_rate;
    if (Number.isFinite(rate) && rate > 0 && Number.isFinite(report.minimum_margin_ratio)) {
      const value = report.minimum_margin_ratio / rate;
      return Number.isFinite(value) ? value : null;
    }
    return null;
  },
  pointMMR(point, report) {
    if (Object.hasOwn(point, 'mmr')) return Number.isFinite(point.mmr) ? point.mmr : null;
    if (point.position_qty === 0) return null;
    if (Number.isFinite(point.equity) && Number.isFinite(point.maintenance_margin) && point.maintenance_margin > 0) {
      const value = point.equity / point.maintenance_margin;
      return Number.isFinite(value) ? value : null;
    }
    const rate = report?.maintenance_margin_rate;
    if (Number.isFinite(rate) && rate > 0 && Number.isFinite(point.margin_ratio)) {
      const value = point.margin_ratio / rate;
      return Number.isFinite(value) ? value : null;
    }
    return null;
  },
  describe(report) {
    if (!report) return { label: '未检查', className: 'm-val',
      detail: '本次运行未启用维持保证金率强平检查。' };
    const rate = `${(report.maintenance_margin_rate * 100).toFixed(3)}%`;
    const minimumMMR = this.minimumMMR(report);
    const minimum = minimumMMR === null ? '无有效持仓数据' : `${(minimumMMR * 100).toFixed(3)}%`;
    const metric = `维持保证金率 ${rate}；最低 MMR（权益 / 维持保证金）${minimum}；MMR ≤ 100% 触发强平。`;
    if (!report.liquidated) return { label: '未爆仓', className: 'm-val m-success',
      detail: metric };
    const event = report.liquidations[0];
    const barTime = new Date(event.bar_open_time_ms).toISOString().slice(0, 19).replace('T', ' ');
    const when = event.phase === 'intrabar' ? `${barTime} UTC 起的 K 线内` :
      new Date(event.time_ms).toISOString().slice(0, 19).replace('T', ' ') + ' UTC';
    const side = event.side === 'long' ? '多头' : '空头';
    return { label: `已爆仓 · ${report.liquidation_count} 次`, className: 'm-val m-danger',
      detail: `${metric} ${when} ${side}强平，触发价 ${event.trigger_price.toFixed(4)}，成交价 ${event.fill_price.toFixed(4)}，强平后权益 ${event.cash_after.toFixed(2)}。本轮已停止交易。` };
  },
  render(report) {
    const description = this.describe(report);
    const badge = document.getElementById('bmLiquidation');
    badge.textContent = description.label;
    badge.className = description.className;
    const minimum = this.minimumMMR(report);
    const metric = document.getElementById('bmMinimumMMR');
    metric.textContent = minimum === null ? '--' : `${(minimum * 100).toFixed(3)}%`;
    metric.className = minimum === null ? 'm-val' : `m-val ${minimum <= 1 + 1e-10 ? 'm-danger' : 'm-success'}`;
    const detail = document.getElementById('btMarginDetails');
    detail.textContent = description.detail;
    detail.hidden = false;
  },
};
window.MarginView = MarginView;
