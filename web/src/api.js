/**
 * QUANTIX - Market Data API Client
 */

const Api = {
  baseUrl: window.location.origin.startsWith('http') ? window.location.origin : 'http://localhost:8080',

  async request(path, body) {
    const res = await fetch(`${this.baseUrl}/api/v1${path}`, body === undefined ? {} : {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const text = await res.text();
    let data;
    try { data = JSON.parse(text); } catch { data = { error: text }; }
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
    return data;
  },

  fetchStrategySnapshot(id, revision = 0) {
    return this.request(`/strategies/${encodeURIComponent(id)}/snapshot${revision ? `?revision=${revision}` : ''}`);
  },
  startRun(request) { return this.request('/backtest-runs', request); },
  fetchRun(id) { return this.request(`/backtest-runs/${encodeURIComponent(id)}`); },
  fetchRunResult(id) { return this.request(`/backtest-runs/${encodeURIComponent(id)}/result`); },
  fetchRunSnapshot(id) { return this.request(`/backtest-runs/${encodeURIComponent(id)}/snapshot`); },
  fetchRunEvents(id) { return this.request(`/backtest-runs/${encodeURIComponent(id)}/events`); },
  cancelRun(id) { return this.request(`/backtest-runs/${encodeURIComponent(id)}/cancel`, {}); },
  replayRun(id) { return this.request(`/backtest-runs/${encodeURIComponent(id)}/replay`, {}); },
  async fetchRuns(strategyId = '') {
    const data = await this.request(`/backtest-runs${strategyId ? `?strategy_id=${encodeURIComponent(strategyId)}` : ''}`);
    return data.runs || [];
  },

  /**
   * 获取多周期 K 线数据
   */
  async fetchMarketBars(symbol = 'BTC-USDT', datasetID = '', multi = true) {
    const url = new URL(`${this.baseUrl}/api/v1/market/bars`);
    url.searchParams.set('symbol', symbol);
    if (datasetID) url.searchParams.set('dataset_id', datasetID);
    if (multi) url.searchParams.set('multi', 'true');

    const res = await fetch(url.toString());
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}: ${await res.text()}`);
    }
    return await res.json();
  },

  /**
   * 获取系统能力指标目录
   */
  async fetchCatalog() {
    const res = await fetch(`${this.baseUrl}/api/v1/catalog`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return await res.json();
  },

  /**
   * 获取策略列表
   */
  async fetchStrategies() {
    try {
      const res = await fetch(`${this.baseUrl}/api/v1/strategies`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      return data.strategies || [];
    } catch (err) {
      console.warn('Failed to fetch strategies:', err);
      return [];
    }
  },

  /**
   * 获取单个策略详情
   */
  async fetchStrategyById(id) {
    try {
      const res = await fetch(`${this.baseUrl}/api/v1/strategies/${encodeURIComponent(id)}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      return await res.json();
    } catch (err) {
      console.warn(`Failed to fetch strategy ${id}:`, err);
      return null;
    }
  },

  /**
   * 保存或更新策略
   */
  async saveStrategy(strategyDef) {
    const res = await fetch(`${this.baseUrl}/api/v1/strategies`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: typeof strategyDef === 'string' ? strategyDef : JSON.stringify(strategyDef),
    });
    const text = await res.text();
    let data;
    try {
      data = JSON.parse(text);
    } catch {
      data = { error: text };
    }
    if (!res.ok) {
      throw new Error(data.error || `HTTP ${res.status}`);
    }
    return data;
  },

  /**
   * 提交回测任务
   */
  async runBacktest(strategyId, request = {}) {
    const res = await fetch(`${this.baseUrl}/api/v1/backtests`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ strategy_id: strategyId, ...request }),
    });
    const data = await res.json();
    if (!res.ok) {
      throw new Error(data.error || `HTTP ${res.status}`);
    }
    return data;
  },

  /**
   * 获取本地所有已下载数据集列表
   */
  async fetchMarketDatasets() {
    try {
      const res = await fetch(`${this.baseUrl}/api/v1/market/datasets`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      return data.datasets || [];
    } catch (err) {
      console.warn('Failed to fetch datasets:', err);
      return [];
    }
  },

  /**
   * 触发下载历史数据并构建 Parquet 数据集 (普通单次请求)
   */
  async downloadMarketData(payload) {
    const res = await fetch(`${this.baseUrl}/api/v1/market/download`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    const text = await res.text();
    let data;
    try {
      data = JSON.parse(text);
    } catch {
      data = { error: text };
    }
    if (!res.ok) {
      throw new Error(data.error || `HTTP ${res.status}`);
    }
    return data;
  },

  /**
   * 触发流式下载并通过 SSE 实时推送进度 (带实时进度条支持)
   */
  async downloadMarketDataStream(payload, onProgress) {
    const url = new URL(`${this.baseUrl}/api/v1/market/download?stream=true`);
    const res = await fetch(url.toString(), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'text/event-stream'
      },
      body: JSON.stringify({ ...payload, stream: true }),
    });

    if (!res.ok) {
      const text = await res.text();
      let errData;
      try {
        errData = JSON.parse(text);
      } catch {
        errData = { error: text };
      }
      throw new Error(errData.error || `HTTP ${res.status}`);
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder('utf-8');
    let buffer = '';
    let finalResult = null;

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop(); // 保留不完整分块

      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed || !trimmed.startsWith('data:')) continue;
        const jsonStr = trimmed.substring(5).trim();
        if (!jsonStr) continue;
        try {
          const ev = JSON.parse(jsonStr);
          if (ev.stage === 'error') {
            throw new Error(ev.message || '下载流水线异常中断');
          }
          if (typeof onProgress === 'function') {
            onProgress(ev);
          }
          if (ev.stage === 'done' && ev.result) {
            finalResult = ev.result;
          }
        } catch (e) {
          if (e.message && e.message.includes('下载流水线异常中断')) throw e;
          console.warn('Parse SSE packet error:', e, jsonStr);
        }
      }
    }

    if (!finalResult) {
      throw new Error('未接收到完整数据集构建报告');
    }
    return finalResult;
  }
};

window.Api = Api;
