/**
 * QUANTIX - High-Performance Canvas Candlestick & Volume Chart
 * Supports Multi-Timeframe, Retina HiDPI, Zoom, Pan, Synchronized Crosshair & Forming State.
 */

class CandleChart {
  constructor(canvasId, options = {}) {
    this.canvas = document.getElementById(canvasId);
    if (!this.canvas) {
      throw new Error(`Canvas #${canvasId} not found`);
    }
    this.ctx = this.canvas.getContext('2d');
    this.timeframe = options.timeframe || '15m';
    this.timeframeMS = options.timeframeMS || 15 * 60 * 1000;
    this.tooltipEl = options.tooltipEl || null;

    // 数据状态
    this.bars = [];
    this.indicatorSeries = [];
    this.visibleBarsCount = options.initialVisibleBars || 60;
    this.barSpacing = 8;
    this.rightOffsetBars = 5; // 右侧预留空隙

    // 交互与视口
    this.isDragging = false;
    this.dragStartX = 0;
    this.dragInitialOffset = 0;
    this.crosshair = null; // { x, y, timeMS, price }
    this.syncedTimeMS = null; // 外部同步光标

    // 视觉配色
    this.colors = {
      bg: '#0b0f13',
      grid: 'rgba(255, 255, 255, 0.04)',
      axisText: '#7f8b98',
      crosshair: 'rgba(255, 255, 255, 0.28)',
      crosshairBadgeBg: '#537df5',
      crosshairBadgeText: '#ffffff',
      bull: '#2ad3a3',
      bullDim: 'rgba(42, 211, 163, 0.2)',
      bear: '#ff6278',
      bearDim: 'rgba(255, 98, 120, 0.2)',
      formingOutline: '#f5b84b',
      priceLine: 'rgba(83, 125, 245, 0.7)',
    };

    // 事件分发
    this.onCrosshairChange = options.onCrosshairChange || null;

    this.initEvents();
    this.resize();
  }

  resize() {
    const rect = this.canvas.parentElement.getBoundingClientRect();
    this.width = rect.width;
    this.height = rect.height;
    this.dpr = window.devicePixelRatio || 1;

    this.canvas.width = this.width * this.dpr;
    this.canvas.height = this.height * this.dpr;
    this.canvas.style.width = `${this.width}px`;
    this.canvas.style.height = `${this.height}px`;

    this.ctx.resetTransform();
    this.ctx.scale(this.dpr, this.dpr);

    // 布局分区
    this.paddingRight = 68; // 价格轴宽度
    this.paddingBottom = 24; // 时间轴高度
    this.chartAreaWidth = this.width - this.paddingRight;
    this.chartAreaHeight = this.height - this.paddingBottom;
    this.volAreaHeight = Math.max(36, this.chartAreaHeight * 0.2);
    this.priceAreaHeight = this.chartAreaHeight - this.volAreaHeight;

    this.render();
  }

  setBars(bars) {
    this.bars = bars || [];
    this.render();
  }

  setIndicators(series) {
    this.indicatorSeries = Array.isArray(series) ? series : [];
    this.render();
  }

  // 跨视口同步光标
  setSyncCrosshair(timeMS) {
    this.syncedTimeMS = timeMS;
    this.render();
  }

  clearSyncCrosshair() {
    this.syncedTimeMS = null;
    this.render();
  }

  initEvents() {
    window.addEventListener('resize', () => this.resize());

    this.canvas.addEventListener('mousedown', (e) => {
      this.isDragging = true;
      this.dragStartX = e.clientX;
      this.dragInitialOffset = this.rightOffsetBars;
    });

    window.addEventListener('mousemove', (e) => {
      if (this.isDragging) {
        const deltaX = e.clientX - this.dragStartX;
        const deltaBars = Math.round(deltaX / this.barSpacing);
        this.rightOffsetBars = Math.max(-10, Math.min(this.bars.length + 10, this.dragInitialOffset - deltaBars));
        this.render();
      }
    });

    window.addEventListener('mouseup', () => {
      this.isDragging = false;
    });

    this.canvas.addEventListener('mousemove', (e) => {
      const rect = this.canvas.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const y = e.clientY - rect.top;

      if (x >= 0 && x <= this.chartAreaWidth && y >= 0 && y <= this.chartAreaHeight) {
        const info = this.getPointInfo(x, y);
        this.crosshair = { x, y, ...info };
        if (this.onCrosshairChange && info.timeMS) {
          this.onCrosshairChange(info.timeMS, info.bar);
        }
      } else {
        this.crosshair = null;
        if (this.onCrosshairChange) {
          this.onCrosshairChange(null, null);
        }
      }
      this.render();
    });

    this.canvas.addEventListener('mouseleave', () => {
      this.crosshair = null;
      if (this.onCrosshairChange) {
        this.onCrosshairChange(null, null);
      }
      this.render();
    });

    // 滚轮缩放
    this.canvas.addEventListener('wheel', (e) => {
      e.preventDefault();
      const zoomFactor = e.deltaY < 0 ? 0.88 : 1.14;
      const newVisible = Math.max(15, Math.min(this.bars.length + 30, Math.round(this.visibleBarsCount * zoomFactor)));
      this.visibleBarsCount = newVisible;
      this.render();
    }, { passive: false });
  }

