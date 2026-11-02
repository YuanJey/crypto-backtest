/**
 * QUANTIX - Strategy Management Controller & Visualizer
 */

class StrategyWorkbench {
  constructor(options = {}) {
    this.container = document.getElementById(options.containerId || 'view-strategies');
    this.onLoadIntoBacktest = options.onLoadIntoBacktest || null;

    this.catalog = [];
    this.strategies = [];
    this.currentStrategy = null;
    this.activeMode = 'visual'; // 'visual' | 'json'

    this.init();
  }

  async init() {
    this.renderSkeleton();
    await this.loadCatalog();
    await this.loadStrategies();
  }

  renderSkeleton() {
    this.container.innerHTML = `
      <div class="strategy-workbench">
        <!-- 左侧栏：策略列表与指标目录 -->
        <aside class="strategy-sidebar">
          <div class="sidebar-section-header">
            <span class="section-title">策略库 (STRATEGIES)</span>
            <button class="btn btn-primary-sm" id="btnNewStrategy">+ 新建策略</button>
          </div>
          <div class="strategy-list-container" id="strategyListContainer">
            <div class="loading-hint">正在加载策略列表...</div>
          </div>

          <div class="sidebar-section-header" style="margin-top: 16px;">
            <span class="section-title">指标能力目录 (CATALOG)</span>
            <span class="badge-subtle" id="catalogCount">--</span>
          </div>
          <div class="catalog-list-container" id="catalogListContainer">
            <div class="loading-hint">加载指标目录...</div>
          </div>
        </aside>

        <!-- 右侧主区：策略详情看板与编辑器 -->
        <main class="strategy-main">
          <div class="strategy-detail-header" id="detailHeader">
            <div class="strategy-meta-left">
              <div class="strategy-title-row">
                <h2 class="strategy-name" id="viewStrategyName">选择或新建策略</h2>
                <span class="strategy-id-badge" id="viewStrategyId">--</span>
                <span class="market-tag" id="viewMarketType">--</span>
              </div>
              <p class="strategy-desc" id="viewStrategyDesc">暂无选择</p>
            </div>
            <div class="strategy-actions">
              <div class="view-mode-toggle">
                <button class="btn-toggle active" data-mode="visual" id="btnModeVisual">📊 可视化构建器</button>
                <button class="btn-toggle" data-mode="json" id="btnModeJson">📝 JSON 源码</button>
              </div>
              <button class="btn btn-primary" id="btnSaveStrategy" title="向后端发送静态与语义校验并保存到策略库">
                <span class="btn-icon">💾</span> 校验并保存策略
              </button>
            </div>
          </div>

          <!-- 反馈横幅 -->
          <div class="strategy-alert-banner" id="strategyAlert" style="display: none;"></div>

          <!-- 内容容器 -->
          <div class="strategy-content-body">
            <!-- 视图 1：可视化看板 -->
            <div class="strategy-view-pane active" id="paneVisual">
              <!-- 概览栅格 -->
              <div class="visual-section">
                <h3 class="section-heading">账户与标的设置</h3>
                <div class="visual-grid-cards" id="overviewCards">
                  <!-- 动态插入 -->
                </div>
              </div>

              <!-- 指标配置卡片组 -->
              <div class="visual-section">
                <div class="section-heading-row">
                  <h3 class="section-heading">指标配置 (INDICATORS)</h3>
                  <span class="section-badge" id="indicatorCountBadge">0 个</span>
                  <button class="btn btn-primary-sm" id="btnAddIndicator" style="margin-left: auto;">+ 添加指标</button>
                </div>
                <div class="indicators-cards-grid" id="indicatorsCards">
                  <!-- 动态插入 -->
                </div>
              </div>

              <!-- 交易规则树 -->
              <div class="visual-section">
                <div class="section-heading-row">
                  <h3 class="section-heading">交易规则树 (RULES & ACTIONS)</h3>
                  <span class="section-badge" id="ruleCountBadge">0 组</span>
                  <button class="btn btn-primary-sm" id="btnAddRule" style="margin-left: auto;">+ 添加规则</button>
                </div>
                <div class="rules-tree-container" id="rulesTreeContainer">
                  <!-- 动态插入 -->
                </div>
              </div>
            </div>

            <!-- 视图 2：JSON 源码编辑器 -->
            <div class="strategy-view-pane" id="paneJson">
              <div class="json-editor-wrapper">
                <div class="editor-toolbar">
                  <span class="editor-tip">提示：严格遵循 Go 静态强类型规范，保存时将执行无未来函数与循环引用语义校验。</span>
                  <button class="btn-xs btn-outline" id="btnFormatJson">格式化 JSON</button>
                </div>
                <textarea class="json-code-textarea" id="jsonEditorArea" spellcheck="false"></textarea>
              </div>
            </div>
          </div>
        </main>
      </div>
    `;

    this.bindEvents();
  }

