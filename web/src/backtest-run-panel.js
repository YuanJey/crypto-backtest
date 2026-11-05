/**
 * Independent experiment controls and archived task view.
 */
class BacktestRunPanel {
  constructor(controller, options = {}) {
    this.controller = controller;
    this.onResult = options.onResult || (() => {});
    this.onReset = options.onReset || (() => {});
    this.getDataSelection = options.getDataSelection;
    this.config = null;
    this.binding = 0;
    this.historyGeneration = 0;
    this.runButton = document.getElementById('btnRunBacktest');
    this.cancelButton = document.getElementById('btnCancelBacktest');
    this.status = document.getElementById('btRunStatus');
    this.history = document.getElementById('btRunHistory');
    controller.onState = record => this.renderState(record);
    controller.onResult = (response, snapshot) => {
      this.onResult(response, snapshot);
      this.refreshHistory();
    };
    this.runButton.addEventListener('click', () => this.start());
    this.cancelButton.addEventListener('click', () => this.perform(() => controller.cancel()));
    document.getElementById('btnRefreshRuns').addEventListener('click', () => this.refreshHistory());
    document.getElementById('btnRefreshRunRevision').addEventListener('click', () => {
      this.bindStrategy(document.getElementById('btSelectStrategy').value);
    });
    this.refreshHistory();
  }

  async bindStrategy(id) {
    const ticket = ++this.binding;
    this.config = null;
    this.runButton.disabled = true;
    document.getElementById('btRunRevision').textContent = '加载策略版本…';
    try {
      const snapshot = await this.controller.bindStrategy(id);
      if (!snapshot || ticket !== this.binding) return;
      this.config = StrategyContractAdapter.clone(snapshot.default_run_config);
      this.renderConfig();
      this.applyDataTimes();
      document.getElementById('btRunRevision').textContent =
        `${id} · v${snapshot.strategy_version.revision} · ${snapshot.strategy_version.definition_sha256.slice(0, 12)}`;
      this.renderState({ status: 'ready' });
    } catch (err) {
      if (ticket === this.binding) {
        document.getElementById('btRunRevision').textContent = '版本加载失败';
        this.status.textContent = err.message;
      }
    }
  }

  renderConfig() {
    const config = this.config;
    for (const [id, value] of [
      ['btInitialCash', config.account.initial_cash],
      ['btTakerFee', config.execution.fee.taker_rate * 100],
      ['btSlippageBps', config.execution.slippage.type === 'bps' ? config.execution.slippage.value : 0],
      ['btExchangeLeverage', config.account.exchange_leverage],
      ['btExposureLimit', config.risk.max_total_exposure_pct],
      ['btSymbolLimit', config.risk.max_symbol_equity_pct],
      ['btDrawdownLimit', config.risk.max_drawdown_halt_pct * 100],
      ['btRunStart', config.data.start.slice(0, 16)],
      ['btRunEnd', config.data.end.slice(0, 16)],
    ]) document.getElementById(id).value = value;
    document.getElementById('btExchangeLeverage').disabled = config.account.market_type === 'spot';
  }

  updateDataSelection() {
    this.controller.invalidate();
    this.renderState({ status: 'ready' });
    this.applyDataTimes();
  }

  applyDataTimes() {
    const data = this.getDataSelection();
    if (data.startMS) document.getElementById('btRunStart').value = new Date(data.startMS).toISOString().slice(0, 16);
    if (data.endMS) document.getElementById('btRunEnd').value = new Date(data.endMS).toISOString().slice(0, 16);
  }