  // 坐标映射
  getPointInfo(x, y) {
    if (this.bars.length === 0) return { price: 0, timeMS: null, bar: null };
    const { startIndex, endIndex, barWidth, spacing } = this.calculateViewport();

    const relX = this.chartAreaWidth - x;
    const barIndexFromRight = Math.floor((relX + spacing * 0.5) / spacing) - this.rightOffsetBars;
    const actualIndex = (this.bars.length - 1) - barIndexFromRight;

    let bar = null;
    let timeMS = null;
    if (actualIndex >= 0 && actualIndex < this.bars.length) {
      bar = this.bars[actualIndex];
      timeMS = bar.open_time_ms;
    }

    const price = this.yToPrice(y);
    return { price, timeMS, bar };
  }

  calculateViewport() {
    const spacing = this.chartAreaWidth / this.visibleBarsCount;
    this.barSpacing = spacing;
    const barWidth = Math.max(1, spacing * 0.72);

    const endIndex = Math.min(this.bars.length, Math.max(0, this.bars.length - this.rightOffsetBars));
    const startIndex = Math.max(0, endIndex - this.visibleBarsCount);

    return { startIndex, endIndex, barWidth, spacing };
  }

  priceToY(price) {
    if (this.maxPrice === this.minPrice) return this.priceAreaHeight / 2;
    return this.priceAreaHeight - ((price - this.minPrice) / (this.maxPrice - this.minPrice)) * this.priceAreaHeight;
  }

  yToPrice(y) {
    if (this.maxPrice === this.minPrice) return this.maxPrice;
    return this.minPrice + ((this.priceAreaHeight - y) / this.priceAreaHeight) * (this.maxPrice - this.minPrice);
  }

  volToY(vol) {
    if (!this.maxVol || this.maxVol === 0) return this.chartAreaHeight;
    const normalized = (vol / this.maxVol) * (this.volAreaHeight - 10);
    return this.chartAreaHeight - normalized;
  }

  render() {
    if (!this.ctx || !this.width) return;
    const ctx = this.ctx;

    // 清屏
    ctx.clearRect(0, 0, this.width, this.height);

    if (this.bars.length === 0) {
      this.drawEmptyPlaceholder();
      return;
    }

    const { startIndex, endIndex, barWidth, spacing } = this.calculateViewport();
    const visibleBars = this.bars.slice(startIndex, endIndex);

    // 计算当前视口的高低价与成交量
    let minPrice = Infinity;
    let maxPrice = -Infinity;
    let maxVol = 0;

    for (const b of visibleBars) {
      if (b.low < minPrice) minPrice = b.low;
      if (b.high > maxPrice) maxPrice = b.high;
      if (b.volume > maxVol) maxVol = b.volume;
    }
    for (const series of this.indicatorSeries) {
      if (series.pane !== 'price') continue;
      for (let index = startIndex; index < endIndex; index++) {
        const value = series.values[index];
        if (!Number.isFinite(value)) continue;
        if (value < minPrice) minPrice = value;
        if (value > maxPrice) maxPrice = value;
      }
    }

    if (!isFinite(minPrice) || !isFinite(maxPrice)) {
      minPrice = 90000;
      maxPrice = 95000;
    }

    // 留出 8% 上下边距
    const priceRange = maxPrice - minPrice || 100;
    this.minPrice = minPrice - priceRange * 0.08;
    this.maxPrice = maxPrice + priceRange * 0.08;
    this.maxVol = maxVol || 100;

    // 绘制网格与坐标轴
    this.drawGrid(ctx);

    // 绘制蜡烛图与成交量副图
    for (let i = startIndex; i < endIndex; i++) {
      const bar = this.bars[i];
      const barX = this.chartAreaWidth - ((this.bars.length - 1 - i + this.rightOffsetBars) * spacing) - spacing * 0.5;

      if (barX < -spacing || barX > this.chartAreaWidth + spacing) continue;

      this.drawBar(ctx, bar, barX, barWidth);
    }

    this.drawIndicators(ctx, startIndex, endIndex, spacing);

    // 绘制最新价格基准线
    const lastBar = this.bars[this.bars.length - 1];
    if (lastBar) {
      this.drawLatestPriceLine(ctx, lastBar);
    }

    // 绘制十字光标
    this.drawCrosshairs(ctx, spacing);
  }

