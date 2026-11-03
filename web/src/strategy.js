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

              <div class="visual-section">
                <h3 class="section-heading">杠杆管理 (LEVERAGE)</h3>
                <div class="visual-grid-cards visual-grid-cards--compact" id="leverageCards">
                  <!-- 动态插入 -->
                </div>
              </div>

              <div class="visual-section">
                <h3 class="section-heading">仓位管理 (POSITION)</h3>
                <div class="visual-grid-cards" id="positionCards">
                  <!-- 动态插入 -->
                </div>
              </div>

              <div class="visual-section">
                <div class="section-heading-row">
                  <h3 class="section-heading">止盈止损策略 (EXIT POLICIES)</h3>
                  <span class="section-badge" id="exitPolicyCountBadge">0 个</span>
                  <button class="btn btn-primary-sm" id="btnAddExitPolicy" style="margin-left: auto;">+ 添加退出策略</button>
                </div>
                <div class="exit-policies-container" id="exitPoliciesCards">
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
    const btnAddExitPolicy = document.getElementById('btnAddExitPolicy');

    btnNew.addEventListener('click', () => this.createNewStrategy());
    if (btnAddIndicator) btnAddIndicator.addEventListener('click', () => this.showIndicatorModal());
    if (btnAddRule) btnAddRule.addEventListener('click', () => this.addNewRule());
    if (btnAddExitPolicy) btnAddExitPolicy.addEventListener('click', () => this.addExitPolicy());

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
    try {
      const data = await Api.fetchCatalog();
      this.catalog = data.indicators || [];
    } catch (err) {
      this.catalog = [];
      const container = document.getElementById('catalogListContainer');
      if (container) {
        container.innerHTML = '';
        const message = document.createElement('div');
        message.className = 'loading-hint';
        message.textContent = `指标目录加载失败: ${err.message}`;
        container.appendChild(message);
      }
      this.showAlert(`指标目录加载失败: ${err.message}`, 'error');
      return;
    }
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
    this.renderExitPoliciesSection(def);
    this.renderIndicatorsSection(def);
    this.renderRulesEditor(def);
  }

  syncToJson() {
    const area = document.getElementById('jsonEditorArea');
    if (area && this.currentStrategy) {
      area.value = JSON.stringify(this.currentStrategy, null, 2);
    }
  }

  migrateLeverageConfig(def) {
    const account = def.account || (def.account = {});
    const position = def.position || (def.position = {});
    const risk = def.risk || (def.risk = {});

    if (account.exchange_leverage === undefined || account.exchange_leverage === null) {
      account.exchange_leverage = 1;
    }
    if (position.position_leverage === undefined || position.position_leverage === null) {
      position.position_leverage = position.target_leverage ?? 1;
    }
    if (position.long_capital_decay_power === undefined ||
        position.long_capital_decay_power === null ||
        position.long_capital_decay_power === 0) {
      position.long_capital_decay_power = 0.5;
    }
    if (position.short_capital_decay_power === undefined ||
        position.short_capital_decay_power === null ||
        position.short_capital_decay_power === 0) {
      position.short_capital_decay_power = 0.5;
    }
    if (account.market_type === 'spot') {
      account.exchange_leverage = 1;
      position.position_leverage = 1;
    }

    delete position.target_leverage;
    delete position.max_position_pct;
    delete risk.max_symbol_equity_pct;
    delete risk.max_total_exposure_pct;
    return def;
  }

  renderOverviewForm(def) {
    this.migrateLeverageConfig(def);
    document.getElementById('viewStrategyName').textContent = def.name || def.id;
    document.getElementById('viewStrategyId').textContent = def.id;
    document.getElementById('viewMarketType').textContent = (def.account && def.account.market_type) || 'spot';
    document.getElementById('viewStrategyDesc').textContent = def.description || '无策略描述说明';

    const overview = document.getElementById('overviewCards');
    const leverageCards = document.getElementById('leverageCards');
    const positionCards = document.getElementById('positionCards');
    const acc = def.account || (def.account = {});
    const data = def.data || {};
    const position = def.position || (def.position = {});
    const behavior = def.behavior || (def.behavior = {});
    const exchangeLeverage = acc.exchange_leverage || 1;
    const positionLeverage = position.position_leverage || 1;
    const isSpot = acc.market_type === 'spot';
    const addMode = position.add_size_multiplier > 0
      ? 'multiplier'
      : (position.add_size_pct > 0 ? 'fixed' : 'initial');
    const addValue = addMode === 'multiplier'
      ? position.add_size_multiplier
      : ((position.add_size_pct || position.initial_size_pct || 0) * 100);
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
    leverageCards.innerHTML = `
      <div class="visual-card">
        <label class="vcard-label">交易所配置杠杆</label>
        <div class="input-with-unit">
          <input type="number" class="form-control" id="formExchangeLeverage"
            value="${exchangeLeverage}" min="1" step="0.1" ${isSpot ? 'disabled' : ''}>
          <span class="input-unit">x</span>
        </div>
      </div>
      <div class="visual-card">
        <label class="vcard-label">实际仓位杠杆</label>
        <div class="input-with-unit">
          <input type="number" class="form-control" id="formPositionLeverage"
            value="${positionLeverage}" min="1" max="${exchangeLeverage}" step="0.1" ${isSpot ? 'disabled' : ''}>
          <span class="input-unit">x</span>
        </div>
      </div>
    `;
    positionCards.innerHTML = `
      <div class="visual-card">
        <label class="vcard-label">初始仓位分配</label>
        <div class="input-with-unit">
          <input type="number" class="form-control" id="formInitialSizePct"
            value="${((position.initial_size_pct || 0) * 100).toFixed(6)}" min="0" max="100" step="any">
          <span class="input-unit">%</span>
        </div>
      </div>
      <div class="visual-card">
        <label class="vcard-label">做多资金衰减系数</label>
        <input type="number" class="form-control" id="formLongCapitalDecayPower"
          value="${position.long_capital_decay_power}" min="0.01" step="0.05">
      </div>
      <div class="visual-card">
        <label class="vcard-label">做空资金衰减系数</label>
        <input type="number" class="form-control" id="formShortCapitalDecayPower"
          value="${position.short_capital_decay_power}" min="0.01" step="0.05">
      </div>
      <div class="visual-card">
        <label class="vcard-label">允许同向加仓</label>
        <label class="binary-control">
          <input type="checkbox" id="formAllowPyramiding" ${behavior.allow_pyramiding ? 'checked' : ''}>
          <span>${behavior.allow_pyramiding ? '已启用' : '已关闭'}</span>
        </label>
      </div>
      <div class="visual-card">
        <label class="vcard-label">最大加仓次数</label>
        <input type="number" class="form-control" id="formMaxAdditions"
          value="${position.max_additions || 0}" min="0" step="1" ${behavior.allow_pyramiding ? '' : 'disabled'}>
      </div>
      <div class="visual-card">
        <label class="vcard-label">加仓计算模式</label>
        <select class="form-select" id="formAddMode" ${behavior.allow_pyramiding && position.max_additions > 0 ? '' : 'disabled'}>
          <option value="initial" ${addMode === 'initial' ? 'selected' : ''}>沿用首次名义仓位</option>
          <option value="fixed" ${addMode === 'fixed' ? 'selected' : ''}>固定比例</option>
          <option value="multiplier" ${addMode === 'multiplier' ? 'selected' : ''}>倍增模式</option>
        </select>
      </div>
      <div class="visual-card">
        <label class="vcard-label">${addMode === 'multiplier' ? '加仓倍数' : '固定加仓比例'}</label>
        <div class="input-with-unit">
          <input type="number" class="form-control" id="formAddValue"
            value="${Number(addValue).toFixed(addMode === 'multiplier' ? 2 : 4)}"
            min="0" step="${addMode === 'multiplier' ? '0.1' : 'any'}"
            ${behavior.allow_pyramiding && position.max_additions > 0 && addMode !== 'initial' ? '' : 'disabled'}>
          <span class="input-unit">${addMode === 'multiplier' ? 'x' : '%'}</span>
        </div>
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
      if (e.target.value === 'spot') {
        def.account.exchange_leverage = 1;
        def.position.position_leverage = 1;
      }
      document.getElementById('viewMarketType').textContent = e.target.value;
      this.renderOverviewForm(def);
      this.syncToJson();
    });
    document.getElementById('formSymbols').addEventListener('change', (e) => {
      if (!def.data) def.data = {};
      def.data.symbols = e.target.value.split(',').map(s => s.trim()).filter(Boolean);
      this.syncToJson();
    });

    const parseNumber = (id, fallback) => {
      const value = parseFloat(document.getElementById(id).value);
      return Number.isFinite(value) ? value : fallback;
    };
    document.getElementById('formExchangeLeverage').addEventListener('change', () => {
      const value = Math.max(1, parseNumber('formExchangeLeverage', 1));
      def.account.exchange_leverage = value;
      this.renderOverviewForm(def);
      this.syncToJson();
    });
    document.getElementById('formPositionLeverage').addEventListener('change', () => {
      def.position.position_leverage = Math.min(
        exchangeLeverage,
        Math.max(1, parseNumber('formPositionLeverage', 1)),
      );
      this.syncToJson();
    });
    document.getElementById('formInitialSizePct').addEventListener('change', () => {
      def.position.initial_size_pct = Math.min(
        100,
        Math.max(0, parseNumber('formInitialSizePct', 0)),
      ) / 100;
      this.syncToJson();
    });
    document.getElementById('formLongCapitalDecayPower').addEventListener('change', () => {
      def.position.long_capital_decay_power = Math.max(
        0.01,
        parseNumber('formLongCapitalDecayPower', 0.5),
      );
      this.syncToJson();
    });
    document.getElementById('formShortCapitalDecayPower').addEventListener('change', () => {
      def.position.short_capital_decay_power = Math.max(
        0.01,
        parseNumber('formShortCapitalDecayPower', 0.5),
      );
      this.syncToJson();
    });
    document.getElementById('formAllowPyramiding').addEventListener('change', (e) => {
      def.behavior.allow_pyramiding = e.target.checked;
      if (!e.target.checked) {
        def.position.max_additions = 0;
        def.position.add_size_pct = 0;
        def.position.add_size_multiplier = 0;
      }
      this.renderOverviewForm(def);
      this.syncToJson();
    });
    document.getElementById('formMaxAdditions').addEventListener('change', () => {
      def.position.max_additions = Math.max(
        0,
        Math.trunc(parseNumber('formMaxAdditions', 0)),
      );
      if (def.position.max_additions === 0) {
        def.position.add_size_pct = 0;
        def.position.add_size_multiplier = 0;
      }
      this.renderOverviewForm(def);
      this.syncToJson();
    });
    document.getElementById('formAddMode').addEventListener('change', (e) => {
      if (e.target.value === 'fixed') {
        def.position.add_size_pct = def.position.initial_size_pct || 0;
        def.position.add_size_multiplier = 0;
      } else if (e.target.value === 'multiplier') {
        def.position.add_size_pct = 0;
        def.position.add_size_multiplier = 2;
      } else {
        def.position.add_size_pct = 0;
        def.position.add_size_multiplier = 0;
      }
      this.renderOverviewForm(def);
      this.syncToJson();
    });
    document.getElementById('formAddValue').addEventListener('change', () => {
      const mode = document.getElementById('formAddMode').value;
      const value = Math.max(0, parseNumber('formAddValue', 0));
      if (mode === 'multiplier') {
        def.position.add_size_multiplier = value;
        def.position.add_size_pct = 0;
      } else if (mode === 'fixed') {
        def.position.add_size_pct = Math.min(100, value) / 100;
        def.position.add_size_multiplier = 0;
      }
      this.syncToJson();
    });
  }

  getVolatilityOperands() {
    const items = [];
    const indicators = (this.currentStrategy && this.currentStrategy.indicators) || [];
    for (const ind of indicators) {
      const definition = this.catalog.find(item => item.type === ind.type);
      const outputs = definition?.outputs || (ind.type === 'atr' ? [{ name: 'value', semantic: 'volatility_series' }] : []);
      for (const output of outputs) {
        if (output.semantic !== 'volatility_series') continue;
        items.push({
          value: `${ind.id}:${output.name}`,
          label: `${definition?.label || ind.type.toUpperCase()}: ${ind.id}.${output.name}`,
          indicator: ind.id,
          field: output.name,
        });
      }
    }
    return items;
  }

  defaultExitTrigger(side, purpose) {
    return (side === 'long' && purpose === 'take_profit') ||
      (side === 'short' && purpose === 'stop_loss') ? 'high' : 'low';
  }

  normalizeExitPolicy(policy) {
    policy.anchor = policy.anchor || { type: 'avg_entry_price' };
    policy.anchor.type = policy.anchor.type || 'avg_entry_price';
    policy.distance = policy.distance || { type: 'percent', value: 0.03 };
    policy.distance.type = policy.distance.type || 'percent';
    policy.distance.value = Number(policy.distance.value) || (policy.distance.type === 'percent' ? 0.03 : 2);
    policy.side = policy.side || 'long';
    policy.purpose = policy.purpose || 'take_profit';
    policy.trigger_price = policy.trigger_price || this.defaultExitTrigger(policy.side, policy.purpose);
    policy.close_pct = Number(policy.close_pct) || 1;
    policy.execution = policy.execution || 'next_open';
    policy.priority = Number(policy.priority) || 0;
    if (policy.distance.type === 'atr_multiple') {
      policy.distance.field = policy.distance.field || 'value';
      policy.distance.sampling = policy.distance.sampling || 'live';
    } else {
      delete policy.distance.indicator;
      delete policy.distance.field;
      delete policy.distance.sampling;
    }
    return policy;
  }

  renderExitPoliciesSection(def) {
    const container = document.getElementById('exitPoliciesCards');
    if (!container) return;
    const policies = def.exit_policies || (def.exit_policies = []);
    const volatilityOperands = this.getVolatilityOperands();
    document.getElementById('exitPolicyCountBadge').textContent = `${policies.length} 个`;

    if (policies.length === 0) {
      container.innerHTML = '<div class="empty-state-hint">暂无退出策略，可添加 ATR、百分比或固定价差止盈止损。</div>';
      return;
    }

    container.innerHTML = policies.map((rawPolicy, index) => {
      const policy = this.normalizeExitPolicy(rawPolicy);
      const isATR = policy.distance.type === 'atr_multiple';
      const selectedIndicator = `${policy.distance.indicator || ''}:${policy.distance.field || 'value'}`;
      const indicatorExists = volatilityOperands.some(item => item.value === selectedIndicator);
      const indicatorOptions = [
        ...(!indicatorExists && policy.distance.indicator ? [{
          value: selectedIndicator,
          label: `缺失指标: ${policy.distance.indicator}.${policy.distance.field || 'value'}`,
        }] : []),
        ...volatilityOperands,
      ];
      return `
        <div class="rule-card-editor exit-policy-card" data-exit-idx="${index}">
          <div class="rule-top-bar">
            <div class="rule-top-inputs">
              <span class="panel-sub-label">策略ID:</span>
              <input type="text" class="form-control exit-policy-field" data-exit-idx="${index}" data-exit-field="id" value="${policy.id || ''}">
              <label class="binary-control exit-enabled-control">
                <input type="checkbox" class="exit-policy-field" data-exit-idx="${index}" data-exit-field="enabled" ${policy.disabled ? '' : 'checked'}>
                <span>${policy.disabled ? '已停用' : '已启用'}</span>
              </label>
            </div>
            <button class="btn-xs btn-danger btn-del-exit-policy" data-exit-idx="${index}">✕ 删除策略</button>
          </div>
          <div class="exit-policy-grid">
            <div class="form-group">
              <label class="form-label">持仓方向</label>
              <select class="form-select exit-policy-field" data-exit-idx="${index}" data-exit-field="side">
                <option value="long" ${policy.side === 'long' ? 'selected' : ''}>多头</option>
                <option value="short" ${policy.side === 'short' ? 'selected' : ''}>空头</option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">用途</label>
              <select class="form-select exit-policy-field" data-exit-idx="${index}" data-exit-field="purpose">
                <option value="take_profit" ${policy.purpose === 'take_profit' ? 'selected' : ''}>止盈</option>
                <option value="stop_loss" ${policy.purpose === 'stop_loss' ? 'selected' : ''}>止损</option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">价格锚点</label>
              <select class="form-select exit-policy-field" data-exit-idx="${index}" data-exit-field="anchor">
                <option value="avg_entry_price" ${policy.anchor.type === 'avg_entry_price' ? 'selected' : ''}>持仓均价</option>
                <option value="first_entry_price" ${policy.anchor.type === 'first_entry_price' ? 'selected' : ''}>首次开仓价</option>
                <option value="last_entry_price" ${policy.anchor.type === 'last_entry_price' ? 'selected' : ''}>最后加仓价</option>
                <option value="highest_since_entry" ${policy.anchor.type === 'highest_since_entry' ? 'selected' : ''}>入场后最高价</option>
                <option value="lowest_since_entry" ${policy.anchor.type === 'lowest_since_entry' ? 'selected' : ''}>入场后最低价</option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">距离类型</label>
              <select class="form-select exit-policy-field" data-exit-idx="${index}" data-exit-field="distance_type">
                <option value="percent" ${policy.distance.type === 'percent' ? 'selected' : ''}>百分比</option>
                <option value="atr_multiple" ${isATR ? 'selected' : ''}>ATR 倍数</option>
                <option value="fixed_amount" ${policy.distance.type === 'fixed_amount' ? 'selected' : ''}>固定价差</option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">${policy.distance.type === 'percent' ? '距离（0.03 = 3%）' : isATR ? 'ATR 倍数' : '价格距离'}</label>
              <input type="number" class="form-control exit-policy-field" data-exit-idx="${index}" data-exit-field="distance_value"
                value="${policy.distance.value}" min="0.000001" step="any">
            </div>
            <div class="form-group">
              <label class="form-label">波动率指标</label>
              <select class="form-select exit-policy-field" data-exit-idx="${index}" data-exit-field="indicator" ${isATR ? '' : 'disabled'}>
                ${indicatorOptions.length > 0
                  ? indicatorOptions.map(item => `<option value="${item.value}" ${item.value === selectedIndicator ? 'selected' : ''}>${item.label}</option>`).join('')
                  : '<option value=":value">请先添加 ATR 指标</option>'}
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">ATR 采样方式</label>
              <select class="form-select exit-policy-field" data-exit-idx="${index}" data-exit-field="sampling" ${isATR ? '' : 'disabled'}>
                <option value="entry" ${policy.distance.sampling === 'entry' ? 'selected' : ''}>首次开仓时冻结</option>
                <option value="each_entry" ${policy.distance.sampling === 'each_entry' ? 'selected' : ''}>每次加仓后刷新</option>
                <option value="live" ${policy.distance.sampling === 'live' ? 'selected' : ''}>逐 K 更新（下一根生效）</option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">触发价格</label>
              <select class="form-select exit-policy-field" data-exit-idx="${index}" data-exit-field="trigger_price">
                <option value="high" ${policy.trigger_price === 'high' ? 'selected' : ''}>最高价</option>
                <option value="low" ${policy.trigger_price === 'low' ? 'selected' : ''}>最低价</option>
                <option value="close" ${policy.trigger_price === 'close' ? 'selected' : ''}>收盘价</option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">成交方式</label>
              <select class="form-select exit-policy-field" data-exit-idx="${index}" data-exit-field="execution">
                <option value="next_open" ${policy.execution === 'next_open' ? 'selected' : ''}>下一根开盘</option>
                <option value="trigger_price" ${policy.execution === 'trigger_price' ? 'selected' : ''}>触发价 / 跳空价</option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">平仓比例（%）</label>
              <input type="number" class="form-control exit-policy-field" data-exit-idx="${index}" data-exit-field="close_pct"
                value="${Number(policy.close_pct * 100).toFixed(4)}" min="0.0001" max="100" step="any">
            </div>
            <div class="form-group">
              <label class="form-label">优先级</label>
              <input type="number" class="form-control exit-policy-field" data-exit-idx="${index}" data-exit-field="priority"
                value="${policy.priority}" step="1">
            </div>
          </div>
        </div>
      `;
    }).join('');

    container.querySelectorAll('.exit-policy-field').forEach(field => {
      field.addEventListener('change', () => {
        const index = parseInt(field.dataset.exitIdx, 10);
        const policy = policies[index];
        const key = field.dataset.exitField;
        if (!policy) return;
        switch (key) {
          case 'id': policy.id = field.value.trim(); break;
          case 'enabled': policy.disabled = !field.checked; break;
          case 'side':
          case 'purpose':
            policy[key] = field.value;
            policy.trigger_price = this.defaultExitTrigger(policy.side, policy.purpose);
            break;
          case 'anchor': policy.anchor.type = field.value; break;
          case 'distance_type': {
            policy.distance.type = field.value;
            policy.distance.value = field.value === 'percent' ? 0.03 : field.value === 'atr_multiple' ? 2 : 100;
            if (field.value === 'atr_multiple') {
              const first = volatilityOperands[0];
              policy.distance.indicator = first?.indicator || '';
              policy.distance.field = first?.field || 'value';
              policy.distance.sampling = 'live';
            } else {
              delete policy.distance.indicator;
              delete policy.distance.field;
              delete policy.distance.sampling;
            }
            break;
          }
          case 'distance_value': policy.distance.value = Math.max(0.000001, Number(field.value) || 0); break;
          case 'indicator': {
            const [indicator, outputField] = field.value.split(':');
            policy.distance.indicator = indicator;
            policy.distance.field = outputField || 'value';
            break;
          }
          case 'sampling': policy.distance.sampling = field.value; break;
          case 'trigger_price': policy.trigger_price = field.value; break;
          case 'execution': policy.execution = field.value; break;
          case 'close_pct': policy.close_pct = Math.min(100, Math.max(0.0001, Number(field.value) || 0)) / 100; break;
          case 'priority': policy.priority = Math.trunc(Number(field.value) || 0); break;
        }
        this.renderExitPoliciesSection(def);
        this.syncToJson();
      });
    });
    container.querySelectorAll('.btn-del-exit-policy').forEach(button => {
      button.addEventListener('click', () => {
        const index = parseInt(button.dataset.exitIdx, 10);
        policies.splice(index, 1);
        this.renderExitPoliciesSection(def);
        this.syncToJson();
      });
    });
  }

  addExitPolicy() {
    if (!this.currentStrategy) return;
    const policies = this.currentStrategy.exit_policies || (this.currentStrategy.exit_policies = []);
    const volatility = this.getVolatilityOperands()[0];
    const useATR = Boolean(volatility);
    const sequence = policies.length + 1;
    policies.push({
      id: `exit_${sequence}`,
      side: 'long',
      purpose: 'take_profit',
      anchor: { type: 'avg_entry_price' },
      distance: useATR
        ? { type: 'atr_multiple', value: 2, indicator: volatility.indicator, field: volatility.field, sampling: 'live' }
        : { type: 'percent', value: 0.03 },
      trigger_price: 'high',
      close_pct: 1,
      priority: 0,
      execution: 'next_open',
    });
    this.renderExitPoliciesSection(this.currentStrategy);
    this.syncToJson();
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
            <span class="kv-k">参数配置:</span>
            <span class="kv-v">${Object.entries(ind.params || {}).map(([k, v]) => `${k}=${v}`).join(', ') || '默认'}</span>
          </div>
          <div class="ind-kv">
            <span class="kv-k">计算输入:</span>
            <span class="kv-v text-accent">K线行情数据 (内部自动计算)</span>
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
    const currentParams = (existingInd && existingInd.params) || {};

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

            <div class="form-group">
              <label class="form-label">计算周期 (Timeframe)</label>
              <select class="form-select" id="modalIndTf">
                <option value="15m" ${defaultTf === '15m' ? 'selected' : ''}>15m (基准)</option>
                <option value="1h" ${defaultTf === '1h' ? 'selected' : ''}>1h (小时线)</option>
                <option value="4h" ${defaultTf === '4h' ? 'selected' : ''}>4h (四小时线)</option>
                <option value="1d" ${defaultTf === '1d' ? 'selected' : ''}>1d (日线)</option>
              </select>
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
      const definition = this.catalog.find(item => item.type === type);
      const labels = {
        period: '计算周期 (Period)',
        stddev: '标准差倍数 (StdDev)',
        multiplier: 'ATR 倍数 (Multiplier)',
      };
      const defaults = {
        period: type === 'supertrend' ? 10 : (type === 'rsi' || type === 'atr' ? 14 : 20),
        stddev: 2,
        multiplier: 3,
      };
      pBox.innerHTML = (definition?.params || []).map(param => {
        const value = currentParams[param.name] ?? defaults[param.name] ?? 1;
        const step = param.type === 'int' ? 1 : 0.1;
        const min = param.name === 'stddev' || param.name === 'multiplier'
          ? Math.max(0.1, Number(param.min || 0))
          : Number(param.min || 0);
        return `
          <div class="form-group">
            <label class="form-label">${labels[param.name] || param.name}</label>
            <input
              type="number"
              class="form-control indicator-param-input"
              data-param-name="${param.name}"
              data-param-type="${param.type}"
              value="${value}"
              step="${step}"
              min="${min}">
          </div>
        `;
      }).join('');
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
      if (!id) {
        alert('请输入指标唯一标识');
        return;
      }

      const params = {};
      document.querySelectorAll('.indicator-param-input').forEach(input => {
        params[input.dataset.paramName] = input.dataset.paramType === 'int'
          ? parseInt(input.value, 10)
          : parseFloat(input.value);
      });

      // 系统内部全自动解构并映射 K 线结构体输入，无需用户人工指定价格字段
      let inputs = [];
      if (type === 'atr' || type === 'supertrend') {
        inputs = [
          { scope: 'market', name: 'bar', field: 'high' },
          { scope: 'market', name: 'bar', field: 'low' },
          { scope: 'market', name: 'bar', field: 'close' }
        ];
      } else {
        inputs = [{ scope: 'market', name: 'bar', field: 'close' }];
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
      { label: '行情: 收盘价 (close)', value: 'market:close', semantic: 'price_series', domain: 'price' },
      { label: '行情: 开盘价 (open)', value: 'market:open', semantic: 'price_series', domain: 'price' },
      { label: '行情: 最高价 (high)', value: 'market:high', semantic: 'price_series', domain: 'price' },
      { label: '行情: 最低价 (low)', value: 'market:low', semantic: 'price_series', domain: 'price' },
      { label: '行情: 成交量 (volume)', value: 'market:volume', semantic: 'volume_series', domain: 'volume' },
      { label: '持仓: 持仓均价', value: 'position:avg_entry_price', semantic: 'position_price', domain: 'price' },
      { label: '持仓: 最新开仓/加仓价', value: 'position:last_entry_price', semantic: 'position_price', domain: 'price' },
      { label: '持仓: 未实现盈亏率 (0.02 = 2%)', value: 'position:unrealized_pnl_pct', semantic: 'position_ratio', domain: 'ratio' },
      { label: '持仓: 开仓次数', value: 'position:entry_count', semantic: 'position_count', domain: 'count' },
      { label: '持仓: 当前实际杠杆', value: 'position:actual_leverage', semantic: 'position_leverage', domain: 'leverage' },
    ];
    const indicators = (this.currentStrategy && this.currentStrategy.indicators) || [];
    for (const ind of indicators) {
      const definition = this.catalog.find(item => item.type === ind.type);
      const outputs = definition?.outputs || [{ name: 'value' }];
      for (const output of outputs) {
        list.push({
          label: `${definition?.label || ind.type.toUpperCase()}: ${ind.id}.${output.name} (${ind.timeframe || '15m'})`,
          value: `indicator:${ind.id}:${output.name}`,
          semantic: output.semantic || 'numeric_series',
          domain: this.outputDomain(output.semantic),
          crossGroup: output.cross_group || '',
          indicatorType: ind.type,
        });
      }
    }
    return list;
  }

  getAvailableIndicatorOperands() {
    return this.getAvailableOperands().filter(item => item.value.startsWith('indicator:'));
  }

  outputDomain(semantic) {
    switch (semantic) {
      case 'price_series':
      case 'trend_level':
        return 'price';
      case 'oscillator_series':
        return 'oscillator';
      case 'volatility_series':
        return 'volatility';
      case 'trend_direction':
        return 'trend_direction';
      default:
        return 'numeric';
    }
  }

  isCrossOperator(op) {
    return op === 'cross_up' || op === 'cross_down';
  }

  isSemanticOperator(op) {
    return [
      'bullish_trend',
      'bearish_trend',
      'trend_turns_bullish',
      'trend_turns_bearish',
      'support',
      'resistance',
    ].includes(op);
  }

  operatorOptionsFor(item, operands) {
    const left = operands.find(option => option.value === item.left);
    if (left?.semantic === 'trend_direction') {
      return [
        { value: 'bullish_trend', label: '多头趋势' },
        { value: 'bearish_trend', label: '空头趋势' },
        { value: 'trend_turns_bullish', label: '趋势转多' },
        { value: 'trend_turns_bearish', label: '趋势转空' },
      ];
    }
    if (left?.semantic === 'trend_level') {
      return [
        { value: 'support', label: '价格位于支撑侧' },
        { value: 'resistance', label: '价格位于阻力侧' },
      ];
    }

    const options = [
      { value: 'gt', label: '大于 (&gt;)' },
      { value: 'gte', label: '大于等于 (&gt;=)' },
      { value: 'lt', label: '小于 (&lt;)' },
      { value: 'lte', label: '小于等于 (&lt;=)' },
      { value: 'eq', label: '等于 (==)' },
    ];
    const crossCandidates = this.compatibleCrossOperands(item.left, operands);
    if (left?.crossGroup && crossCandidates.length > 0) {
      options.push(
        { value: 'cross_up', label: '金叉：同类指标上穿' },
        { value: 'cross_down', label: '死叉：同类指标下穿' },
      );
    }
    return options;
  }

  compatibleCrossOperands(leftValue, operands) {
    const left = operands.find(option => option.value === leftValue);
    if (!left?.crossGroup) return [];
    return operands.filter(option =>
      option.value !== leftValue &&
      option.crossGroup === left.crossGroup
    );
  }

  compatibleComparisonOperands(leftValue, operands) {
    const left = operands.find(option => option.value === leftValue);
    if (!left) return [];
    return operands.filter(option =>
      option.value !== leftValue &&
      option.domain === left.domain &&
      option.semantic !== 'trend_direction'
    );
  }

  normalizeConditionItem(item, operands) {
    const left = operands.find(option => option.value === item.left) || operands[0];
    item.left = left.value;
    const operatorOptions = this.operatorOptionsFor(item, operands);
    if (!operatorOptions.some(option => option.value === item.op)) {
      item.op = operatorOptions[0]?.value || 'gt';
    }

    if (this.isSemanticOperator(item.op)) {
      item.rightType = 'semantic';
      return item;
    }
    if (this.isCrossOperator(item.op)) {
      const candidates = this.compatibleCrossOperands(item.left, operands);
      item.rightType = 'indicator';
      if (!candidates.some(option => option.value === item.rightIndicator)) {
        item.rightIndicator = candidates[0]?.value || item.left;
      }
      return item;
    }

    if (item.rightType !== 'number' && item.rightType !== 'indicator') {
      item.rightType = 'number';
    }
    if (item.rightType === 'indicator') {
      const candidates = this.compatibleComparisonOperands(item.left, operands);
      if (!candidates.some(option => option.value === item.rightIndicator)) {
        if (candidates.length > 0) item.rightIndicator = candidates[0].value;
        else item.rightType = 'number';
      }
    }
    return item;
  }

  renderRulesEditor(def) {
    const rulesContainer = document.getElementById('rulesTreeContainer');
    const rules = def.rules || [];
    document.getElementById('ruleCountBadge').textContent = `${rules.length} 组`;
    const operands = this.getAvailableOperands();

    rulesContainer.innerHTML = rules.map((rule, rIdx) => {
      const parsedWhen = this.parseAstWhen(rule.when);
      parsedWhen.items.forEach(item => this.normalizeConditionItem(item, operands));
      rule.when = this.compileAstWhen(parsedWhen.gate, parsedWhen.items);
      const action = rule.action || { type: 'open', side: 'long', size_pct: 0.25 };
      const sizePct = Number((((action.size_pct ?? 0.25) * 100).toFixed(4)));

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
              <input type="range" class="pct-slider rule-size-slider" data-ridx="${rIdx}" min="0" max="100" step="0.1" value="${sizePct}">
              <div class="rule-size-number-wrap">
                <input type="number" class="form-control rule-size-input" data-ridx="${rIdx}"
                  min="0" max="100" step="any" value="${sizePct}">
                <span>%</span>
              </div>
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
    this.normalizeConditionItem(item, operands);
    const isCross = this.isCrossOperator(item.op);
    const isSemantic = this.isSemanticOperator(item.op);
    const operatorOptions = this.operatorOptionsFor(item, operands);
    const rightOperands = isCross
      ? this.compatibleCrossOperands(item.left, operands)
      : this.compatibleComparisonOperands(item.left, operands);
    const isNum = !isCross && !isSemantic && item.rightType === 'number';
    const semanticHints = {
      bullish_trend: 'direction = 1',
      bearish_trend: 'direction = -1',
      trend_turns_bullish: '-1 → 1',
      trend_turns_bearish: '1 → -1',
      support: 'close ≥ trend line',
      resistance: 'close ≤ trend line',
    };
    return `
      <div class="condition-item-row" data-ridx="${rIdx}" data-cidx="${cIdx}">
        <select class="form-select cond-left-select" data-ridx="${rIdx}" data-cidx="${cIdx}">
          ${operands.map(op => `<option value="${op.value}" ${op.value === item.left ? 'selected' : ''}>${op.label}</option>`).join('')}
        </select>

        <select class="form-select cond-op-select" data-ridx="${rIdx}" data-cidx="${cIdx}">
          ${operatorOptions.map(option => `
            <option value="${option.value}" ${item.op === option.value ? 'selected' : ''}>${option.label}</option>
          `).join('')}
        </select>

        <div class="right-operand-group">
          ${isSemantic ? `
            <span class="semantic-condition-hint">${semanticHints[item.op]}</span>
          ` : isNum ? `
            <input type="number" step="0.5" class="form-control cond-right-num" data-ridx="${rIdx}" data-cidx="${cIdx}" value="${item.rightNumber}">
          ` : `
            <select class="form-select cond-right-select" data-ridx="${rIdx}" data-cidx="${cIdx}">
              ${rightOperands.map(op => `<option value="${op.value}" ${op.value === item.rightIndicator ? 'selected' : ''}>${op.label}</option>`).join('')}
            </select>
          `}
          ${isCross ? '<span class="operand-kind-label">同类指标</span>' : isSemantic ? '' : `
            <button class="btn-toggle-operand" data-ridx="${rIdx}" data-cidx="${cIdx}" title="切换右操作数模式 (常数/变量)">
              ${isNum ? '常数' : '变量'}
            </button>
          `}
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

    const updateRuleSize = (rIdx, value) => {
      const normalized = Math.min(100, Math.max(0, Number(value) || 0));
      const slider = container.querySelector(`.rule-size-slider[data-ridx="${rIdx}"]`);
      const input = container.querySelector(`.rule-size-input[data-ridx="${rIdx}"]`);
      if (slider) slider.value = normalized;
      if (input) input.value = normalized;
      def.rules[rIdx].action.size_pct = normalized / 100;
      this.syncToJson();
    };

    // 仓位滑块
    container.querySelectorAll('.rule-size-slider').forEach(slider => {
      slider.addEventListener('input', (e) => {
        const rIdx = parseInt(e.target.dataset.ridx, 10);
        updateRuleSize(rIdx, e.target.value);
      });
    });
    container.querySelectorAll('.rule-size-input').forEach(input => {
      input.addEventListener('change', (e) => {
        const rIdx = parseInt(e.target.dataset.ridx, 10);
        updateRuleSize(rIdx, e.target.value);
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
          const item = parsed.items[cIdx];
          item.left = sel.value;
          this.normalizeConditionItem(item, this.getAvailableOperands());
          def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
          this.renderRulesEditor(def);
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
          const item = parsed.items[cIdx];
          item.op = sel.value;
          this.normalizeConditionItem(item, this.getAvailableOperands());
          def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items);
          this.renderRulesEditor(def);
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
          if (
            this.isCrossOperator(parsed.items[cIdx].op) ||
            this.isSemanticOperator(parsed.items[cIdx].op)
          ) return;
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
    const newRule = {
      id: id,
      when: {
        kind: 'operator',
        op: 'gt',
        args: [
          this.operandValueToNode('market:close'),
          { kind: 'number', number: 0 }
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
    const semanticItem = this.parseSemanticCondition(node);
    if (semanticItem) return semanticItem;

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

  parseSemanticCondition(node) {
    const operands = this.getAvailableOperands();
    const leftNode = node.args && node.args[0];
    const rightNode = node.args && node.args[1];
    const leftValue = this.nodeToOperandValue(leftNode);
    const rightValue = this.nodeToOperandValue(rightNode);
    const leftOperand = operands.find(item => item.value === leftValue);
    const rightOperand = operands.find(item => item.value === rightValue);

    if (
      (node.op === 'turn_bullish' || node.op === 'turn_bearish') &&
      leftOperand?.semantic === 'trend_direction'
    ) {
      return {
        left: leftValue,
        op: node.op === 'turn_bullish' ? 'trend_turns_bullish' : 'trend_turns_bearish',
        rightType: 'semantic',
        rightIndicator: '',
        rightNumber: 0,
      };
    }

    if (
      node.op === 'eq' &&
      leftOperand?.semantic === 'trend_direction' &&
      rightNode?.kind === 'number' &&
      (rightNode.number === 1 || rightNode.number === -1)
    ) {
      return {
        left: leftValue,
        op: rightNode.number === 1 ? 'bullish_trend' : 'bearish_trend',
        rightType: 'semantic',
        rightIndicator: '',
        rightNumber: rightNode.number,
      };
    }

    const leftIsClose = leftValue === 'market:close';
    const rightIsClose = rightValue === 'market:close';
    if (leftIsClose && rightOperand?.semantic === 'trend_level') {
      if (node.op === 'gte' || node.op === 'lte') {
        return {
          left: rightValue,
          op: node.op === 'gte' ? 'support' : 'resistance',
          rightType: 'semantic',
          rightIndicator: '',
          rightNumber: 0,
        };
      }
    }
    if (rightIsClose && leftOperand?.semantic === 'trend_level') {
      if (node.op === 'lte' || node.op === 'gte') {
        return {
          left: leftValue,
          op: node.op === 'lte' ? 'support' : 'resistance',
          rightType: 'semantic',
          rightIndicator: '',
          rightNumber: 0,
        };
      }
    }
    return null;
  }

  nodeToOperandValue(node) {
    if (!node) return '';
    if (node.kind === 'reference' && node.ref) {
      if (node.ref.scope === 'market') return `market:${node.ref.field || 'close'}`;
      if (node.ref.scope === 'position') return `position:${node.ref.field || 'avg_entry_price'}`;
      return `indicator:${node.ref.name || node.ref.id}:${node.ref.field || 'value'}`;
    }
    return '';
  }

  compileConditionItem(item) {
    this.normalizeConditionItem(item, this.getAvailableOperands());
    const leftArg = this.operandValueToNode(item.left);
    if (item.op === 'bullish_trend' || item.op === 'bearish_trend') {
      return {
        kind: 'operator',
        op: 'eq',
        args: [
          leftArg,
          { kind: 'number', number: item.op === 'bullish_trend' ? 1 : -1 }
        ]
      };
    }
    if (item.op === 'trend_turns_bullish' || item.op === 'trend_turns_bearish') {
      return {
        kind: 'operator',
        op: item.op === 'trend_turns_bullish' ? 'turn_bullish' : 'turn_bearish',
        args: [leftArg]
      };
    }
    if (item.op === 'support' || item.op === 'resistance') {
      return {
        kind: 'operator',
        op: item.op === 'support' ? 'gte' : 'lte',
        args: [
          this.operandValueToNode('market:close'),
          leftArg
        ]
      };
    }

    let rightArg;
    if (!this.isCrossOperator(item.op) && item.rightType === 'number') {
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
    if (parts[0] === 'position') {
      return {
        kind: 'reference',
        ref: { scope: 'position', name: 'current', field: parts[1] || 'avg_entry_price' }
      };
    }
    return {
      kind: 'reference',
      ref: {
        scope: 'indicator',
        name: parts[1],
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
    this.migrateLeverageConfig(content);

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
        quote_currency: 'USDT',
        exchange_leverage: 1
      },
      behavior: {
        allow_pyramiding: false,
        allow_same_bar_reversal: false,
        cooldown_bars: 0
      },
      position: {
        initial_size_pct: 0.25,
        max_additions: 0,
        position_leverage: 1,
        long_capital_decay_power: 0.5,
        short_capital_decay_power: 0.5
      },
      exit_policies: [],
      execution: {
        signal_time: 'bar_close',
        market_fill_time: 'next_bar_open',
        intrabar_conflict: 'conservative',
        fee: { maker_rate: 0.0002, taker_rate: 0.0005 },
        slippage: { type: 'bps', value: 2 }
      },
      risk: {
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
                  { kind: 'reference', ref: { scope: 'indicator', name: 'ema_fast_15m', field: 'value', offset: 0 } },
                  { kind: 'reference', ref: { scope: 'indicator', name: 'ema_slow_15m', field: 'value', offset: 0 } }
                ]
              },
              {
                kind: 'operator',
                op: 'gt',
                args: [
                  { kind: 'reference', ref: { scope: 'indicator', name: 'rsi_14_4h', field: 'value', offset: 0 } },
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
                  { kind: 'reference', ref: { scope: 'indicator', name: 'ema_fast_15m', field: 'value', offset: 0 } },
                  { kind: 'reference', ref: { scope: 'indicator', name: 'ema_slow_15m', field: 'value', offset: 0 } }
                ]
              },
              {
                kind: 'operator',
                op: 'gt',
                args: [
                  { kind: 'reference', ref: { scope: 'indicator', name: 'rsi_14_4h', field: 'value', offset: 0 } },
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