  readConfig() {
    if (!this.config) throw new Error('策略版本尚未就绪');
    const config = StrategyContractAdapter.clone(this.config);
    const read = (id, min, max = Infinity) => {
      const raw = document.getElementById(id).value;
      const value = Number(raw);
      if (raw.trim() === '' || !Number.isFinite(value) || value < min || value > max) {
        throw new Error(`${document.getElementById(id).closest('label')?.textContent.trim() || id} 参数无效`);
      }
      return value;
    };
    config.account.initial_cash = read('btInitialCash', Number.MIN_VALUE);
    config.account.exchange_leverage = read('btExchangeLeverage', 1);
    config.execution.fee.taker_rate = read('btTakerFee', 0, 100) / 100;
    const slippage = read('btSlippageBps', 0);
    config.execution.slippage = { type: slippage ? 'bps' : 'none', value: slippage };
    config.risk.max_total_exposure_pct = read('btExposureLimit', 0);
    config.risk.max_symbol_equity_pct = read('btSymbolLimit', 0);
    config.risk.max_drawdown_halt_pct = read('btDrawdownLimit', 0, 100) / 100;
    const data = this.getDataSelection();
    config.data.dataset_id = data.datasetID;
    config.data.symbols = [data.symbol];
    for (const [field, id] of [['start', 'btRunStart'], ['end', 'btRunEnd']]) {
      const value = document.getElementById(id).value;
      const time = new Date(value + (value.length === 16 ? ':00Z' : 'Z'));
      if (!Number.isFinite(time.getTime())) throw new Error('请选择有效的 UTC 回测区间');
      config.data[field] = time.toISOString();
    }
    if (config.data.start >= config.data.end) throw new Error('回测结束时间必须晚于开始时间');
    return config;
  }

  async start() {
    await this.perform(async () => {
      const config = this.readConfig();
      const indicators = this.controller.selection.definition.indicators || [];
      this.onReset();
      const record = await this.controller.start(config, {
        equity_curve: true, orders: true, fills: true, ledger: true,
        metrics: ['final_equity', 'total_return_pct', 'max_drawdown_pct', 'max_position_leverage', 'order_count', 'fill_count'],
        indicators: indicators.map(indicator => ({ id: indicator.id })),
      });
      this.refreshHistory();
      return record;
    });
  }

  async perform(action) {
    try { return await action(); }
    catch (err) { this.status.textContent = err.message; }
  }

  renderState(record) {
    const labels = {
      ready: '运行就绪', submitting: '预检并提交中', queued: '等待执行', running: '回测中',
      completed: '回测完成', cancelled: '已取消', failed: '执行失败', interrupted: '执行中断',
      disconnected: '连接中断，可从历史任务重新连接',
    };
    this.status.textContent = `${labels[record.status] || record.status}${record.id ? ` · ${record.id.slice(0, 16)} · ${record.progress}%` : ''}${record.error ? ` · ${record.error}` : ''}`;
    this.runButton.disabled = this.controller.busy || !this.config;
    this.cancelButton.disabled = !record.id || !['queued', 'running', 'disconnected'].includes(record.status);
    if (['completed', 'cancelled', 'failed', 'interrupted'].includes(record.status)) this.refreshHistory();
  }

  async refreshHistory() {
    const ticket = ++this.historyGeneration;
    try {
      const records = await this.controller.api.fetchRuns();
      if (ticket !== this.historyGeneration) return;
      this.history.replaceChildren();
      if (!records.length) this.history.textContent = '暂无运行记录';
      for (const record of records) {
        const row = document.createElement('div');
        row.className = 'run-history-row';
        const text = document.createElement('span');
        text.textContent = `${record.strategy_id} · v${record.strategy_revision} · ${record.status} ${record.progress}% · ${record.created_at.slice(0, 19)}`;
        row.appendChild(text);
        for (const [label, action] of [
          ['查看 / 连接', () => this.controller.open(record.id)],
          ['快照重跑', () => this.controller.replay(record.id)],
        ]) {
          const button = document.createElement('button');
          button.className = 'btn btn-outline btn-xs';
          button.textContent = label;
          button.addEventListener('click', () => this.perform(async () => {
            this.onReset();
            return await action();
          }));
          row.appendChild(button);
        }
        this.history.appendChild(row);
      }
    } catch (err) {
      if (ticket === this.historyGeneration) this.history.textContent = `运行记录读取失败：${err.message}`;
    }
  }
}

window.BacktestRunPanel = BacktestRunPanel;