  drawGrid(ctx) {
    ctx.lineWidth = 1;
    ctx.strokeStyle = this.colors.grid;
    ctx.font = '10px JetBrains Mono, monospace';
    ctx.fillStyle = this.colors.axisText;
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';

    // 价格横线（5条）
    const steps = 5;
    for (let i = 0; i <= steps; i++) {
      const price = this.minPrice + (i / steps) * (this.maxPrice - this.minPrice);
      const y = Math.round(this.priceToY(price));

      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(this.chartAreaWidth, y);
      ctx.stroke();

      // 右侧价格轴文字
      ctx.fillText(price.toFixed(2), this.chartAreaWidth + 6, y);
    }

    // 副图分割线
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.08)';
    ctx.beginPath();
    ctx.moveTo(0, this.priceAreaHeight);
    ctx.lineTo(this.width, this.priceAreaHeight);
    ctx.stroke();

    // 右侧价格轴左分割线
    ctx.beginPath();
    ctx.moveTo(this.chartAreaWidth, 0);
    ctx.lineTo(this.chartAreaWidth, this.height);
    ctx.stroke();
  }

  drawBar(ctx, bar, x, width) {
    const isBull = bar.close >= bar.open;
    const isForming = bar.state === 'forming';
    const color = isBull ? this.colors.bull : this.colors.bear;

    const openY = this.priceToY(bar.open);
    const closeY = this.priceToY(bar.close);
    const highY = this.priceToY(bar.high);
    const lowY = this.priceToY(bar.low);

    const candleTop = Math.min(openY, closeY);
    const candleHeight = Math.max(1.5, Math.abs(closeY - openY));

    // 1. 绘制影线
    ctx.lineWidth = 1.2;
    ctx.strokeStyle = isForming ? this.colors.formingOutline : color;
    ctx.beginPath();
    ctx.moveTo(Math.round(x), Math.round(highY));
    ctx.lineTo(Math.round(x), Math.round(candleTop));
    ctx.moveTo(Math.round(x), Math.round(candleTop + candleHeight));
    ctx.lineTo(Math.round(x), Math.round(lowY));
    ctx.stroke();

    // 2. 绘制实体
    const rectX = Math.round(x - width / 2);
    const rectY = Math.round(candleTop);
    const rectW = Math.max(2, Math.round(width));
    const rectH = Math.round(candleHeight);

    if (isForming) {
      // 形成中的蜡烛：半透明虚线框标记
      ctx.fillStyle = isBull ? 'rgba(0, 192, 135, 0.35)' : 'rgba(246, 70, 93, 0.35)';
      ctx.fillRect(rectX, rectY, rectW, rectH);
      ctx.lineWidth = 1.5;
      ctx.setLineDash([3, 2]);
      ctx.strokeStyle = this.colors.formingOutline;
      ctx.strokeRect(rectX, rectY, rectW, rectH);
      ctx.setLineDash([]);
    } else {
      ctx.fillStyle = color;
      ctx.fillRect(rectX, rectY, rectW, rectH);
    }

    // 3. 绘制成交量柱
    const volY = this.volToY(bar.volume);
    const volHeight = this.chartAreaHeight - volY;
    ctx.fillStyle = isBull ? this.colors.bullDim : this.colors.bearDim;
    ctx.fillRect(rectX, Math.round(volY), rectW, Math.round(volHeight));
  }

  drawIndicators(ctx, startIndex, endIndex, spacing) {
    const priceSeries = this.indicatorSeries.filter(series => series.pane === 'price');
    const oscillatorSeries = this.indicatorSeries.filter(series => series.pane === 'oscillator');

    for (const series of priceSeries) {
      this.drawSeriesLine(
        ctx,
        series,
        startIndex,
        endIndex,
        spacing,
        value => this.priceToY(value),
        1.45,
      );
    }

    if (oscillatorSeries.length === 0) return;
    const oscillatorTop = this.priceAreaHeight + 5;
    const oscillatorBottom = this.chartAreaHeight - 4;
    const oscillatorHeight = Math.max(1, oscillatorBottom - oscillatorTop);

    for (const series of oscillatorSeries) {
      const visibleValues = series.values
        .slice(startIndex, endIndex)
        .filter(Number.isFinite);
      if (visibleValues.length === 0) continue;

      let minValue = Number.isFinite(series.min) ? series.min : Math.min(...visibleValues);
      let maxValue = Number.isFinite(series.max) ? series.max : Math.max(...visibleValues);
      if (maxValue === minValue) {
        minValue -= 1;
        maxValue += 1;
      } else if (!Number.isFinite(series.min) && !Number.isFinite(series.max)) {
        const padding = (maxValue - minValue) * 0.12;
        minValue -= padding;
        maxValue += padding;
      }

      if (series.kind === 'rsi') {
        ctx.save();
        ctx.setLineDash([3, 3]);
        ctx.strokeStyle = 'rgba(148, 163, 184, 0.16)';
        for (const threshold of [30, 70]) {
          const y = oscillatorBottom - ((threshold - minValue) / (maxValue - minValue)) * oscillatorHeight;
          ctx.beginPath();
          ctx.moveTo(0, y);
          ctx.lineTo(this.chartAreaWidth, y);
          ctx.stroke();
        }
        ctx.restore();
      }

      this.drawSeriesLine(
        ctx,
        series,
        startIndex,
        endIndex,
        spacing,
        value => oscillatorBottom - ((value - minValue) / (maxValue - minValue)) * oscillatorHeight,
        1.25,
      );
    }
  }

  drawSeriesLine(ctx, series, startIndex, endIndex, spacing, valueToY, lineWidth) {
    ctx.save();
    ctx.lineWidth = lineWidth;
    ctx.strokeStyle = series.color || '#f59e0b';
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';
    if (series.dashed) {
      ctx.setLineDash([4, 3]);
    }

    let drawing = false;
    ctx.beginPath();
    for (let index = startIndex; index < endIndex; index++) {
      const value = series.values[index];
      if (!Number.isFinite(value)) {
        drawing = false;
        continue;
      }
      const x = this.chartAreaWidth -
        ((this.bars.length - 1 - index + this.rightOffsetBars) * spacing) -
        spacing * 0.5;
      const y = valueToY(value);
      if (!Number.isFinite(x) || !Number.isFinite(y)) {
        drawing = false;
        continue;
      }
      if (!drawing) {
        ctx.moveTo(x, y);
        drawing = true;
      } else {
        ctx.lineTo(x, y);
      }
    }
    ctx.stroke();
    ctx.restore();
  }

  drawLatestPriceLine(ctx, lastBar) {
    const y = Math.round(this.priceToY(lastBar.close));
    const isBull = lastBar.close >= lastBar.open;
    const color = isBull ? this.colors.bull : this.colors.bear;

    ctx.save();
    ctx.setLineDash([4, 3]);
    ctx.lineWidth = 1;
    ctx.strokeStyle = color;
    ctx.beginPath();
    ctx.moveTo(0, y);
    ctx.lineTo(this.chartAreaWidth, y);
    ctx.stroke();
    ctx.restore();

    // 价格徽标
    ctx.fillStyle = color;
    ctx.fillRect(this.chartAreaWidth + 1, y - 9, this.paddingRight - 2, 18);
    ctx.fillStyle = '#ffffff';
    ctx.font = 'bold 10px JetBrains Mono, monospace';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    ctx.fillText(lastBar.close.toFixed(2), this.chartAreaWidth + 5, y);
  }

  drawCrosshairs(ctx, spacing) {
    let targetTimeMS = null;
    let targetPrice = null;
    let crossX = null;
    let crossY = null;

    if (this.crosshair) {
      crossX = this.crosshair.x;
      crossY = this.crosshair.y;
      targetTimeMS = this.crosshair.timeMS;
      targetPrice = this.crosshair.price;
    } else if (this.syncedTimeMS) {
      targetTimeMS = this.syncedTimeMS;
      // 根据同步时间查找当前周期的 Bar X 坐标
      crossX = this.findXByTime(targetTimeMS, spacing);
    }

    if (crossX === null && crossY === null) return;

    ctx.save();
    ctx.setLineDash([4, 4]);
    ctx.strokeStyle = this.colors.crosshair;
    ctx.lineWidth = 1;

    // 垂直时间线
    if (crossX !== null && crossX >= 0 && crossX <= this.chartAreaWidth) {
      ctx.beginPath();
      ctx.moveTo(Math.round(crossX), 0);
      ctx.lineTo(Math.round(crossX), this.chartAreaHeight);
      ctx.stroke();

      // 底部时间刻度徽标
      if (targetTimeMS) {
        const timeStr = this.formatTime(targetTimeMS);
        ctx.fillStyle = '#2563eb';
        const badgeW = 95;
        ctx.fillRect(Math.round(crossX - badgeW / 2), this.chartAreaHeight + 2, badgeW, 18);
        ctx.fillStyle = '#ffffff';
        ctx.font = '10px JetBrains Mono, monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(timeStr, Math.round(crossX), this.chartAreaHeight + 11);
      }
    }

    // 水平价格线
    if (crossY !== null && crossY >= 0 && crossY <= this.priceAreaHeight) {
      ctx.beginPath();
      ctx.moveTo(0, Math.round(crossY));
      ctx.lineTo(this.chartAreaWidth, Math.round(crossY));
      ctx.stroke();

      // 右侧价格轴徽标
      if (targetPrice !== null) {
        ctx.fillStyle = '#374151';
        ctx.fillRect(this.chartAreaWidth + 1, Math.round(crossY - 9), this.paddingRight - 2, 18);
        ctx.fillStyle = '#ffffff';
        ctx.font = '10px JetBrains Mono, monospace';
        ctx.textAlign = 'left';
        ctx.textBaseline = 'middle';
        ctx.fillText(targetPrice.toFixed(2), this.chartAreaWidth + 5, Math.round(crossY));
      }
    }
    ctx.restore();
  }

  findXByTime(timeMS, spacing) {
    if (!timeMS || this.bars.length === 0) return null;
    // 找到对应时间戳所在的 Bar（若高周期，属于其区间的 Bar）
    let matchedIndex = -1;
    for (let i = 0; i < this.bars.length; i++) {
      const b = this.bars[i];
      if (timeMS >= b.open_time_ms && timeMS < b.open_time_ms + this.timeframeMS) {
        matchedIndex = i;
        break;
      }
    }
    if (matchedIndex === -1) {
      // 容错：找距离最近的一个
      if (timeMS < this.bars[0].open_time_ms) matchedIndex = 0;
      else matchedIndex = this.bars.length - 1;
    }

    return this.chartAreaWidth - ((this.bars.length - 1 - matchedIndex + this.rightOffsetBars) * spacing) - spacing * 0.5;
  }

  formatTime(ms) {
    const d = new Date(ms);
    const m = (d.getUTCMonth() + 1).toString().padStart(2, '0');
    const day = d.getUTCDate().toString().padStart(2, '0');
    const h = d.getUTCHours().toString().padStart(2, '0');
    const min = d.getUTCMinutes().toString().padStart(2, '0');
    return `${m}-${day} ${h}:${min}Z`;
  }

  drawEmptyPlaceholder() {
    this.ctx.fillStyle = '#848e9c';
    this.ctx.font = '13px Inter, sans-serif';
    this.ctx.textAlign = 'center';
    this.ctx.textBaseline = 'middle';
    this.ctx.fillText(`[${this.timeframe}] 等待数据加载...`, this.width / 2, this.height / 2);
  }
}

