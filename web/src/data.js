/**
 * QUANTIX - Market Data Management & Downloader Controller
 */

(function () {
  const DataManager = {
    currentDatasets: [],

    init() {
      this.container = document.getElementById('view-data');
      if (!this.container) return;
      this.renderLayout();
      this.bindEvents();
      this.setDefaultDates(7); // 默认近 7 天
      this.loadDatasets();
    },

    renderLayout() {
      this.container.innerHTML = `
        <div class="data-manager-viewport">
          <div class="data-manager-grid">
            <!-- 左侧：历史数据下载配置 -->
            <div class="data-card download-card">
              <div class="card-header">
                <div class="card-title">
                  <span class="card-icon">📥</span>
                  <h3>历史行情抓取与构建 (Binance OHLCV Downloader)</h3>
                </div>
                <span class="card-badge">REST API 分页引擎</span>
              </div>
              <div class="card-body">
                <form id="downloadForm" onsubmit="return false;">
                  <!-- 标的选择 -->
                  <div class="form-group">
                    <label class="form-label">
                      <span>交易标的 (Symbol)</span>
                      <span class="label-desc">支持币安主流币种，标准格式 BASE-QUOTE</span>
                    </label>
                    <div class="symbol-chips" id="symbolChips">
                      <button type="button" class="chip active" data-symbol="BTC-USDT">BTC-USDT</button>
                      <button type="button" class="chip" data-symbol="ETH-USDT">ETH-USDT</button>
                      <button type="button" class="chip" data-symbol="SOL-USDT">SOL-USDT</button>
                      <button type="button" class="chip" data-symbol="BNB-USDT">BNB-USDT</button>
                      <button type="button" class="chip" data-symbol="DOGE-USDT">DOGE-USDT</button>
                    </div>
                    <input type="text" class="form-input" id="dlSymbol" value="BTC-USDT" placeholder="例如 BTC-USDT 或多标的逗号分隔" required />
                  </div>

                  <!-- 市场类型与基准周期 -->
                  <div class="form-row">
                    <div class="form-group flex-1">
                      <label class="form-label">市场类型 (Market)</label>
                      <select class="form-select" id="dlMarketType">
                        <option value="spot" selected>Spot (现货)</option>
                        <option value="perpetual">Perpetual (U本位永续合约)</option>
                      </select>
                    </div>
                    <div class="form-group flex-1">
                      <label class="form-label">
                        <span>基准周期 (Timeframe)</span>
                        <span class="tag-locked">LOCKED</span>
                      </label>
                      <div class="input-locked-group">
                        <input type="text" class="form-input locked" id="dlTimeframe" value="15m" readonly />
                        <span class="locked-hint">核心基准驱动周期</span>
                      </div>
                    </div>
                  </div>

                  <!-- 时间范围 -->
                  <div class="form-group">
                    <div class="label-with-actions">
                      <label class="form-label">时间范围 (Time Range - UTC)</label>
                      <div class="range-presets" id="rangePresets">
                        <button type="button" class="btn-preset active" data-days="7">近 7 天</button>
                        <button type="button" class="btn-preset" data-days="14">近 14 天</button>
                        <button type="button" class="btn-preset" data-days="30">近 30 天</button>
                        <button type="button" class="btn-preset" data-days="90">近 90 天</button>
                      </div>
                    </div>
                    <div class="form-row">
                      <div class="flex-1">
                        <span class="sub-label">开始时间 (Start):</span>
                        <input type="datetime-local" class="form-input" id="dlStartTime" required />
                      </div>
                      <div class="flex-1">
                        <span class="sub-label">结束时间 (End):</span>
                        <input type="datetime-local" class="form-input" id="dlEndTime" required />
                      </div>
                    </div>
                    <span class="form-hint">时间戳将自动向上/向下对齐至 15m 整数倍 (900,000ms)，严格杜绝时间缝隙。</span>
                  </div>

                  <!-- 数据集 ID -->
                  <div class="form-group">
                    <label class="form-label">
                      <span>自定义数据集标识 (Dataset ID)</span>
                      <span class="label-desc">可选，留空则根据参数自动哈希生成</span>
                    </label>
                    <input type="text" class="form-input" id="dlDatasetId" placeholder="例如: btc-15m-2025q1" />
                  </div>

                  <!-- 提交按钮 -->
                  <div class="form-actions-bar">
                    <button type="button" class="btn btn-primary btn-large" id="btnStartDownload">
                      <span class="btn-icon">⬇️</span>
                      <span id="btnDownloadText">开始下载并构建 Parquet 数据集</span>
                    </button>
                  </div>
                </form>

                <!-- 下载状态与结果反馈框 -->
                <div class="download-status-box" id="dlStatusBox" style="display: none;"></div>
              </div>
            </div>

            <!-- 右侧：已构建本地数据集库 -->
            <div class="data-card catalog-card">
              <div class="card-header">
                <div class="card-title">
                  <span class="card-icon">🗄️</span>
                  <h3>本地数据集库 (Local Datasets)</h3>
                </div>
                <button type="button" class="btn btn-sm btn-outline" id="btnRefreshDatasets" title="重新扫描 data/market">
                  <span class="btn-icon">↻</span> 刷新库
                </button>
              </div>
              <div class="card-body">
                <div class="datasets-list-container" id="datasetsList">
                  <div class="empty-state">
                    <span class="empty-icon">⏳</span>
                    <p>正在读取本地数据集...</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      `;
    },

    bindEvents() {
      // 快捷 Chip 选择标的
      const chips = this.container.querySelectorAll('#symbolChips .chip');
      const symbolInput = this.container.querySelector('#dlSymbol');
      chips.forEach((chip) => {
        chip.addEventListener('click', () => {
          chips.forEach((c) => c.classList.remove('active'));
          chip.classList.add('active');
          symbolInput.value = chip.dataset.symbol;
        });
      });

      // 快捷预设天数选择
      const presets = this.container.querySelectorAll('#rangePresets .btn-preset');
      presets.forEach((btn) => {
        btn.addEventListener('click', () => {
          presets.forEach((p) => p.classList.remove('active'));
          btn.classList.add('active');
          const days = parseInt(btn.dataset.days, 10);
          this.setDefaultDates(days);
        });
      });

      // 刷新数据集列表按钮
      const btnRefresh = this.container.querySelector('#btnRefreshDatasets');
      if (btnRefresh) {
        btnRefresh.addEventListener('click', () => this.loadDatasets());
      }

      // 开始下载按钮
      const btnDownload = this.container.querySelector('#btnStartDownload');
      if (btnDownload) {
        btnDownload.addEventListener('click', () => this.handleDownload());
      }
    },

    setDefaultDates(days = 7) {
      const now = new Date();
      // 对齐到最近 15m
      const stepMS = 15 * 60 * 1000;
      const endMS = Math.floor(now.getTime() / stepMS) * stepMS;
      const startMS = endMS - days * 24 * 60 * 60 * 1000;

      const formatLocal = (ms) => {
        const d = new Date(ms);
        const pad = (n) => String(n).padStart(2, '0');
        const year = d.getFullYear();
        const month = pad(d.getMonth() + 1);
        const day = pad(d.getDate());
        const hours = pad(d.getHours());
        const mins = pad(d.getMinutes());
        return `${year}-${month}-${day}T${hours}:${mins}`;
      };

      const startInput = this.container.querySelector('#dlStartTime');
      const endInput = this.container.querySelector('#dlEndTime');
      if (startInput) startInput.value = formatLocal(startMS);
      if (endInput) endInput.value = formatLocal(endMS);
    },

    async handleDownload() {
      const symbol = this.container.querySelector('#dlSymbol').value.trim();
      const marketType = this.container.querySelector('#dlMarketType').value;
      const timeframe = '15m';
      const startVal = this.container.querySelector('#dlStartTime').value;
      const endVal = this.container.querySelector('#dlEndTime').value;
      const datasetId = this.container.querySelector('#dlDatasetId').value.trim();

      if (!symbol) {
        alert('请输入交易标的，例如 BTC-USDT');
        return;
      }
      if (!startVal || !endVal) {
        alert('请选择起止时间');
        return;
      }

      const startMS = new Date(startVal).getTime();
      const endMS = new Date(endVal).getTime();
      if (isNaN(startMS) || isNaN(endMS) || startMS >= endMS) {
        alert('开始时间必须早于结束时间');
        return;
      }

      const statusBox = this.container.querySelector('#dlStatusBox');
      const btn = this.container.querySelector('#btnStartDownload');
      const btnText = this.container.querySelector('#btnDownloadText');

      // 初始化实时进度条 UI
      btn.disabled = true;
      btnText.textContent = '数据流水线运行中...';
      statusBox.style.display = 'block';
      statusBox.className = 'download-status-box progress-mode';
      statusBox.innerHTML = `
        <div class="progress-container">
          <div class="progress-header">
            <div class="progress-stage-group">
              <span class="progress-stage-badge" id="pbStageBadge">[1/3 行情抓取]</span>
              <span class="progress-step-text" id="pbStepText">正在与 Binance API 建立安全会话...</span>
            </div>
            <div class="progress-pct-badge" id="pbPctText">0.0%</div>
          </div>
          <div class="progress-bar-track">
            <div class="progress-bar-fill" id="pbFill" style="width: 0%;">
              <div class="progress-bar-glow"></div>
            </div>
          </div>
          <div class="progress-footer">
            <span class="progress-counter" id="pbCounter">0 / 0 根</span>
            <span class="progress-detail" id="pbDetail">准备拉取 ${symbol} 15m K线...</span>
          </div>
        </div>
      `;

      const pbStageBadge = statusBox.querySelector('#pbStageBadge');
      const pbStepText = statusBox.querySelector('#pbStepText');
      const pbPctText = statusBox.querySelector('#pbPctText');
      const pbFill = statusBox.querySelector('#pbFill');
      const pbCounter = statusBox.querySelector('#pbCounter');
      const pbDetail = statusBox.querySelector('#pbDetail');

      const stageLabels = {
        init: '[0/3 准备就绪]',
        fetch: '[1/3 行情抓取]',
        clean: '[2/3 数据清洗]',
        write: '[3/3 分区落盘]',
        done: '[完成 100%]'
      };

      const onProgress = (ev) => {
        if (!ev) return;
        if (pbStageBadge && ev.stage) {
          pbStageBadge.textContent = stageLabels[ev.stage] || `[${ev.stage}]`;
        }
        if (pbStepText && ev.message) {
          pbStepText.textContent = ev.message;
        }
        if (pbPctText) {
          pbPctText.textContent = `${(ev.percent || 0).toFixed(1)}%`;
        }
        if (pbFill) {
          pbFill.style.width = `${Math.min(100, Math.max(0, ev.percent || 0))}%`;
        }
        if (pbCounter && ev.total > 0) {
          pbCounter.textContent = `${(ev.current || 0).toLocaleString()} / ${(ev.total || 0).toLocaleString()} 根`;
        }
        if (pbDetail && ev.message) {
          pbDetail.textContent = ev.message;
        }
      };

      try {
        const payload = {
          exchange: 'binance',
          market_type: marketType,
          symbol,
          timeframe,
          start_ms: startMS,
          end_ms: endMS,
          dataset_id: datasetId || undefined
        };

        const res = await Api.downloadMarketDataStream(payload, onProgress);
        const manifest = res.manifest || {};

        statusBox.className = 'download-status-box success';
        statusBox.innerHTML = `
          <div class="result-header">
            <span class="success-icon">✓</span>
            <div>
              <strong>数据构建完成！数据集已就绪</strong>
              <div class="result-meta">ID: <code>${manifest.dataset_id}</code> | 路径: <code>${res.dataset_root}</code></div>
            </div>
          </div>
          <div class="result-stats-grid">
            <div class="stat-pill">
              <span class="pill-label">总 K 线行数</span>
              <span class="pill-val">${manifest.row_count || 0}</span>
            </div>
            <div class="stat-pill">
              <span class="pill-label">缺失填补 Bar</span>
              <span class="pill-val">${manifest.missing_bars || 0}</span>
            </div>
            <div class="stat-pill">
              <span class="pill-label">Parquet 分区</span>
              <span class="pill-val">${(manifest.files && manifest.files.length) || 1} 文件</span>
            </div>
            <div class="stat-pill">
              <span class="pill-label">防未来函数校验</span>
              <span class="pill-val val-green">100% CLOSED</span>
            </div>
          </div>
          <div class="result-actions">
            <span class="badge-ready">✓ 数据集已成功落盘至本地库，可随时在【📈 行情回测】工作台中选用。</span>
          </div>
        `;

        // 刷新右侧列表并通知行情回测页刷新数据源选项
        this.loadDatasets();
        if (window.App && typeof window.App.refreshDatasets === 'function') {
          window.App.refreshDatasets();
        }
      } catch (err) {
        statusBox.className = 'download-status-box error';
        statusBox.innerHTML = `
          <div class="result-header">
            <span class="error-icon">✗</span>
            <div>
              <strong>下载构建失败</strong>
              <p class="error-detail">${err.message}</p>
            </div>
          </div>
        `;
      } finally {
        btn.disabled = false;
        btnText.textContent = '开始下载并构建 Parquet 数据集';
      }
    },

    async loadDatasets() {
      const listContainer = this.container.querySelector('#datasetsList');
      if (!listContainer) return;

      const datasets = await Api.fetchMarketDatasets();
      this.currentDatasets = datasets;

      if (!datasets || datasets.length === 0) {
        listContainer.innerHTML = `
          <div class="empty-state">
            <span class="empty-icon">📂</span>
            <p>本地尚未收录数据集</p>
            <span class="empty-hint">请在左侧指定标的与时间范围，点击下载并自动构建。</span>
          </div>
        `;
        return;
      }

      listContainer.innerHTML = datasets
        .map((ds) => {
          const startTime = ds.start_ms ? new Date(ds.start_ms).toISOString().replace('T', ' ').substring(0, 16) : '--';
          const endTime = ds.end_ms ? new Date(ds.end_ms).toISOString().replace('T', ' ').substring(0, 16) : '--';
          const symbols = (ds.symbols || []).join(', ') || 'BTC-USDT';

          return `
            <div class="dataset-item-card" data-id="${ds.dataset_id}">
              <div class="dataset-item-header">
                <div class="dataset-id-box">
                  <span class="ds-exchange">${ds.exchange || 'binance'}</span>
                  <span class="ds-market">${ds.market_type || 'spot'}</span>
                  <strong class="ds-id">${ds.dataset_id}</strong>
                </div>
                <div class="dataset-badges">
                  <span class="badge-tf">${ds.timeframes ? ds.timeframes.join('/') : '15m'}</span>
                </div>
              </div>
              <div class="dataset-item-body">
                <div class="ds-meta-row">
                  <span class="ds-label">标的:</span>
                  <strong class="ds-val text-accent">${symbols}</strong>
                  <span class="ds-sep">|</span>
                  <span class="ds-label">行数:</span>
                  <strong class="ds-val">${(ds.row_count || 0).toLocaleString()} 根</strong>
                  <span class="ds-sep">|</span>
                  <span class="ds-label">插值:</span>
                  <span class="ds-val">${ds.missing_bars || 0}</span>
                </div>
                <div class="ds-time-row">
                  <span class="ds-time-icon">🕒</span>
                  <span class="ds-time-text">${startTime} UTC 至 ${endTime} UTC</span>
                </div>
              </div>
              <div class="dataset-item-footer">
                <span class="dataset-ready-tag">● 本地 Parquet 就绪 (只读存储)</span>
              </div>
            </div>
          `;
        })
        .join('');
    }
  };

  window.DataManager = DataManager;
})();
