/**
 * Validated catalog metadata shared by editors and the run UI.
 */
class CapabilityCatalog {
  constructor() {
    this.indicators = [];
    this.capabilities = {};
  }

  accept(data) {
    if (!Array.isArray(data.indicators)) throw new Error('指标目录响应无效');
    this.indicators = StrategyContractAdapter.clone(data.indicators);
    this.capabilities = StrategyContractAdapter.clone(data.capabilities || {});
    return this;
  }

  supports(name) {
    return this.capabilities[name] === true;
  }

  render(container) {
    if (!container) return;
    const escape = value => String(value).replace(/[&<>"']/g,
      character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character]);
    container.innerHTML = this.indicators.map(item => `
      <div class="catalog-badge-item" title="类型: ${escape(item.type)}">
        <div class="catalog-item-header">
          <span class="catalog-type">${escape(item.type.toUpperCase())}</span>
          <span class="catalog-category">${escape(item.category || 'indicator')}</span>
        </div>
        <div class="catalog-item-params">参数: ${escape((item.params || [])
          .map(param => `${param.name}(${param.type})`).join(', ') || '无')}</div>
      </div>`).join('');
  }
}

window.CapabilityCatalog = CapabilityCatalog;