  bindEvents() {
    const btnNew = document.getElementById('btnNewStrategy');
    const btnModeVisual = document.getElementById('btnModeVisual');
    const btnModeJson = document.getElementById('btnModeJson');
    const btnSave = document.getElementById('btnSaveStrategy');
    const btnFormat = document.getElementById('btnFormatJson');
    const btnAddIndicator = document.getElementById('btnAddIndicator');
    const btnAddRule = document.getElementById('btnAddRule');

    btnNew.addEventListener('click', () => this.createNewStrategy());
    if (btnAddIndicator) btnAddIndicator.addEventListener('click', () => this.showIndicatorModal());
    if (btnAddRule) btnAddRule.addEventListener('click', () => this.addNewRule());

    btnModeVisual.addEventListener('click', () => this.switchMode('visual'));
    btnModeJson.addEventListener('click', () => this.switchMode('json'));

    btnSave.addEventListener('click', () => this.handleSave());

    btnFormat.addEventListener('click', () => {
      const area = document.getElementById('jsonEditorArea');
      try {
        const parsed = JSON.parse(area.value);
        area.value = JSON.stringify(parsed, null, 2);
      } catch (err) {
        this.showAlert(`JSON 格式无效: ${err.message}`, 'error');
      }
    });
  }

  switchMode(mode) {
    this.activeMode = mode;
    const btnVisual = document.getElementById('btnModeVisual');
    const btnJson = document.getElementById('btnModeJson');
    const paneVisual = document.getElementById('paneVisual');
    const paneJson = document.getElementById('paneJson');

    if (mode === 'visual') {
      btnVisual.classList.add('active');
      btnJson.classList.remove('active');
      paneVisual.classList.add('active');
      paneJson.classList.remove('active');

      // 从编辑器文本同步回对象
      const area = document.getElementById('jsonEditorArea');
      try {
        this.currentStrategy = JSON.parse(area.value);
        this.renderVisualDetail(this.currentStrategy);
      } catch {}
    } else {
      btnJson.classList.add('active');
      btnVisual.classList.remove('active');
      paneJson.classList.add('active');
      paneVisual.classList.remove('active');

      // 同步最新对象到编辑器
      const area = document.getElementById('jsonEditorArea');
      if (this.currentStrategy) {
        area.value = JSON.stringify(this.currentStrategy, null, 2);
      }
    }
  }

  async loadCatalog() {
    const data = await Api.fetchCatalog();
    this.catalog = data.indicators || [];
    const countEl = document.getElementById('catalogCount');
    if (countEl) countEl.textContent = `${this.catalog.length} 个`;

    const container = document.getElementById('catalogListContainer');
    if (!container) return;

    container.innerHTML = this.catalog.map(item => `
      <div class="catalog-badge-item" title="类型: ${item.type}">
        <div class="catalog-item-header">
          <span class="catalog-type">${item.type.toUpperCase()}</span>
          <span class="catalog-category">${item.category || 'indicator'}</span>
        </div>
        <div class="catalog-item-params">
          参数: ${(item.params || []).map(p => `${p.name}(${p.type})`).join(', ') || '无'}
        </div>
      </div>
    `).join('');
  }

  async loadStrategies() {
    let list = await Api.fetchStrategies();
    // 若数据库尚无策略，使用默认演示策略种子
    if (list.length === 0) {
      list = [
        {
          id: 'ema-boll-rsi',
          name: 'EMA + BOLL + RSI',
          description: '多周期趋势突破与布林带动量结合策略',
          market_type: 'perpetual',
          symbols: ['BTC-USDT'],
          timeframe: '15m',
          rule_count: 2,
          indicator_count: 4,
          updated_at: new Date().toISOString()
        }
      ];
    }
    this.strategies = list;
    this.renderStrategyList();

    // 默认选中第一个
    if (this.strategies.length > 0) {
      this.selectStrategy(this.strategies[0].id);
    }
  }

  renderStrategyList() {
    const container = document.getElementById('strategyListContainer');
    if (!container) return;

    container.innerHTML = this.strategies.map(s => `
      <div class="strategy-card ${this.currentStrategy && this.currentStrategy.id === s.id ? 'active' : ''}" data-id="${s.id}">
        <div class="card-header-row">
          <span class="card-name">${s.name || s.id}</span>
          <span class="market-tag-sm">${s.market_type || 'spot'}</span>
        </div>
        <p class="card-desc">${s.description || '无策略说明'}</p>
        <div class="card-footer-tags">
          <span class="tag-meta">${(s.symbols || ['BTC-USDT']).join(',')}</span>
          <span class="tag-meta">${s.timeframe || '15m'}</span>
          <span class="tag-meta-accent">${s.rule_count || 0} 规则</span>
        </div>
      </div>
    `).join('');

    container.querySelectorAll('.strategy-card').forEach(card => {
      card.addEventListener('click', () => {
        this.selectStrategy(card.dataset.id);
      });
    });
  }

  async selectStrategy(id) {
    let def = await Api.fetchStrategyById(id);
    if (!def) {
      def = this.getDefaultStrategyTemplate(id);
    }
    this.currentStrategy = def;

    // 刷新列表激活项
    document.querySelectorAll('.strategy-card').forEach(c => {
      c.classList.toggle('active', c.dataset.id === id);
    });

    // 渲染详情
    this.renderVisualDetail(def);

    // 填充 JSON
    const area = document.getElementById('jsonEditorArea');
    if (area) {
      area.value = JSON.stringify(def, null, 2);
    }
  }

  renderVisualDetail(def) {
    if (!def) return;
    this.renderOverviewForm(def);
    this.renderIndicatorsSection(def);
    this.renderRulesEditor(def);
  }

  syncToJson() {
    const area = document.getElementById('jsonEditorArea');
    if (area && this.currentStrategy) {
      area.value = JSON.stringify(this.currentStrategy, null, 2);
    }
  }

