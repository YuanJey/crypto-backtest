/**
 * QUANTIX - Multi-Timeframe Replay Engine
 * Strictly follows Anti-Lookahead semantics:
 * Advances tick-by-tick at 15m base resolution.
 * Higher timeframes (1h, 4h, 1d) are aggregated dynamically.
 * Unfinished periods are tagged as 'forming'; completed periods as 'closed'.
 */

class ReplayEngine {
  constructor(options = {}) {
    this.baseBars = []; // 全量 15m K线序列
    this.currentIndex = 0; // 当前回放时刻索引
    this.isPlaying = false;
    this.speed = 1.0; // 回放倍速基准
    this.baseIntervalMs = 600; // 1x 速度下每步间隔 (毫秒)
    this.timerId = null;

    // 回调通知
    this.onTick = options.onTick || null;
    this.onStateChange = options.onStateChange || null;

    this.timeframeSpecs = [
      { name: '1h', ms: 60 * 60 * 1000, baseRatio: 4 },
      { name: '4h', ms: 4 * 60 * 60 * 1000, baseRatio: 16 },
      { name: '1d', ms: 24 * 60 * 60 * 1000, baseRatio: 96 }
    ];
  }

  loadData(baseBars) {
    this.pause();
    this.baseBars = (baseBars || []).slice().sort((a, b) => a.open_time_ms - b.open_time_ms);
    // 初始展示前 80 根 K 线（若数据不足则全部）
    this.currentIndex = Math.min(this.baseBars.length - 1, Math.max(0, Math.min(80, this.baseBars.length - 1)));
    this.emitTick();
    this.notifyState();
  }

  play() {
    if (this.isPlaying || this.baseBars.length === 0) return;
    this.isPlaying = true;
    this.notifyState();
    this.scheduleNextTick();
  }

  pause() {
    if (!this.isPlaying) return;
    this.isPlaying = false;
    if (this.timerId) {
      clearTimeout(this.timerId);
      this.timerId = null;
    }
    this.notifyState();
  }

  togglePlay() {
    if (this.isPlaying) {
      this.pause();
    } else {
      this.play();
    }
  }

  setSpeed(speed) {
    this.speed = Math.max(0.1, speed);
    this.notifyState();
    if (this.isPlaying) {
      clearTimeout(this.timerId);
      this.scheduleNextTick();
    }
  }

  stepForward() {
    this.pause();
    if (this.currentIndex < this.baseBars.length - 1) {
      this.currentIndex++;
      this.emitTick();
      this.notifyState();
    }
  }

  stepBackward() {
    this.pause();
    if (this.currentIndex > 0) {
      this.currentIndex--;
      this.emitTick();
      this.notifyState();
    }
  }

  seek(index) {
    const target = Math.max(0, Math.min(this.baseBars.length - 1, Math.round(index)));
    this.currentIndex = target;
    this.emitTick();
    this.notifyState();
  }

  scheduleNextTick() {
    if (!this.isPlaying) return;
    const interval = Math.max(30, this.baseIntervalMs / this.speed);
    this.timerId = setTimeout(() => {
      if (this.currentIndex < this.baseBars.length - 1) {
        this.currentIndex++;
        this.emitTick();
        this.notifyState();
        this.scheduleNextTick();
      } else {
        // 到达末尾自动暂停
        this.pause();
      }
    }, interval);
  }

  /**
   * 动态多周期聚合计算（消除未来函数）
   */
  emitTick() {
    if (this.baseBars.length === 0) return;
    const current15m = this.baseBars[this.currentIndex];
    const sliced15m = this.baseBars.slice(0, this.currentIndex + 1);

    const multiBars = {
      '15m': sliced15m,
      '1h': this.aggregateSliced(sliced15m, 60 * 60 * 1000, 15 * 60 * 1000),
      '4h': this.aggregateSliced(sliced15m, 4 * 60 * 60 * 1000, 15 * 60 * 1000),
      '1d': this.aggregateSliced(sliced15m, 24 * 60 * 60 * 1000, 15 * 60 * 1000),
    };

    if (this.onTick) {
      this.onTick({
        index: this.currentIndex,
        total: this.baseBars.length,
        currentBar15m: current15m,
        multiBars: multiBars,
      });
    }
  }

  aggregateSliced(bars15m, targetMS, baseMS) {
    if (bars15m.length === 0) return [];
    const result = [];
    let current = null;
    let barCount = 0;
    const expectedBars = targetMS / baseMS;

    for (let i = 0; i < bars15m.length; i++) {
      const b = bars15m[i];
      const periodStart = Math.floor(b.open_time_ms / targetMS) * targetMS;

      if (!current || current.open_time_ms !== periodStart) {
        if (current) {
          result.push(current);
        }
        current = {
          open_time_ms: periodStart,
          open: b.open,
          high: b.high,
          low: b.low,
          close: b.close,
          volume: b.volume,
          quote_volume: b.quote_volume || 0,
          trade_count: b.trade_count || 0,
          state: 'forming',
        };
        barCount = 1;
      } else {
        if (b.high > current.high) current.high = b.high;
        if (b.low < current.low) current.low = b.low;
        current.close = b.close;
        current.volume += b.volume;
        current.quote_volume += (b.quote_volume || 0);
        current.trade_count += (b.trade_count || 0);
        barCount++;
      }

      // 如果当前周期的所有 15m 柱体已完整完成，则固化为 closed
      if (b.open_time_ms + baseMS === periodStart + targetMS && barCount === expectedBars) {
        current.state = 'closed';
      }
    }

    if (current) {
      result.push(current);
    }

    return result;
  }

  notifyState() {
    if (this.onStateChange) {
      this.onStateChange({
        isPlaying: this.isPlaying,
        speed: this.speed,
        currentIndex: this.currentIndex,
        totalBars: this.baseBars.length,
        currentBar: this.baseBars[this.currentIndex] || null,
      });
    }
  }
}

window.ReplayEngine = ReplayEngine;