window.CandleChart = CandleChart;

class EquityCurveChart {
  constructor(canvasId) {
    this.canvas = document.getElementById(canvasId);
    if (!this.canvas) {
      throw new Error(`Canvas #${canvasId} not found`);
    }
    this.ctx = this.canvas.getContext('2d');
    this.points = [];
    this.visibleCount = 0;
    window.addEventListener('resize', () => this.resize());
    this.resize();
  }

  resize() {
    const rect = this.canvas.parentElement.getBoundingClientRect();
    this.width = rect.width;
    this.height = rect.height;
    this.dpr = window.devicePixelRatio || 1;
    this.canvas.width = this.width * this.dpr;
    this.canvas.height = this.height * this.dpr;
    this.canvas.style.width = `${this.width}px`;
    this.canvas.style.height = `${this.height}px`;
    this.ctx.resetTransform();
    this.ctx.scale(this.dpr, this.dpr);
    this.render();
  }

  setPoints(points, visibleCount = null) {
    this.points = Array.isArray(points) ? points : [];
    this.visibleCount = visibleCount === null
      ? this.points.length
      : Math.max(0, Math.min(this.points.length, visibleCount));
    this.render();
  }

  setVisibleCount(visibleCount) {
    this.visibleCount = Math.max(0, Math.min(this.points.length, visibleCount));
    this.render();
  }