  renderOverviewForm(def) {
    document.getElementById('viewStrategyName').textContent = def.name || def.id;
    document.getElementById('viewStrategyId').textContent = def.id;
    document.getElementById('viewMarketType').textContent = (def.account && def.account.market_type) || 'spot';
    document.getElementById('viewStrategyDesc').textContent = def.description || '无策略描述说明';

    const overview = document.getElementById('overviewCards');
    const acc = def.account || {};
    const data = def.data || {};
    overview.innerHTML = `
      <div class="visual-card">
        <label class="vcard-label">策略显示名称</label>
        <input type="text" class="form-control" id="formStratName" value="${def.name || ''}" placeholder="策略名称">
      </div>
      <div class="visual-card">
        <label class="vcard-label">初始资金 (${acc.quote_currency || 'USDT'})</label>
        <input type="number" class="form-control" id="formInitialCash" value="${acc.initial_cash || 10000}" step="1000">
      </div>
      <div class="visual-card">
        <label class="vcard-label">市场类型</label>
        <select class="form-select" id="formMarketType">
          <option value="spot" ${acc.market_type === 'spot' ? 'selected' : ''}>现货 (Spot)</option>
          <option value="perpetual" ${acc.market_type === 'perpetual' ? 'selected' : ''}>永续合约 (Perpetual)</option>
        </select>
      </div>
      <div class="visual-card">
        <label class="vcard-label">交易标的 (逗号分隔)</label>
        <input type="text" class="form-control" id="formSymbols" value="${(data.symbols || ['BTC-USDT']).join(', ')}">
      </div>
    `;

    document.getElementById('formStratName').addEventListener('input', (e) => {
      def.name = e.target.value;
      document.getElementById('viewStrategyName').textContent = def.name;
      this.syncToJson();
    });
    document.getElementById('formInitialCash').addEventListener('change', (e) => {
      if (!def.account) def.account = {};
      def.account.initial_cash = parseFloat(e.target.value) || 10000;
      this.syncToJson();
    });
    document.getElementById('formMarketType').addEventListener('change', (e) => {
      if (!def.account) def.account = {};
      def.account.market_type = e.target.value;
      document.getElementById('viewMarketType').textContent = e.target.value;
      this.syncToJson();
    });
    document.getElementById('formSymbols').addEventListener('change', (e) => {
      if (!def.data) def.data = {};
      def.data.symbols = e.target.value.split(',').map(s => s.trim()).filter(Boolean);
      this.syncToJson();
    });
  }

