/**
 * Owns saved snapshots, editable drafts and selection races.
 * Rendering and backtest execution do not belong to this store.
 */
class StrategyStore {
  constructor(api) {
    this.api = api;
    this.list = [];
    this.source = null;
    this.draft = null;
    this.version = null;
    this.selection = 0;
  }

  async refresh() {
    this.list = await this.api.fetchStrategies();
    return this.list;
  }

  async select(id) {
    const ticket = ++this.selection;
    let snapshot;
    try {
      snapshot = this.api.fetchStrategySnapshot
        ? await this.api.fetchStrategySnapshot(id)
        : { definition: await this.api.fetchStrategyById(id) };
    } catch (err) {
      if (ticket !== this.selection) return null;
      throw err;
    }
    if (ticket !== this.selection) return null;
    if (!snapshot?.definition) throw new Error('策略不存在或无法读取');
    const source = StrategyContractAdapter.clone(snapshot.definition);
    let draft, conversionError;
    try {
      draft = StrategyContractAdapter.prepareDraft(source);
    } catch (err) {
      draft = StrategyContractAdapter.clone(source);
      conversionError = err;
    }
    this.source = source;
    this.draft = draft;
    this.version = StrategyContractAdapter.clone(snapshot.strategy_version || null);
    return { draft, conversionError };
  }

  create(definition) {
    ++this.selection;
    this.source = null;
    this.version = null;
    this.draft = StrategyContractAdapter.prepareDraft(definition);
  }

  async save(content) {
    // Own the submitted value even if the user continues editing during HTTP.
    const submitted = StrategyContractAdapter.prepareSave(content);
    const result = await this.api.saveStrategy(submitted);
    return { submitted, result };
  }
}

window.StrategyStore = StrategyStore;