  render() {
    if (!this.ctx || !this.width) return;
    const ctx = this.ctx;
    ctx.clearRect(0, 0, this.width, this.height);

    const points = this.points.slice(0, this.visibleCount);
    if (points.length === 0) return;

    const padding = { top: 10, right: 82, bottom: 20, left: 10 };
    const plotWidth = Math.max(1, this.width - padding.left - padding.right);
    const plotHeight = Math.max(1, this.height - padding.top - padding.bottom);
    const equities = points.map(point => point.equity).filter(Number.isFinite);
    if (equities.length === 0) return;

    let minEquity = Math.min(...equities);
    let maxEquity = Math.max(...equities);
    const range = maxEquity - minEquity || Math.max(1, Math.abs(maxEquity) * 0.01);
    minEquity -= range * 0.12;
    maxEquity += range * 0.12;
    const xFor = index => padding.left + (index / Math.max(1, points.length - 1)) * plotWidth;
    const yFor = value => padding.top + (1 - (value - minEquity) / (maxEquity - minEquity)) * plotHeight;

    ctx.save();
    ctx.font = '9px JetBrains Mono, monospace';
    ctx.fillStyle = '#64748b';
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.05)';
    ctx.lineWidth = 1;
    ctx.textBaseline = 'middle';
    for (let step = 0; step <= 3; step++) {
      const value = minEquity + (step / 3) * (maxEquity - minEquity);
      const y = yFor(value);
      ctx.beginPath();
      ctx.moveTo(padding.left, y);
      ctx.lineTo(this.width - padding.right, y);
      ctx.stroke();
      ctx.fillText(value.toFixed(2), this.width - padding.right + 7, y);
    }

