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
                <h3 class="section-heading">杠杆设置 (LEVERAGE)</h3>
                <div class="visual-grid-cards" id="positionCards">
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
                  <label class="additions-switch" title="关闭后保留加仓规则配置，仅停止加仓。">
                    <input type="checkbox" id="formAllowAdditions">
                    <span>允许加仓</span>
                    <span class="section-badge" id="additionsSwitchStatus">已关闭</span>
                  </label>
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
    document.getElementById('formAllowAdditions').addEventListener('change', (e) => {
      if (!this.currentStrategy) return;
      const behavior = this.currentStrategy.behavior || (this.currentStrategy.behavior = {});
      behavior.allow_additions = e.target.checked;
      this.renderRulesEditor(this.currentStrategy);
      this.syncToJson();
    });

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
    if (mode === 'visual' && this.activeMode === 'json') {
      const area = document.getElementById('jsonEditorArea');
      try {
        const parsed = JSON.parse(area.value);
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          throw new Error('策略必须是 JSON 对象');
        }
        this.currentStrategy = parsed;
      } catch (err) {
        this.showAlert(`JSON 格式无效: ${err.message}`, 'error');
        return;
      }
    }
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

      this.renderVisualDetail(this.currentStrategy);
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
      await this.selectStrategy(this.strategies[0].id);
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
    try {
      this.migrateRuleSizing(def);
    } catch (err) {
      this.showAlert(`旧仓位配置无法转换：${err.message}，请在 JSON 源码中修正。`, 'error');
      return;
    }
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

  migrateRuleSizing(def) {
    const position = def.position || {};
    const behavior = def.behavior || {};
    const rules = def.rules || [];
    const hasLegacyOpen = rules.some(rule => rule.action?.type === 'open');
    const allowAdditions = behavior.allow_pyramiding || position.allow_pyramiding;
    const maxAdditions = position.max_additions ?? 0;
    if (hasLegacyOpen) {
      if (!Number.isSafeInteger(maxAdditions) || maxAdditions < 0) {
        throw new Error('最大加仓次数必须是非负整数');
      }
      for (const field of ['initial_size_pct', 'add_size_pct', 'add_size_multiplier']) {
        const value = position[field] ?? 0;
        if (!Number.isFinite(value) || value < 0 ||
            (field !== 'add_size_multiplier' && value > 1)) {
          throw new Error(`${field} 无效`);
        }
      }
      if (position.add_size_pct > 0 && position.add_size_multiplier > 0) {
        throw new Error('固定比例与倍增配置不能同时启用');
      }
      if (maxAdditions > 0 && !allowAdditions) {
        throw new Error('加仓次数与加仓开关不一致');
      }
      if (maxAdditions === 0 && (position.add_size_pct > 0 || position.add_size_multiplier > 0)) {
        throw new Error('加仓配置缺少适用次数');
      }
      const ids = new Set(rules.map(rule => rule.id));
      const converted = [];
      for (const rule of rules) {
        if (rule.action?.type !== 'open') {
          converted.push(rule);
          continue;
        }
        const sizePct = rule.action.size_pct || position.initial_size_pct || 0;
        if (!rule.disabled && (!Number.isFinite(sizePct) || sizePct <= 0 || sizePct > 1)) {
          throw new Error(`规则 ${rule.id} 缺少有效的开仓比例`);
        }
        const sourceID = rule.source_rule_id || rule.id;
        converted.push({
          ...this.cloneCondition(rule),
          source_rule_id: sourceID,
          action: { ...rule.action, type: 'enter', size_pct: sizePct },
        });
        for (let index = 1; allowAdditions && index <= maxAdditions; index++) {
          const baseID = `${rule.id}__add_${index}`;
          let id = baseID;
          for (let suffix = 2; ids.has(id); suffix++) id = `${baseID}_${suffix}`;
          ids.add(id);
          const action = { type: 'add', side: rule.action.side, addition_index: index };
          if (rule.action.size_pct > 0 || position.add_size_pct > 0) {
            action.size_pct = rule.action.size_pct || position.add_size_pct;
          } else {
            action.size_multiplier = position.add_size_multiplier > 0
              ? Math.pow(position.add_size_multiplier, index) : 1;
            if (!Number.isFinite(action.size_multiplier) || action.size_multiplier <= 0) {
              throw new Error(`规则 ${rule.id} 第 ${index} 次加仓倍数超出范围`);
            }
          }
          converted.push({
            ...this.cloneCondition(rule), id, source_rule_id: sourceID,
            name: `${rule.name || rule.id} · 第 ${index} 次加仓`, action,
          });
        }
      }
      // Commit only after every legacy rule has converted successfully.
      def.rules = converted;
    }
    for (const field of [
      'initial_size_pct', 'allow_pyramiding', 'max_additions',
      'add_size_pct', 'add_size_multiplier',
    ]) delete position[field];
    const migratedBehavior = def.behavior || (def.behavior = {});
    if (migratedBehavior.allow_additions === undefined || migratedBehavior.allow_additions === null) {
      migratedBehavior.allow_additions = (def.rules || [])
        .some(rule => rule.action?.type === 'add' && !rule.disabled);
    }
    migratedBehavior.allow_pyramiding = false;
    return def;
  }

  renderOverviewForm(def) {
    this.migrateLeverageConfig(def);
    document.getElementById('viewStrategyName').textContent = def.name || def.id;
    document.getElementById('viewStrategyId').textContent = def.id;
    document.getElementById('viewMarketType').textContent = (def.account && def.account.market_type) || 'spot';
    document.getElementById('viewStrategyDesc').textContent = def.description || '无策略描述说明';

    const overview = document.getElementById('overviewCards');
    const positionCards = document.getElementById('positionCards');
    const acc = def.account || (def.account = {});
    const data = def.data || {};
    const position = def.position || (def.position = {});
    const exchangeLeverage = acc.exchange_leverage || 1;
    const positionLeverage = position.position_leverage || 1;
    const isSpot = acc.market_type === 'spot';
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
    positionCards.innerHTML = `
      <div class="visual-card">
        <label class="vcard-label">交易所配置杠杆</label>
        <div class="input-with-unit">
          <input type="number" class="form-control" id="formExchangeLeverage"
            value="${exchangeLeverage}" min="1" step="0.1" ${isSpot ? 'disabled' : ''}>
          <span class="input-unit">x</span>
        </div>
      </div>
      <div class="visual-card">
        <label class="vcard-label">首次开仓杠杆</label>
        <div class="input-with-unit">
          <input type="number" class="form-control" id="formPositionLeverage"
            value="${positionLeverage}" min="1" max="${exchangeLeverage}" step="0.1" ${isSpot ? 'disabled' : ''}>
          <span class="input-unit">x</span>
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
      def.position.position_leverage = Math.min(def.position.position_leverage || 1, value);
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
    if (item.readOnly) return item;
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

  cloneCondition(node) {
    return node === undefined ? undefined : JSON.parse(JSON.stringify(node));
  }

  escapeHtml(value) {
    return String(value ?? '').replace(/[&<>"']/g, char => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[char]));
  }

  ruleSizeMode(action) {
    const size = action?.size_pct;
    if (action?.type === 'close') {
      return size === undefined || size === null || size === 0 || size === 1 ? 'all' : 'custom';
    }
    if (action?.type === 'add') {
      const modes = {
        equity_pct: 'custom',
        first_entry_multiple: 'multiplier',
        current_position_multiple: 'current_position_multiple',
        first_entry_geometric: 'first_entry_geometric',
      };
      if (action.sizing_mode) return modes[action.sizing_mode] || 'unsupported';
      if (action.size_multiplier > 0) return 'multiplier';
    }
    if (action?.type === 'open' && !size) return 'inherit';
    return 'custom';
  }

  setRuleSizeMode(rule, mode) {
    const action = rule.action;
    this.ruleSizeOverrides ||= new WeakMap();
    this.ruleMultiplierOverrides ||= new WeakMap();
    if (this.ruleSizeMode(action) === 'custom' && action.size_pct > 0) {
      this.ruleSizeOverrides.set(rule, action.size_pct);
    }
    if (action.size_multiplier > 0 && this.isMultiplierMode(this.ruleSizeMode(action))) {
      const values = this.ruleMultiplierOverrides.get(rule) || {};
      values[this.ruleSizeMode(action)] = action.size_multiplier;
      this.ruleMultiplierOverrides.set(rule, values);
    }
    if (mode === 'inherit' && action.type === 'open') {
      delete action.size_pct;
    } else if (mode === 'all' && action.type === 'close') {
      action.size_pct = 1;
    } else if (mode === 'custom') {
      delete action.size_multiplier;
      if (action.type === 'add') action.sizing_mode = 'equity_pct';
      else delete action.sizing_mode;
      action.size_pct = this.ruleSizeOverrides.get(rule) || 0.25;
    } else if (this.isMultiplierMode(mode) && action.type === 'add') {
      delete action.size_pct;
      action.sizing_mode = mode === 'multiplier' ? 'first_entry_multiple' : mode;
      action.size_multiplier = this.ruleMultiplierOverrides.get(rule)?.[mode] ??
        (mode === 'multiplier' ? 1 : 2);
    }
  }

  isMultiplierMode(mode) {
    return ['multiplier', 'current_position_multiple', 'first_entry_geometric'].includes(mode);
  }

  validGeometricSize(action) {
    if (this.ruleSizeMode(action) !== 'first_entry_geometric') return true;
    const multiplier = Math.pow(action.size_multiplier, action.addition_index);
    return Number.isFinite(multiplier) && multiplier > 0;
  }

  additionsEnabled(def = this.currentStrategy) {
    if (typeof def?.behavior?.allow_additions === 'boolean') return def.behavior.allow_additions;
    return (def?.rules || []).some(rule => rule.action?.type === 'add' && !rule.disabled) ||
      !!(def?.behavior?.allow_pyramiding || def?.position?.allow_pyramiding);
  }

  setRuleActionType(rule, type) {
    const previousMode = this.ruleSizeMode(rule.action);
    const previousType = rule.action.type;
    // Remember a custom amount before changing to an incompatible action.
    if (previousMode === 'custom') this.setRuleSizeMode(rule, 'custom');
    if (this.isMultiplierMode(previousMode)) this.setRuleSizeMode(rule, previousMode);
    if (previousMode === 'all' && type !== 'close') delete rule.action.size_pct;
    rule.action.type = type;
    if (type !== 'add') {
      delete rule.action.addition_index;
      delete rule.action.size_multiplier;
      delete rule.action.sizing_mode;
    } else if (!rule.action.addition_index) {
      const indexes = (this.currentStrategy.rules || [])
        .filter(other => other !== rule && other.action?.type === 'add' && other.action.side === rule.action.side)
        .map(other => other.action.addition_index || 0);
      rule.action.addition_index = Math.max(0, ...indexes) + 1;
    }
    this.setRuleSizeMode(rule, type === 'close' && previousType !== 'close'
      ? 'all' : (type === 'add'
        ? (previousType === 'add' ? previousMode : 'current_position_multiple') : 'custom'));
  }

  ruleSizeHint(action) {
    const mode = this.ruleSizeMode(action);
    if (mode === 'inherit') {
      return '旧版兼容开仓配置。进入可视化构建器时转换为首单与逐次加仓规则。';
    }
    if (mode === 'all') return '平掉该方向的全部持仓。';
    if (action.type === 'add') {
      if (mode === 'current_position_multiple') {
        return '名义金额 = 加仓前同向总持仓名义金额 × 本规则倍数；按成交时的持仓和价格计算，不重复乘首次开仓杠杆。';
      }
      if (mode === 'first_entry_geometric') {
        return `名义金额 = 本轮首单实际成交名义金额 × 本规则倍数的 n 次方；n = 本次加仓次数（${action.addition_index || '待配置'}），第一次从 1 开始。`;
      }
      if (mode === 'unsupported') return '该仓位计算模式暂不支持，请选择已支持的模式或在 JSON 中修正。';
      return mode === 'multiplier'
        ? '仅已有同向持仓时触发；名义金额 = 本轮首单实际成交名义金额 × 本规则倍数，不重复乘首次开仓杠杆。'
        : '仅已有同向持仓时触发；名义金额 = 当前权益 × 本规则比例，不重复乘首次开仓杠杆。';
    }
    return action.type === 'close'
      ? '比例以该方向当前持仓数量为基准。'
      : '仅空仓时触发；首单名义金额 = 当前权益 × 本规则比例 × 首次开仓杠杆。';
  }

  renderRulesEditor(def) {
    const rulesContainer = document.getElementById('rulesTreeContainer');
    const rules = def.rules || [];
    const allowAdditions = this.additionsEnabled(def);
    const allowSwitch = document.getElementById('formAllowAdditions');
    if (allowSwitch) allowSwitch.checked = allowAdditions;
    const switchStatus = document.getElementById('additionsSwitchStatus');
    if (switchStatus) switchStatus.textContent = allowAdditions ? '已开启' : '已关闭';
    document.getElementById('ruleCountBadge').textContent = `${rules.length} 组`;
    const operands = this.getAvailableOperands();

    rulesContainer.innerHTML = rules.map((rule, rIdx) => {
      const parsedWhen = this.parseAstWhen(rule.when);
      const action = rule.action || { type: 'enter', side: 'long' };
      const sizeMode = this.ruleSizeMode(action);
      const isCustomSize = sizeMode === 'custom';
      const additionDisabled = action.type === 'add' && !allowAdditions;
      const sizePct = isCustomSize && action.size_pct > 0
        ? Number((action.size_pct * 100).toPrecision(15)) : '';
      const usesConditionString = typeof rule.condition === 'string' && rule.condition.trim() !== '';

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
                <option value="enter" ${action.type === 'enter' ? 'selected' : ''}>首次开仓 (ENTER)</option>
                <option value="add" ${action.type === 'add' ? 'selected' : ''} ${allowAdditions ? '' : 'disabled'}>同向加仓 (ADD)</option>
                <option value="close" ${action.type === 'close' ? 'selected' : ''}>平仓 (CLOSE)</option>
                ${action.type === 'open' ? '<option value="open" selected>旧版开仓 (OPEN)</option>' : ''}
              </select>
              <select class="form-select rule-action-side" data-ridx="${rIdx}">
                <option value="long" ${action.side === 'long' ? 'selected' : ''}>多头 (LONG)</option>
                <option value="short" ${action.side === 'short' ? 'selected' : ''}>空头 (SHORT)</option>
              </select>
            </div>
            ${action.type === 'add' ? `
              <label class="rule-addition-index-wrap">
                <span class="panel-sub-label">适用次数:</span>
                第 <input type="number" class="form-control rule-addition-index" data-ridx="${rIdx}"
                  min="1" step="1" value="${action.addition_index || ''}" ${additionDisabled ? 'disabled' : ''}> 次加仓
              </label>
            ` : ''}
            <div class="slider-wrapper">
              <span class="panel-sub-label">仓位:</span>
              <select class="form-select rule-size-mode" data-ridx="${rIdx}" ${additionDisabled ? 'disabled' : ''}>
                ${action.type === 'close' ? `
                  <option value="all" ${sizeMode === 'all' ? 'selected' : ''}>全部平仓</option>
                  <option value="custom" ${isCustomSize ? 'selected' : ''}>按持仓比例平仓</option>
                ` : action.type === 'add' ? `
                  <option value="current_position_multiple" ${sizeMode === 'current_position_multiple' ? 'selected' : ''}>当前持仓倍增</option>
                  <option value="first_entry_geometric" ${sizeMode === 'first_entry_geometric' ? 'selected' : ''}>首仓层级倍增</option>
                  <option value="custom" ${isCustomSize ? 'selected' : ''}>当前权益比例</option>
                  <option value="multiplier" ${sizeMode === 'multiplier' ? 'selected' : ''}>首仓固定倍数</option>
                  ${sizeMode === 'unsupported' ? `<option selected disabled>${this.escapeHtml(action.sizing_mode)}</option>` : ''}
                ` : action.type === 'open' ? `
                  <option value="inherit" ${sizeMode === 'inherit' ? 'selected' : ''}>继承策略配置</option>
                  <option value="custom" ${isCustomSize ? 'selected' : ''}>自定义权益比例</option>
                ` : `
                  <option value="custom" selected>首单权益比例</option>
                `}
              </select>
              ${this.isMultiplierMode(sizeMode) ? `
                <div class="rule-size-number-wrap">
                  <input type="number" class="form-control rule-multiplier-input" data-ridx="${rIdx}"
                    min="0" step="any" value="${action.size_multiplier}" ${additionDisabled ? 'disabled' : ''}>
                  <span>x</span>
                </div>
              ` : `
              <input type="range" class="pct-slider rule-size-slider" data-ridx="${rIdx}" min="0" max="100" step="0.1"
                value="${isCustomSize ? sizePct : 0}" ${isCustomSize && !additionDisabled ? '' : 'disabled'}>
              <div class="rule-size-number-wrap">
                <input type="number" class="form-control rule-size-input" data-ridx="${rIdx}"
                  min="0" max="100" step="any" value="${sizePct}" ${isCustomSize && !additionDisabled ? '' : 'disabled'}>
                <span>%</span>
              </div>
              `}
            </div>
            <div class="rule-sizing-hint">${this.ruleSizeHint(action)}</div>
            ${additionDisabled ? '<div class="rule-sizing-hint additions-paused-hint">加仓已关闭，启用“允许加仓”后可选择模式；当前配置已保留。</div>' : ''}
          </div>

          <div class="conditions-builder" data-ridx="${rIdx}">
            ${usesConditionString ? `
              <div class="preserved-condition">
                <span class="panel-sub-label">触发条件 (WHEN):</span>
                <p>链式条件已完整保留，请在 JSON 源码中编辑。</p>
                <pre>${this.escapeHtml(rule.condition)}</pre>
                <button class="btn-xs btn-outline btn-edit-condition-json">编辑 JSON 条件</button>
              </div>
            ` : `
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
            `}
          </div>
        </div>
      `;
    }).join('');

    this.bindRulesEditorEvents(def);
  }

  renderConditionRowHtml(item, rIdx, cIdx, operands) {
    if (item.readOnly) {
      return `
        <div class="preserved-condition condition-readonly-row">
          <div>
            <p>${this.escapeHtml(item.reason)}，原条件已完整保留。</p>
            <pre>${this.escapeHtml(JSON.stringify(item.originalNode, null, 2) ?? '未配置条件')}</pre>
            <button class="btn-xs btn-outline btn-edit-condition-json">编辑 JSON 条件</button>
          </div>
          <button class="btn-delete-row" data-ridx="${rIdx}" data-cidx="${cIdx}" title="删除此条件">✕</button>
        </div>
      `;
    }
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
    const historicalRefs = (item.originalNode?.args || [])
      .filter(arg => arg.kind === 'reference' && arg.ref?.offset < 0)
      .map(arg => `${arg.ref.name}.${arg.ref.field}：前 ${-arg.ref.offset} 根基础 K 线`);
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
            <button class="btn-toggle-operand" data-ridx="${rIdx}" data-cidx="${cIdx}" title="切换右操作数模式 (常数/序列)">
              ${isNum ? '常数' : '序列'}
            </button>
          `}
        </div>

        <button class="btn-delete-row" data-ridx="${rIdx}" data-cidx="${cIdx}" title="删除此条件">✕</button>
        ${historicalRefs.length ? `<div class="condition-history-hint">${this.escapeHtml(historicalRefs.join('；'))}</div>` : ''}
      </div>
    `;
  }

  bindRulesEditorEvents(def) {
    const container = document.getElementById('rulesTreeContainer');
    const commitConditions = (rIdx, parsed) => {
      def.rules[rIdx].when = this.compileAstWhen(parsed.gate, parsed.items, parsed.sourceWhen);
      this.syncToJson();
    };
    container.querySelectorAll('.btn-edit-condition-json').forEach(btn => {
      btn.addEventListener('click', () => {
        this.switchMode('json');
        document.getElementById('jsonEditorArea')?.focus();
      });
    });

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
        const rule = def.rules[rIdx];
        if (e.target.value === 'add' && !this.additionsEnabled(def)) {
          e.target.value = rule.action.type;
          this.showAlert('请先开启交易规则区的“允许加仓”。', 'info');
          return;
        }
        this.setRuleActionType(rule, e.target.value);
        this.renderRulesEditor(def);
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

    container.querySelectorAll('.rule-size-mode').forEach(select => {
      select.addEventListener('change', () => {
        const rIdx = parseInt(select.dataset.ridx, 10);
        if (def.rules[rIdx].action.type === 'add' && !this.additionsEnabled(def)) return;
        const rule = def.rules[rIdx];
        const previous = this.cloneCondition(rule.action);
        this.setRuleSizeMode(rule, select.value);
        if (!this.validGeometricSize(rule.action)) {
          rule.action = previous;
          this.showAlert('倍数的加仓次数次方需为有限的正数，请调整倍数或次数。', 'error');
        }
        this.renderRulesEditor(def);
        this.syncToJson();
      });
    });

    container.querySelectorAll('.rule-addition-index').forEach(input => {
      input.addEventListener('change', () => {
        const rule = def.rules[Number(input.dataset.ridx)];
        if (!this.additionsEnabled(def)) return;
        const value = Number(input.value);
        if (!Number.isSafeInteger(value) || value < 1 ||
            !this.validGeometricSize({ ...rule.action, addition_index: value })) {
          input.value = rule.action.addition_index;
          this.showAlert('次数需为正整数，且倍数的该次数次方需为有限的正数。', 'error');
          return;
        }
        rule.action.addition_index = value;
        this.renderRulesEditor(def);
        this.syncToJson();
      });
    });
    container.querySelectorAll('.rule-multiplier-input').forEach(input => {
      input.addEventListener('change', () => {
        const rule = def.rules[Number(input.dataset.ridx)];
        if (!this.additionsEnabled(def)) return;
        const value = Number(input.value);
        if (!Number.isFinite(value) || value <= 0 ||
            !this.validGeometricSize({ ...rule.action, size_multiplier: value })) {
          input.value = rule.action.size_multiplier;
          this.showAlert('加仓倍数及按层级计算的结果必须是有限的正数。', 'error');
          return;
        }
        rule.action.size_multiplier = value;
        this.renderRulesEditor(def);
        this.syncToJson();
      });
    });

    const updateRuleSize = (rIdx, value) => {
      const rule = def.rules[rIdx];
      if (rule.action.type === 'add' && !this.additionsEnabled(def)) return;
      if (this.ruleSizeMode(rule.action) !== 'custom') return;
      const normalized = Number(value);
      const slider = container.querySelector(`.rule-size-slider[data-ridx="${rIdx}"]`);
      const input = container.querySelector(`.rule-size-input[data-ridx="${rIdx}"]`);
      if (!Number.isFinite(normalized) || normalized <= 0 || normalized > 100) {
        const previous = rule.action.size_pct * 100;
        if (slider) slider.value = previous;
        if (input) input.value = previous;
        this.showAlert('仓位比例需大于 0 且不超过 100%；全部平仓请使用平仓模式。', 'error');
        return;
      }
      if (slider) slider.value = normalized;
      if (input) input.value = normalized;
      rule.action.size_pct = normalized / 100;
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
        commitConditions(rIdx, parsed);
        this.renderRulesEditor(def);
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
          item.dirty = true;
          this.normalizeConditionItem(item, this.getAvailableOperands());
          commitConditions(rIdx, parsed);
          this.renderRulesEditor(def);
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
          item.dirty = true;
          this.normalizeConditionItem(item, this.getAvailableOperands());
          commitConditions(rIdx, parsed);
          this.renderRulesEditor(def);
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
          parsed.items[cIdx].dirty = true;
          commitConditions(rIdx, parsed);
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
          const value = Number(input.value);
          if (input.value.trim() === '' || !Number.isFinite(value)) return;
          parsed.items[cIdx].rightNumber = value;
          parsed.items[cIdx].dirty = true;
          commitConditions(rIdx, parsed);
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
          parsed.items[cIdx].dirty = true;
          this.normalizeConditionItem(parsed.items[cIdx], this.getAvailableOperands());
          commitConditions(rIdx, parsed);
          this.renderRulesEditor(def);
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
        commitConditions(rIdx, parsed);
        this.renderRulesEditor(def);
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
        commitConditions(rIdx, parsed);
        this.renderRulesEditor(def);
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
    const ids = new Set(this.currentStrategy.rules.map(rule => rule.id));
    let sequence = this.currentStrategy.rules.length + 1;
    while (ids.has(`rule_${sequence}`)) sequence++;
    const id = `rule_${sequence}`;
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
        type: 'enter',
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
    const sourceWhen = this.cloneCondition(when);
    if (
      when?.kind === 'operator' &&
      (when.op === 'and' || when.op === 'or') &&
      Array.isArray(when.args)
    ) {
      return {
        gate: when.op,
        items: when.args.map(arg => this.parseConditionItem(arg)),
        sourceWhen,
      };
    }
    return { gate: 'and', items: [this.parseConditionItem(when)], sourceWhen };
  }

  parseConditionItem(node) {
    const originalNode = this.cloneCondition(node);
    const preserved = reason => ({ readOnly: true, reason, originalNode });
    if (node?.kind === 'boolean') return preserved(node.bool ? '此条件始终满足' : '此条件始终不满足');
    if (!node || node.kind !== 'operator' || !Array.isArray(node.args)) {
      return preserved('此条件暂不支持可视化编辑');
    }
    if (['and', 'or', 'not'].includes(node.op)) {
      return preserved('嵌套逻辑条件请在 JSON 源码中编辑');
    }
    const semanticItem = this.parseSemanticCondition(node);
    if (semanticItem) return { ...semanticItem, originalNode };

    const op = node.op;
    const [leftNode, rightNode] = node.args;
    const operands = this.getAvailableOperands();
    if (node.args.length !== 2 || !['gt', 'gte', 'lt', 'lte', 'eq', 'cross_up', 'cross_down'].includes(op)) {
      return preserved('此操作条件暂不支持可视化编辑');
    }

    const leftVal = this.nodeToOperandValue(leftNode);
    if (!operands.some(operand => operand.value === leftVal)) {
      return preserved('持仓条件或未识别的引用请在 JSON 源码中编辑');
    }
    let rightType = 'indicator';
    let rightVal = '';
    let rightNum = 50.0;

    if (rightNode && rightNode.kind === 'number') {
      rightType = 'number';
      rightNum = rightNode.number ?? 0;
    } else if (rightNode) {
      rightVal = this.nodeToOperandValue(rightNode);
    }

    const item = {
      left: leftVal,
      op,
      rightType: rightType,
      rightIndicator: rightVal,
      rightNumber: rightNum,
      originalNode,
    };
    if (!this.operatorOptionsFor(item, operands).some(option => option.value === op)) {
      return preserved('此指标操作条件请在 JSON 源码中编辑');
    }
    if (rightType === 'indicator') {
      const candidates = this.isCrossOperator(op)
        ? this.compatibleCrossOperands(leftVal, operands)
        : this.compatibleComparisonOperands(leftVal, operands);
      if (!candidates.some(operand => operand.value === rightVal)) {
        return preserved('此引用组合请在 JSON 源码中编辑');
      }
    } else if (!Number.isFinite(rightNum) || this.isCrossOperator(op)) {
      return preserved('此数值操作条件请在 JSON 源码中编辑');
    }
    return item;
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
      if (node.ref.scope === 'market') return `market:${node.ref.field}`;
      if (node.ref.scope === 'position') return `position:${node.ref.field}`;
      if (node.ref.scope === 'indicator') return `indicator:${node.ref.name}:${node.ref.field}`;
    }
    return '';
  }

  conditionOperandNode(value, item, argIndex) {
    const args = item.originalNode?.args || [];
    const preferred = args[argIndex];
    if (this.nodeToOperandValue(preferred) === value) return this.cloneCondition(preferred);
    const original = args.find(arg => this.nodeToOperandValue(arg) === value);
    return original ? this.cloneCondition(original) : this.operandValueToNode(value);
  }

  compileConditionItem(item) {
    if (item.readOnly || (!item.dirty && Object.hasOwn(item, 'originalNode'))) {
      return this.cloneCondition(item.originalNode);
    }
    const leftArg = this.conditionOperandNode(item.left, item, 0);
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
          this.conditionOperandNode('market:close', item, 0),
          leftArg
        ]
      };
    }

    let rightArg;
    if (!this.isCrossOperator(item.op) && item.rightType === 'number') {
      rightArg = { kind: 'number', number: parseFloat(item.rightNumber) || 0 };
    } else {
      rightArg = this.conditionOperandNode(item.rightIndicator, item, 1);
    }
    return {
      ...this.cloneCondition(item.originalNode),
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
      return { kind: 'reference', ref: { scope: 'position', name: 'current', field: parts[1] } };
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

  compileAstWhen(gate, items, sourceWhen) {
    const args = (items || []).map(item => this.compileConditionItem(item));
    const originalItems = sourceWhen?.kind === 'operator' && ['and', 'or'].includes(sourceWhen.op)
      ? sourceWhen.args
      : [sourceWhen];
    if (
      JSON.stringify(args) === JSON.stringify(originalItems) &&
      (sourceWhen?.op === gate || !['and', 'or'].includes(sourceWhen?.op))
    ) {
      return this.cloneCondition(sourceWhen);
    }
    if (!items || items.length === 0) {
      return { kind: 'boolean', bool: false };
    }
    if (items.length === 1) {
      return this.compileConditionItem(items[0]);
    }
    return {
      ...(sourceWhen?.kind === 'operator' && ['and', 'or'].includes(sourceWhen.op)
        ? this.cloneCondition(sourceWhen) : {}),
      kind: 'operator',
      op: gate || 'and',
      args,
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
      this.migrateRuleSizing(content);
      this.migrateLeverageConfig(content);
      this.showAlert('正在向 Go 后端提交静态校验与持久化...', 'info');
      const res = await Api.saveStrategy(content);
      this.showAlert(`策略 [${content.id}] 校验通过并保存成功！`, 'success');

      // 刷新列表并通知行情回测页刷新策略下拉框
      await this.loadStrategies();
      await this.selectStrategy(content.id);
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
        allow_additions: false,
        allow_pyramiding: false,
        allow_same_bar_reversal: false,
        cooldown_bars: 0
      },
      position: {
        position_leverage: 1
      },
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
            type: 'enter',
            side: 'long',
            size_pct: 0.25
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