  renderIndicatorsSection(def) {
    const indContainer = document.getElementById('indicatorsCards');
    const indicators = def.indicators || [];
    document.getElementById('indicatorCountBadge').textContent = `${indicators.length} 个`;

    indContainer.innerHTML = indicators.map((ind, i) => `
      <div class="indicator-visual-card">
        <div class="ind-card-header">
          <span class="ind-id">${ind.id}</span>
          <div style="display: flex; align-items: center; gap: 6px;">
            <span class="ind-type-badge">${ind.type.toUpperCase()} · ${ind.timeframe || '15m'}</span>
            <button class="btn-xs btn-outline btn-edit-ind" data-idx="${i}" title="编辑指标">✎</button>
            <button class="btn-xs btn-danger btn-del-ind" data-idx="${i}" title="删除指标">✕</button>
          </div>
        </div>
        <div class="ind-card-body">
          <div class="ind-kv">
            <span class="kv-k">参数:</span>
            <span class="kv-v">${JSON.stringify(ind.params || {})}</span>
          </div>
          <div class="ind-kv">
            <span class="kv-k">输入:</span>
            <span class="kv-v">${(ind.inputs || []).map(inp => `${inp.scope}.${inp.field}`).join(', ')}</span>
          </div>
        </div>
      </div>
    `).join('');

    indContainer.querySelectorAll('.btn-edit-ind').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const idx = parseInt(btn.dataset.idx, 10);
        this.showIndicatorModal(def.indicators[idx], idx);
      });
    });

    indContainer.querySelectorAll('.btn-del-ind').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const idx = parseInt(btn.dataset.idx, 10);
        const removed = def.indicators.splice(idx, 1)[0];
        this.renderVisualDetail(def);
        this.syncToJson();
        this.showAlert(`已移除指标: ${removed.id}`, 'info');
      });
    });
  }

  showIndicatorModal(existingInd = null, editIdx = -1) {
    const modalContainer = document.getElementById('modalContainer');
    const isEdit = existingInd !== null;
    const defaultType = existingInd ? existingInd.type : (this.catalog[0]?.type || 'ema');
    const defaultTf = existingInd ? (existingInd.timeframe || '15m') : '15m';
    const defaultId = existingInd ? existingInd.id : `${defaultType}_${Date.now().toString().slice(-4)}`;
    const defaultField = (existingInd && existingInd.inputs && existingInd.inputs[0]?.field) || 'close';
    const currentParams = (existingInd && existingInd.params) || { period: 20, stddev: 2.0 };

    modalContainer.innerHTML = `
      <div class="modal-backdrop" id="indModalBackdrop">
        <div class="modal-window">
          <div class="modal-header">
            <h4 class="modal-title">${isEdit ? '编辑技术指标' : '添加技术指标'}</h4>
            <button class="modal-close" id="btnCloseIndModal">✕</button>
          </div>
          <div class="modal-body">
            <div class="form-row">
              <div class="form-group">
                <label class="form-label">指标标识 (ID)</label>
                <input type="text" class="form-control" id="modalIndId" value="${defaultId}" placeholder="如 ema_fast_15m">
              </div>
              <div class="form-group">
                <label class="form-label">指标类型</label>
                <select class="form-select" id="modalIndType">
                  ${this.catalog.map(c => `<option value="${c.type}" ${c.type === defaultType ? 'selected' : ''}>${c.label || c.type.toUpperCase()}</option>`).join('')}
                </select>
              </div>
            </div>

            <div class="form-row">
              <div class="form-group">
                <label class="form-label">计算周期 (Timeframe)</label>
                <select class="form-select" id="modalIndTf">
                  <option value="15m" ${defaultTf === '15m' ? 'selected' : ''}>15m (基准)</option>
                  <option value="1h" ${defaultTf === '1h' ? 'selected' : ''}>1h (小时线)</option>
                  <option value="4h" ${defaultTf === '4h' ? 'selected' : ''}>4h (四小时线)</option>
                  <option value="1d" ${defaultTf === '1d' ? 'selected' : ''}>1d (日线)</option>
                </select>
              </div>
              <div class="form-group">
                <label class="form-label">输入价格字段</label>
                <select class="form-select" id="modalIndField">
                  <option value="close" ${defaultField === 'close' ? 'selected' : ''}>收盘价 (close)</option>
                  <option value="open" ${defaultField === 'open' ? 'selected' : ''}>开盘价 (open)</option>
                  <option value="high" ${defaultField === 'high' ? 'selected' : ''}>最高价 (high)</option>
                  <option value="low" ${defaultField === 'low' ? 'selected' : ''}>最低价 (low)</option>
                  <option value="volume" ${defaultField === 'volume' ? 'selected' : ''}>成交量 (volume)</option>
                </select>
              </div>
            </div>

            <div class="form-group" style="margin-top: 6px;">
              <label class="form-label">指标参数配置</label>
              <div id="modalParamInputs" class="form-row">
                <!-- 动态渲染参数输入项 -->
              </div>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-outline" id="btnCancelIndModal">取消</button>
            <button class="btn btn-primary" id="btnSaveIndModal">保存指标</button>
          </div>
        </div>
      </div>
    `;

    const renderParamInputs = (type) => {
      const pBox = document.getElementById('modalParamInputs');
      if (type === 'boll') {
        pBox.innerHTML = `
          <div class="form-group">
            <label class="form-label">计算周期 (Period)</label>
            <input type="number" class="form-control" id="paramValPeriod" value="${currentParams.period || 20}" min="1">
          </div>
          <div class="form-group">
            <label class="form-label">标准差倍数 (StdDev)</label>
            <input type="number" class="form-control" id="paramValStddev" value="${currentParams.stddev || 2.0}" step="0.1" min="0.1">
          </div>
        `;
      } else {
        pBox.innerHTML = `
          <div class="form-group" style="grid-column: span 2;">
            <label class="form-label">计算周期 (Period)</label>
            <input type="number" class="form-control" id="paramValPeriod" value="${currentParams.period || 14}" min="1">
          </div>
        `;
      }
    };

    renderParamInputs(defaultType);

    document.getElementById('modalIndType').addEventListener('change', (e) => {
      renderParamInputs(e.target.value);
    });

    const closeModal = () => {
      modalContainer.innerHTML = '';
    };

    document.getElementById('btnCloseIndModal').addEventListener('click', closeModal);
    document.getElementById('btnCancelIndModal').addEventListener('click', closeModal);
    document.getElementById('indModalBackdrop').addEventListener('click', (e) => {
      if (e.target.id === 'indModalBackdrop') closeModal();
    });

    document.getElementById('btnSaveIndModal').addEventListener('click', () => {
      const id = document.getElementById('modalIndId').value.trim();
      const type = document.getElementById('modalIndType').value;
      const tf = document.getElementById('modalIndTf').value;
      const field = document.getElementById('modalIndField').value;
      if (!id) {
        alert('请输入指标唯一标识');
        return;
      }

      let params = {};
      const periodEl = document.getElementById('paramValPeriod');
      if (periodEl) params.period = parseInt(periodEl.value, 10) || 14;
      const stddevEl = document.getElementById('paramValStddev');
      if (stddevEl) params.stddev = parseFloat(stddevEl.value) || 2.0;

      let inputs = [];
      if (type === 'atr') {
        inputs = [
          { scope: 'market', name: 'bar', field: 'high' },
          { scope: 'market', name: 'bar', field: 'low' },
          { scope: 'market', name: 'bar', field: 'close' }
        ];
      } else {
        inputs = [{ scope: 'market', name: 'bar', field: field }];
      }

      const indSpec = {
        id: id,
        type: type,
        timeframe: tf,
        inputs: inputs,
        params: params
      };

      if (!this.currentStrategy.indicators) this.currentStrategy.indicators = [];
      if (isEdit && editIdx >= 0) {
        this.currentStrategy.indicators[editIdx] = indSpec;
      } else {
        this.currentStrategy.indicators.push(indSpec);
      }

      closeModal();
      this.renderVisualDetail(this.currentStrategy);
      this.syncToJson();
      this.showAlert(`指标 [${id}] 已更新！`, 'success');
    });
  }

  getAvailableOperands() {
    const list = [
      { label: '行情: 收盘价 (close)', value: 'market:close' },
      { label: '行情: 开盘价 (open)', value: 'market:open' },
      { label: '行情: 最高价 (high)', value: 'market:high' },
      { label: '行情: 最低价 (low)', value: 'market:low' },
      { label: '行情: 成交量 (volume)', value: 'market:volume' },
    ];
    const indicators = (this.currentStrategy && this.currentStrategy.indicators) || [];
    for (const ind of indicators) {
      if (ind.type === 'boll') {
        list.push({ label: `BOLL上轨: ${ind.id}.upper`, value: `indicator:${ind.id}:upper` });
        list.push({ label: `BOLL中轨: ${ind.id}.middle`, value: `indicator:${ind.id}:middle` });
        list.push({ label: `BOLL下轨: ${ind.id}.lower`, value: `indicator:${ind.id}:lower` });
      } else {
        list.push({ label: `指标: ${ind.id}.value (${ind.type.toUpperCase()} ${ind.timeframe || '15m'})`, value: `indicator:${ind.id}:value` });
      }
    }
    return list;
  }

  renderRulesEditor(def) {
    const rulesContainer = document.getElementById('rulesTreeContainer');
    const rules = def.rules || [];
    document.getElementById('ruleCountBadge').textContent = `${rules.length} 组`;
    const operands = this.getAvailableOperands();

    rulesContainer.innerHTML = rules.map((rule, rIdx) => {
      const parsedWhen = this.parseAstWhen(rule.when);
      const action = rule.action || { type: 'open', side: 'long', size_pct: 0.25 };
      const sizePct = Math.round((action.size_pct || 0.25) * 100);

      return `
        <div class="rule-card-editor" data-ridx="${rIdx}">
          <div class="rule-top-bar">
            <div class="rule-top-inputs">
              <span class="panel-sub-label">规则ID:</span>
              <input type="text" class="form-control input-rule-id" value="${rule.id}" data-ridx="${rIdx}">
            </div>
            <button class="btn-xs btn-danger btn-del-rule" data-ridx="${rIdx}" title="删除此规则">✕ 删除规则</button>
          </div>

          <div class="rule-actions-panel">
            <span class="panel-sub-label">执行动作:</span>
            <div class="action-inline-group">
              <select class="form-select rule-action-type" data-ridx="${rIdx}">
                <option value="open" ${action.type === 'open' ? 'selected' : ''}>开仓 (OPEN)</option>
                <option value="close" ${action.type === 'close' ? 'selected' : ''}>平仓 (CLOSE)</option>
              </select>
              <select class="form-select rule-action-side" data-ridx="${rIdx}">
                <option value="long" ${action.side === 'long' ? 'selected' : ''}>多头 (LONG)</option>
                <option value="short" ${action.side === 'short' ? 'selected' : ''}>空头 (SHORT)</option>
              </select>
            </div>
            <div class="slider-wrapper">
              <span class="panel-sub-label">仓位:</span>
              <input type="range" class="pct-slider rule-size-slider" data-ridx="${rIdx}" min="5" max="100" step="5" value="${sizePct}">
              <span class="pct-val-display" id="sizeDisplay-${rIdx}">${sizePct}%</span>
            </div>
          </div>

          <div class="conditions-builder" data-ridx="${rIdx}">
            <div class="condition-tree-header">
              <span class="panel-sub-label">触发条件 (WHEN):</span>
              <div class="logic-gate-toggle">
                <button class="logic-btn ${parsedWhen.gate === 'and' ? 'active' : ''}" data-gate="and" data-ridx="${rIdx}">AND (全满足)</button>
                <button class="logic-btn ${parsedWhen.gate === 'or' ? 'active' : ''}" data-gate="or" data-ridx="${rIdx}">OR (任一满足)</button>
              </div>
            </div>

            <div class="condition-rows-list" id="condRows-${rIdx}">
              ${parsedWhen.items.map((item, cIdx) => this.renderConditionRowHtml(item, rIdx, cIdx, operands)).join('')}
            </div>

            <button class="btn-ctrl btn-add-condition" data-ridx="${rIdx}">＋ 添加条件项</button>
          </div>
        </div>
      `;
    }).join('');

    this.bindRulesEditorEvents(def);
  }

  renderConditionRowHtml(item, rIdx, cIdx, operands) {
    const isNum = item.rightType === 'number';
    return `
      <div class="condition-item-row" data-ridx="${rIdx}" data-cidx="${cIdx}">
        <select class="form-select cond-left-select" data-ridx="${rIdx}" data-cidx="${cIdx}">
          ${operands.map(op => `<option value="${op.value}" ${op.value === item.left ? 'selected' : ''}>${op.label}</option>`).join('')}
        </select>

        <select class="form-select cond-op-select" data-ridx="${rIdx}" data-cidx="${cIdx}">
          <option value="cross_up" ${item.op === 'cross_up' ? 'selected' : ''}>金叉 (cross_up)</option>
          <option value="cross_down" ${item.op === 'cross_down' ? 'selected' : ''}>死叉 (cross_down)</option>
          <option value="gt" ${item.op === 'gt' ? 'selected' : ''}>大于 (&gt;)</option>
          <option value="gte" ${item.op === 'gte' ? 'selected' : ''}>大于等于 (&gt;=)</option>
          <option value="lt" ${item.op === 'lt' ? 'selected' : ''}>小于 (&lt;)</option>
          <option value="lte" ${item.op === 'lte' ? 'selected' : ''}>小于等于 (&lt;=)</option>
          <option value="eq" ${item.op === 'eq' ? 'selected' : ''}>等于 (==)</option>
        </select>

        <div class="right-operand-group">
          ${isNum ? `
            <input type="number" step="0.5" class="form-control cond-right-num" data-ridx="${rIdx}" data-cidx="${cIdx}" value="${item.rightNumber}">
          ` : `
            <select class="form-select cond-right-select" data-ridx="${rIdx}" data-cidx="${cIdx}">
              ${operands.map(op => `<option value="${op.value}" ${op.value === item.rightIndicator ? 'selected' : ''}>${op.label}</option>`).join('')}
            </select>
          `}
          <button class="btn-toggle-operand" data-ridx="${rIdx}" data-cidx="${cIdx}" title="切换右操作数模式 (常数/指标)">
            ${isNum ? '常数' : '指标'}
          </button>
        </div>

        <button class="btn-delete-row" data-ridx="${rIdx}" data-cidx="${cIdx}" title="删除此条件">✕</button>
      </div>
    `;
  }

  bindRulesEditorEvents(def) {
    const container = document.getElementById('rulesTreeContainer');

    // 规则ID修改
    container.querySelectorAll('.input-rule-id').forEach(input => {
      input.addEventListener('change', (e) => {
        const rIdx = parseInt(e.target.dataset.ridx, 10);
        def.rules[rIdx].id = e.target.value.trim();
        this.syncToJson();
      });
    });

    // 动作类型修改
    container.querySelectorAll('.rule-action-type').forEach(select => {
      select.addEventListener('change', (e) => {
        const rIdx = parseInt(e.target.dataset.ridx, 10);
        def.rules[rIdx].action.type = e.target.value;
        this.syncToJson();
      });
    });

    // 多空方向修改
    container.querySelectorAll('.rule-action-side').forEach(select => {
      select.addEventListener('change', (e) => {
        const rIdx = parseInt(e.target.dataset.ridx, 10);
        def.rules[rIdx].action.side = e.target.value;
        this.syncToJson();
      });
    });

    // 仓位滑块
    container.querySelectorAll('.rule-size-slider').forEach(slider => {
      slider.addEventListener('input', (e) => {
        const rIdx = parseInt(e.target.dataset.ridx, 10);
        const val = parseInt(e.target.value, 10);
        document.getElementById(`sizeDisplay-${rIdx}`).textContent = `${val}%`;
        def.rules[rIdx].action.size_pct = val / 100;
        this.syncToJson();
      });
    });

    // 逻辑门切换 (AND / OR)
    container.querySelectorAll('.logic-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const rIdx = parseInt(btn.dataset.ridx, 10);
        const gate = btn.dataset.gate;
        const parsed = this.parseAstWhen(def.rules[rIdx].when);
        parsed.gate = gate;
        def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
        this.renderRulesEditor(def);
        this.syncToJson();
      });
    });

    // 左操作数修改
    container.querySelectorAll('.cond-left-select').forEach(sel => {
      sel.addEventListener('change', (e) => {
        const rIdx = parseInt(sel.dataset.ridx, 10);
        const cIdx = parseInt(sel.dataset.cidx, 10);
        const parsed = this.parseAstWhen(def.rules[rIdx].when);
        if (parsed.items[cIdx]) {
          parsed.items[cIdx].left = sel.value;
          def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
          this.syncToJson();
        }
      });
    });

    // 算子修改
    container.querySelectorAll('.cond-op-select').forEach(sel => {
      sel.addEventListener('change', (e) => {
        const rIdx = parseInt(sel.dataset.ridx, 10);
        const cIdx = parseInt(sel.dataset.cidx, 10);
        const parsed = this.parseAstWhen(def.rules[rIdx].when);
        if (parsed.items[cIdx]) {
          parsed.items[cIdx].op = sel.value;
          def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
          this.syncToJson();
        }
      });
    });

    // 右操作数选择
    container.querySelectorAll('.cond-right-select').forEach(sel => {
      sel.addEventListener('change', (e) => {
        const rIdx = parseInt(sel.dataset.ridx, 10);
        const cIdx = parseInt(sel.dataset.cidx, 10);
        const parsed = this.parseAstWhen(def.rules[rIdx].when);
        if (parsed.items[cIdx]) {
          parsed.items[cIdx].rightIndicator = sel.value;
          def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
          this.syncToJson();
        }
      });
    });

    // 右操作数数值
    container.querySelectorAll('.cond-right-num').forEach(input => {
      input.addEventListener('input', (e) => {
        const rIdx = parseInt(input.dataset.ridx, 10);
        const cIdx = parseInt(input.dataset.cidx, 10);
        const parsed = this.parseAstWhen(def.rules[rIdx].when);
        if (parsed.items[cIdx]) {
          parsed.items[cIdx].rightNumber = parseFloat(input.value) || 0;
          def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
          this.syncToJson();
        }
      });
    });

    // 切换常数 / 指标模式
    container.querySelectorAll('.btn-toggle-operand').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const rIdx = parseInt(btn.dataset.ridx, 10);
        const cIdx = parseInt(btn.dataset.cidx, 10);
        const parsed = this.parseAstWhen(def.rules[rIdx].when);
        if (parsed.items[cIdx]) {
          parsed.items[cIdx].rightType = parsed.items[cIdx].rightType === 'number' ? 'indicator' : 'number';
          def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
          this.renderRulesEditor(def);
          this.syncToJson();
        }
      });
    });

    // 删除条件项
    container.querySelectorAll('.btn-delete-row').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const rIdx = parseInt(btn.dataset.ridx, 10);
        const cIdx = parseInt(btn.dataset.cidx, 10);
        const parsed = this.parseAstWhen(def.rules[rIdx].when);
        parsed.items.splice(cIdx, 1);
        def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
        this.renderRulesEditor(def);
        this.syncToJson();
      });
    });

    // 添加条件项
    container.querySelectorAll('.btn-add-condition').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const rIdx = parseInt(btn.dataset.ridx, 10);
        const parsed = this.parseAstWhen(def.rules[rIdx].when);
        const operands = this.getAvailableOperands();
        parsed.items.push({
          left: operands[0].value,
          op: 'gt',
          rightType: 'number',
          rightIndicator: operands[0].value,
          rightNumber: 50.0
        });
        def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
        this.renderRulesEditor(def);
        this.syncToJson();
      });
    });

    // 删除规则
    container.querySelectorAll('.btn-del-rule').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const rIdx = parseInt(btn.dataset.ridx, 10);
        const removed = def.rules.splice(rIdx, 1)[0];
        this.renderRulesEditor(def);
        this.syncToJson();
        this.showAlert(`已删除规则: ${removed.id}`, 'info');
      });
    });
  }

  addNewRule() {
    if (!this.currentStrategy.rules) this.currentStrategy.rules = [];
    const id = `rule_${this.currentStrategy.rules.length + 1}`;
    const operands = this.getAvailableOperands();
    const newRule = {
      id: id,
      when: {
        kind: 'operator',
        op: 'cross_up',
        args: [
          this.operandValueToNode(operands[0]?.value || 'market:close'),
          this.operandValueToNode(operands[1]?.value || 'market:open')
        ]
      },
      action: {
        type: 'open',
        side: 'long',
        size_pct: 0.25
      }
    };
    this.currentStrategy.rules.push(newRule);
    this.renderRulesEditor(this.currentStrategy);
    this.syncToJson();
    this.showAlert(`已新增规则: ${id}`, 'info');
  }

  parseAstWhen(when) {
    if (!when) return { gate: 'and', items: [] };
    if (when.kind === 'operator') {
      if (when.op === 'and' || when.op === 'or') {
        const items = (when.args || []).map(arg => this.parseConditionItem(arg)).filter(Boolean);
        return { gate: when.op, items: items };
      }
      const item = this.parseConditionItem(when);
      return { gate: 'and', items: item ? [item] : [] };
    }
    return { gate: 'and', items: [] };
  }

  parseConditionItem(node) {
    if (!node || node.kind !== 'operator') return null;
    const op = node.op;
    const leftNode = node.args && node.args[0];
    const rightNode = node.args && node.args[1];

    const leftVal = this.nodeToOperandValue(leftNode);
    let rightType = 'indicator';
    let rightVal = '';
    let rightNum = 50.0;

    if (rightNode && rightNode.kind === 'number') {
      rightType = 'number';
      rightNum = rightNode.number ?? 0;
    } else if (rightNode) {
      rightVal = this.nodeToOperandValue(rightNode);
    }

    return {
      left: leftVal || 'market:close',
      op: op || 'gt',
      rightType: rightType,
      rightIndicator: rightVal || 'market:close',
      rightNumber: rightNum,
    };
  }

  nodeToOperandValue(node) {
    if (!node) return '';
    if (node.kind === 'reference' && node.ref) {
      if (node.ref.scope === 'market') return `market:${node.ref.field || 'close'}`;
      return `indicator:${node.ref.id}:${node.ref.field || 'value'}`;
    }
    return '';
  }

  compileConditionItem(item) {
    const leftArg = this.operandValueToNode(item.left);
    let rightArg;
    if (item.rightType === 'number') {
      rightArg = { kind: 'number', number: parseFloat(item.rightNumber) || 0 };
    } else {
      rightArg = this.operandValueToNode(item.rightIndicator);
    }
    return {
      kind: 'operator',
      op: item.op,
      args: [leftArg, rightArg]
    };
  }

  operandValueToNode(str) {
    if (!str || str.startsWith('market:')) {
      const field = (str && str.split(':')[1]) || 'close';
      return { kind: 'reference', ref: { scope: 'market', name: 'bar', field: field } };
    }
    const parts = str.split(':');
    return {
      kind: 'reference',
      ref: {
        scope: 'indicator',
        id: parts[1],
        field: parts[2] || 'value',
        offset: 0
      }
    };
  }

  compileAstWhen(gate, items) {
    if (!items || items.length === 0) {
      return { kind: 'boolean', bool: true };
    }
    if (items.length === 1) {
      return this.compileConditionItem(items[0]);
    }
    return {
      kind: 'operator',
      op: gate || 'and',
      args: items.map(item => this.compileConditionItem(item))
    };
  }

  createNewStrategy() {
    const id = `strategy-${Date.now().toString().slice(-4)}`;
    const template = this.getDefaultStrategyTemplate(id);
    this.strategies.unshift({
      id: template.id,
      name: template.name,
      description: template.description,
      market_type: template.account.market_type,
      symbols: template.data.symbols,
      timeframe: template.data.timeframe,
      rule_count: template.rules.length,
      indicator_count: template.indicators.length,
      updated_at: new Date().toISOString()
    });
    this.renderStrategyList();
    this.selectStrategy(template.id);
    this.showAlert(`已创建空白模板: ${template.name}`, 'info');
  }

  async handleSave() {
    let content;
    const area = document.getElementById('jsonEditorArea');
    if (this.activeMode === 'json') {
      try {
        content = JSON.parse(area.value);
      } catch (err) {
        this.showAlert(`JSON 格式语法错误: ${err.message}`, 'error');
        return;
      }
    } else {
      content = this.currentStrategy;
    }

    try {
      this.showAlert('正在向 Go 后端提交静态校验与持久化...', 'info');
      const res = await Api.saveStrategy(content);
      this.showAlert(`策略 [${content.id}] 校验通过并保存成功！`, 'success');

      // 刷新列表并通知行情回测页刷新策略下拉框
      await this.loadStrategies();
      this.selectStrategy(content.id);
      if (window.App && typeof window.App.refreshStrategies === 'function') {
        window.App.refreshStrategies();
      }
    } catch (err) {
      this.showAlert(`校验失败: ${err.message}`, 'error');
    }
  }

  showAlert(msg, type = 'info') {
    const el = document.getElementById('strategyAlert');
    if (!el) return;
    el.textContent = msg;
    el.className = `strategy-alert-banner alert-${type}`;
    el.style.display = 'block';

    if (type === 'success' || type === 'info') {
      setTimeout(() => {
        el.style.display = 'none';
      }, 4000);
    }
  }

  getDefaultStrategyTemplate(id = 'ema-boll-rsi') {
    return {
      schema_version: 1,
      id: id,
      name: id === 'ema-boll-rsi' ? 'EMA + BOLL + RSI' : `策略 ${id}`,
      description: '多周期趋势突破与布林带/RSI动量结合策略',
      data: {
        dataset_id: 'binance-spot-15m',
        symbols: ['BTC-USDT'],
        timeframe: '15m',
        start: '2025-01-01T00:00:00Z',
        end: '2025-02-01T00:00:00Z'
      },
      account: {
        market_type: 'spot',
        initial_cash: 10000,
        quote_currency: 'USDT'
      },
      behavior: {
        allow_pyramiding: false,
        allow_same_bar_reversal: false,
        cooldown_bars: 0
      },
      position: {
        initial_size_pct: 0.25,
        allow_pyramiding: false,
        max_additions: 0,
        add_size_multiplier: 1,
        max_position_pct: 1
      },
      execution: {
        fill_mode: 'next_open',
        fee: { maker_rate: 0.0002, taker_rate: 0.0005 },
        slippage: { type: 'fixed', value: 2 }
      },
      risk: {
        max_symbol_equity_pct: 1,
        max_total_exposure_pct: 1,
        max_open_positions: 1,
        max_drawdown_halt_pct: 0.2
      },
      indicators: [
        {
          id: 'ema_fast_15m',
          type: 'ema',
          timeframe: '15m',
          inputs: [{ scope: 'market', name: 'bar', field: 'close' }],
          params: { period: 20 }
        },
        {
          id: 'ema_slow_15m',
          type: 'ema',
          timeframe: '15m',
          inputs: [{ scope: 'market', name: 'bar', field: 'close' }],
          params: { period: 60 }
        },
        {
          id: 'rsi_14_4h',
          type: 'rsi',
          timeframe: '4h',
          inputs: [{ scope: 'market', name: 'bar', field: 'close' }],
          params: { period: 14 }
        },
        {
          id: 'boll_20_15m',
          type: 'boll',
          timeframe: '15m',
          inputs: [{ scope: 'market', name: 'bar', field: 'close' }],
          params: { period: 20, stddev: 2.0 }
        }
      ],
      rules: [
        {
          id: 'open_long',
          when: {
            kind: 'operator',
            op: 'and',
            args: [
              {
                kind: 'operator',
                op: 'cross_up',
                args: [
                  { kind: 'reference', ref: { scope: 'indicator', id: 'ema_fast_15m', field: 'value', offset: 0 } },
                  { kind: 'reference', ref: { scope: 'indicator', id: 'ema_slow_15m', field: 'value', offset: 0 } }
                ]
              },
              {
                kind: 'operator',
                op: 'gt',
                args: [
                  { kind: 'reference', ref: { scope: 'indicator', id: 'rsi_14_4h', field: 'value', offset: 0 } },
                  { kind: 'number', number: 45.0 }
                ]
              }
            ]
          },
          action: {
            type: 'open',
            side: 'long',
            size_pct: 0.3
          }
        },
        {
          id: 'close_long',
          when: {
            kind: 'operator',
            op: 'or',
            args: [
              {
                kind: 'operator',
                op: 'cross_down',
                args: [
                  { kind: 'reference', ref: { scope: 'indicator', id: 'ema_fast_15m', field: 'value', offset: 0 } },
                  { kind: 'reference', ref: { scope: 'indicator', id: 'ema_slow_15m', field: 'value', offset: 0 } }
                ]
              },
              {
                kind: 'operator',
                op: 'gt',
                args: [
                  { kind: 'reference', ref: { scope: 'indicator', id: 'rsi_14_4h', field: 'value', offset: 0 } },
                  { kind: 'number', number: 80.0 }
                ]
              }
            ]
          },
          action: {
            type: 'close',
            side: 'long',
            size_pct: 1.0
          }
        }
      ]
    };
  }
}

window.StrategyWorkbench = StrategyWorkbench;