    const initialEquity = points[0].equity;
    const finalEquity = points[points.length - 1].equity;
    const positive = finalEquity >= initialEquity;
    const lineColor = positive ? '#2ad3a3' : '#ff6278';

    ctx.beginPath();
    points.forEach((point, index) => {
      const x = xFor(index);
      const y = yFor(point.equity);
      if (index === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.lineTo(xFor(points.length - 1), padding.top + plotHeight);
    ctx.lineTo(xFor(0), padding.top + plotHeight);
    ctx.closePath();
    ctx.fillStyle = positive ? 'rgba(42, 211, 163, 0.09)' : 'rgba(255, 98, 120, 0.09)';
    ctx.fill();

    ctx.beginPath();
    points.forEach((point, index) => {
      const x = xFor(index);
      const y = yFor(point.equity);
      if (index === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.strokeStyle = lineColor;
    ctx.lineWidth = 1.6;
    ctx.lineJoin = 'round';
    ctx.stroke();

    const lastX = xFor(points.length - 1);
    const lastY = yFor(finalEquity);
    ctx.beginPath();
    ctx.arc(lastX, lastY, 3, 0, Math.PI * 2);
    ctx.fillStyle = lineColor;
    ctx.fill();

    const lastPoint = points[points.length - 1];
    const timeLabel = new Date(lastPoint.time_ms).toISOString().substring(5, 16).replace('T', ' ');
    ctx.fillStyle = '#64748b';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'bottom';
    ctx.fillText(timeLabel, this.width - padding.right, this.height - 3);
    ctx.restore();
  }
}

window.EquityCurveChart = EquityCurveChart;
