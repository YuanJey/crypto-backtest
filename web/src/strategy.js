/**
 * Coordinates editor components, the draft store and catalog loading.
 */
class StrategyWorkbench extends StrategyEditors {
  constructor(options = {}) {
    super(options);
    this._store = new StrategyStore(Api);
    this.capabilityCatalog = new CapabilityCatalog();
    this.ready = this.init();
  }

  get store() {
    // Also supports independently instantiated editor hosts.
    if (!this._store) this._store = new StrategyStore(typeof Api === 'undefined' ? null : Api);
    this._store.api = typeof Api === 'undefined' ? this._store.api : Api;
    return this._store;
  }
  get currentStrategy() { return this.store.draft; }
  set currentStrategy(value) { this.store.draft = value; }
  get sourceStrategy() { return this.store.source; }
  set sourceStrategy(value) { this.store.source = value; }
  get strategies() { return this.store.list; }
  set strategies(value) { this.store.list = value; }
  get catalog() { return this.capabilityCatalog?.indicators || []; }
  set catalog(value) {
    if (!this.capabilityCatalog) this.capabilityCatalog = new CapabilityCatalog();
    this.capabilityCatalog.indicators = value;
  }

  async init() {
    this.renderSkeleton();
    await this.loadCatalog();
    await this.loadStrategies();
  }

  async loadCatalog() {
    try {
      const catalog = new CapabilityCatalog().accept(await Api.fetchCatalog());
      this.capabilityCatalog = catalog;
      catalog.render(document.getElementById('catalogListContainer'));
      const count = document.getElementById('catalogCount');
      if (count) count.textContent = `${catalog.indicators.length} 个`;
    } catch (err) {
      this.catalog = [];
      this.showAlert(`指标目录加载失败: ${err.message}`, 'error');
    }
  }

  async loadStrategies(selectFirst = true) {
    try {
      await this.store.refresh();
      this.renderStrategyList();
      if (selectFirst && this.strategies.length) await this.selectStrategy(this.strategies[0].id);
    } catch (err) {
      this.showAlert(`策略列表加载失败: ${err.message}`, 'error');
    }
  }

  async selectStrategy(id) {
    try {
      const selection = await this.store.select(id);
      if (!selection) return;
      document.querySelectorAll('.strategy-card').forEach(card =>
        card.classList.toggle('active', card.dataset.id === id));
      if (selection.conversionError) throw selection.conversionError;
      this.renderVisualDetail(selection.draft);
      this.syncToJson();
      if (this.store.version) {
        document.getElementById('viewStrategyId').textContent = `${id} · v${this.store.version.revision}`;
      }
    } catch (err) {
      if (this.currentStrategy?.id === id) {
        this.activeMode = 'json';
        this.switchMode('json');
      }
      this.showAlert(`配置无法展示或读取：${err.message}，请在 JSON 源码中修正。`, 'error');
    }
  }

  createNewStrategy() {
    const template = this.getDefaultStrategyTemplate(`strategy-${Date.now()}`);
    this.store.create(template);
    this.strategies.unshift({
      id: template.id, name: template.name, description: template.description,
      market_type: template.account.market_type, symbols: template.data.symbols,
      timeframe: template.data.timeframe, rule_count: template.rules.length,
    });
    this.renderStrategyList();
    this.renderVisualDetail(this.currentStrategy);
    this.syncToJson();
    this.showAlert(`已创建模板: ${template.name}`, 'info');
  }

  async handleSave() {
    const area = document.getElementById('jsonEditorArea');
    let content;
    try {
      content = this.activeMode === 'json' ? JSON.parse(area.value) : this.currentStrategy;
    } catch (err) {
      this.showAlert(`JSON 格式语法错误: ${err.message}`, 'error');
      return;
    }
    const draftBefore = JSON.stringify(this.currentStrategy);
    const jsonBefore = area?.value;
    const selectionBefore = this.store.selection;
    try {
      this.showAlert('正在向 Go 后端提交静态校验与持久化...', 'info');
      const { submitted } = await this.store.save(content);
      const unchanged = this.store.selection === selectionBefore
        && JSON.stringify(this.currentStrategy) === draftBefore && area?.value === jsonBefore;
      await this.loadStrategies(false);
      if (unchanged && this.store.selection === selectionBefore
          && JSON.stringify(this.currentStrategy) === draftBefore && area?.value === jsonBefore) {
        await this.selectStrategy(submitted.id);
      }
      this.showAlert(`策略 [${submitted.id}] 已保存${unchanged ? '' : '；当前草稿仍保留未提交的修改'}。`, 'success');
      if (window.App?.refreshStrategies) window.App.refreshStrategies();
    } catch (err) {
      this.showAlert(`校验失败: ${err.message}`, 'error');
    }
  }
}

window.StrategyWorkbench = StrategyWorkbench;
