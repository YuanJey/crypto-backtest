/**
 * QUANTIX - Market Data API Client
 */

const Api = {
  baseUrl: window.location.origin.startsWith('http') ? window.location.origin : 'http://localhost:8080',

  /**
   * 获取多周期 K 线数据
   */
  async fetchMarketBars(symbol = 'BTC-USDT', datasetID = '', multi = true) {
    const url = new URL(`${this.baseUrl}/api/v1/market/bars`);
    url.searchParams.set('symbol', symbol);
    if (datasetID) url.searchParams.set('dataset_id', datasetID);
    if (multi) url.searchParams.set('multi', 'true');

    try {
      const res = await fetch(url.toString());
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: ${await res.text()}`);
      }
      return await res.json();
    } catch (err) {
      console.warn('API request failed, generating client-side fallback data:', err);
      return this.generateFallbackBars(symbol);
    }
  },

  /**
   * 客户端确定性仿真数据（保证在断网或本地静态单页时也能即刻体验）
   */
  generateFallbackBars(symbol) {
    const total = 672; // 7 days of 15m
    const baseMS = 1735689600000;
    const stepMS = 15 * 60 * 1000;
    const bars15m = [];
    let price = 93500.0;

    for (let i = 0; i < total; i++) {
      const t = baseMS + i * stepMS;
      const phase = i * 0.08;
      const harmonic = Math.sin(phase) * 180.0 + Math.cos(phase * 0.35) * 350.0 + Math.sin(i * 0.015) * 600.0;
      const drift = i * 6.5;

      const open = price;
      const noise = Math.sin(i * 13) * 45.0 + Math.cos(i * 7) * 30.0;
      const close = 93500.0 + harmonic + drift + noise;
      const high = Math.max(open, close) + Math.abs(Math.sin(i * 17)) * 75.0 + 20.0;
      const low = Math.min(open, close) - Math.abs(Math.cos(i * 23)) * 75.0 - 20.0;
      const volume = 120.0 + Math.abs(Math.sin(i)) * 250.0 + Math.abs(close - open) * 0.8;

      bars15m.push({
        open_time_ms: t,
        open: Math.round(open * 100) / 100,
        high: Math.round(high * 100) / 100,
        low: Math.round(low * 100) / 100,
        close: Math.round(close * 100) / 100,
        volume: Math.round(volume * 100) / 100,
        quote_volume: Math.round(volume * close * 100) / 100,
        trade_count: 1500 + (i % 50) * 30,
        state: 'closed'
      });
      price = close;
    }

    return {
      symbol: symbol,
      dataset_id: 'sample-client',
      base_timeframe: '15m',
      timeframes: {
        '15m': bars15m
      }
    };
  },

  /**
   * 获取系统能力指标目录
   */
  async fetchCatalog() {
    try {
      const res = await fetch(`${this.baseUrl}/api/v1/catalog`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      return await res.json();
    } catch (err) {
      console.warn('Failed to fetch catalog from API, returning builtin catalog fallback:', err);
      return {
        indicators: [
          { type: 'ema', label: 'EMA', category: 'trend', params: [{ name: 'period', type: 'int', required: true }] },
          { type: 'sma', label: 'SMA', category: 'trend', params: [{ name: 'period', type: 'int', required: true }] },
          { type: 'rsi', label: 'RSI', category: 'momentum', params: [{ name: 'period', type: 'int', required: true }] },
          { type: 'atr', label: 'ATR', category: 'volatility', params: [{ name: 'period', type: 'int', required: true }] },
          { type: 'boll', label: 'BOLL', category: 'volatility', params: [{ name: 'period', type: 'int', required: true }, { name: 'stddev', type: 'float', required: true }] },
        ]
      };
    }
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
  async runBacktest(strategyId, reportSpec = {}) {
    const res = await fetch(`${this.baseUrl}/api/v1/backtests`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ strategy_id: strategyId, report: reportSpec }),
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

