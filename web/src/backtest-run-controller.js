/**
 * Owns selection, fixed revision, submission and task monitoring.
 * Callbacks only publish responses belonging to the current view generation.
 */
class BacktestRunController {
  constructor(api, options = {}) {
    this.api = api;
    this.onState = options.onState || (() => {});
    this.onResult = options.onResult || (() => {});
    this.schedule = options.schedule || (callback => setTimeout(callback, 500));
    this.generation = 0;
    this.selectionGeneration = 0;
    this.selection = null;
    this.record = null;
    this.busy = false;
  }

  invalidate() {
    ++this.generation;
    this.record = null;
    this.busy = false;
  }

  async bindStrategy(id) {
    this.invalidate();
    this.selection = null;
    const ticket = ++this.selectionGeneration;
    if (!id) return null;
    const snapshot = await this.api.fetchStrategySnapshot(id);
    if (ticket !== this.selectionGeneration) return null;
    if (!snapshot.strategy_version?.revision || !snapshot.default_run_config) {
      throw new Error('策略缺少可运行的固定版本');
    }
    this.selection = StrategyContractAdapter.clone(snapshot);
    return StrategyContractAdapter.clone(snapshot);
  }

  async start(config, report) {
    if (this.busy) throw new Error('当前任务尚未结束');
    if (!this.selection) throw new Error('请先加载已保存的策略版本');
    const request = StrategyContractAdapter.clone({
      strategy_id: this.selection.definition.id,
      strategy_revision: this.selection.strategy_version.revision,
      run_config: config,
      report,
    });
    const ticket = ++this.generation;
    this.busy = true;
    this.onState({ status: 'submitting', progress: 0 });
    try {
      const record = await this.api.startRun(request);
      if (ticket === this.generation) this.watch(record, ticket);
      return record;
    } catch (err) {
      if (ticket === this.generation) {
        this.busy = false;
        this.onState({ status: 'failed', error: err.message });
      }
      throw err;
    }
  }

  watch(record, ticket) {
    if (ticket !== this.generation) return;
    this.record = record;
    this.busy = ['queued', 'running'].includes(record.status);
    this.onState(record);
    if (record.status === 'completed') {
      this.loadResult(record.id, ticket).catch(err => this.monitorError(err, ticket));
    } else if (this.busy) {
      this.schedule(async () => {
        if (ticket !== this.generation) return;
        try { this.watch(await this.api.fetchRun(record.id), ticket); }
        catch (err) { this.monitorError(err, ticket); }
      });
    }
  }

  monitorError(err, ticket) {
    if (ticket !== this.generation) return;
    // The server task keeps running. History can reconnect to it.
    this.busy = false;
    this.onState({ ...this.record, status: 'disconnected', error: err.message });
  }

  async loadResult(id, ticket) {
    const response = await this.api.fetchRunResult(id);
    const snapshot = await this.api.fetchRunSnapshot(id);
    if (ticket !== this.generation) return;
    this.onResult(response, snapshot);
  }

  async open(id) {
    this.invalidate();
    const ticket = this.generation;
    const record = await this.api.fetchRun(id);
    if (ticket === this.generation) this.watch(record, ticket);
    return record;
  }

  async replay(id) {
    if (this.busy) throw new Error('当前任务尚未结束');
    const ticket = ++this.generation;
    this.busy = true;
    this.onState({ status: 'submitting', progress: 0 });
    try {
      const record = await this.api.replayRun(id);
      if (ticket === this.generation) this.watch(record, ticket);
      return record;
    } catch (err) {
      if (ticket === this.generation) {
        this.busy = false;
        this.onState({ status: 'failed', error: err.message });
      }
      throw err;
    }
  }

  async cancel() {
    const id = this.record?.id;
    if (!id) return;
    const ticket = this.generation;
    const record = await this.api.cancelRun(id);
    if (ticket === this.generation) {
      // Stop the previous polling chain, including an in-flight result read.
      this.watch(record, ++this.generation);
    }
    return record;
  }
}

window.BacktestRunController = BacktestRunController;
