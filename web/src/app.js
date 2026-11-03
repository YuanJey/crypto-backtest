/**
 * QUANTIX - Terminal Controller & Interaction Orchestrator
 */

document.addEventListener('DOMContentLoaded', async () => {
  // 1. 初始化 4 个周期的图表实例
  const timeframes = [
    { tf: '15m', ms: 15 * 60 * 1000 },
    { tf: '1h', ms: 60 * 60 * 1000 },
    { tf: '4h', ms: 4 * 60 * 60 * 1000 },
    { tf: '1d', ms: 24 * 60 * 60 * 1000 },
  ];

  const charts = {};
  const equityChart = new EquityCurveChart('canvas-equity');
  const indicatorColors = ['#22d3ee', '#f59e0b', '#a78bfa', '#34d399', '#fb7185', '#60a5fa', '#facc15'];
  let strategySummaries = [];
  let activeBacktest = null;

  timeframes.forEach(({ tf, ms }) => {
    charts[tf] = new CandleChart(`canvas-${tf}`, {
      timeframe: tf,
      timeframeMS: ms,
      onCrosshairChange: (timeMS, bar) => {
        handleCrosshairSync(tf, timeMS, bar);
      }
    });
  });

  // 跨视口十字光标联动
  function handleCrosshairSync(sourceTf, timeMS, bar) {
    timeframes.forEach(({ tf }) => {
      if (tf !== sourceTf) {
        if (timeMS) {
          charts[tf].setSyncCrosshair(timeMS);
        } else {
          charts[tf].clearSyncCrosshair();
        }
      }
    });

    if (bar) {
      updateHeaderMeta(bar);
    }
  }

  // 2. 初始化回放引擎
  const replay = new ReplayEngine({
    onTick: ({ index, total, currentBar15m, multiBars }) => {
      // 分发更新 4 个周期图表
      timeframes.forEach(({ tf }) => {
        if (multiBars[tf]) {
          charts[tf].setBars(multiBars[tf]);
        }
      });
      updateBacktestVisuals(index, currentBar15m, multiBars);

      // 更新顶部当前行情指标
      updateHeaderMeta(currentBar15m);

      // 更新高周期形成中 (Forming) 状态标识
      updateFormingBadges(multiBars);

      // 更新进度条 UI
      updateScrubberUI(index, total, currentBar15m);
    },

    onStateChange: ({ isPlaying, speed, currentIndex, totalBars, currentBar }) => {
      updatePlayControlsUI(isPlaying, speed, currentIndex, totalBars, currentBar);
    }
  });

  // 3. UI 元素引用
  const btnPlayPause = document.getElementById('btnPlayPause');
  const playIcon = document.getElementById('playIcon');
  const playText = document.getElementById('playText');
  const btnStepBack = document.getElementById('btnStepBack');
  const btnStepForward = document.getElementById('btnStepForward');
  const btnJumpStart = document.getElementById('btnJumpStart');
  const btnJumpEnd = document.getElementById('btnJumpEnd');
  const timelineSlider = document.getElementById('timelineSlider');
  const scrubberFill = document.getElementById('scrubberFill');
  const barIndexText = document.getElementById('barIndexText');
  const replayStatusPill = document.getElementById('replayStatusPill');
  const replayStatusText = document.getElementById('replayStatusText');
  const speedButtons = document.querySelectorAll('.btn-speed');
  const chartsGrid = document.getElementById('chartsGrid');
  const backtestWorkspace = document.getElementById('backtestWorkspace');
  const btnReloadData = document.getElementById('btnReloadData');
  const equityEmptyState = document.getElementById('equityEmptyState');
  const equityStatus = document.getElementById('equityStatus');
  const liveEquity = document.getElementById('liveEquity');
  const liveReturn = document.getElementById('liveReturn');
  const liveDrawdown = document.getElementById('liveDrawdown');
  const livePosition = document.getElementById('livePosition');
  const liveLeverage = document.getElementById('liveLeverage');

  // 4. 事件绑定：回放控制
  btnPlayPause.addEventListener('click', () => replay.togglePlay());
  btnStepBack.addEventListener('click', () => replay.stepBackward());
  btnStepForward.addEventListener('click', () => replay.stepForward());
  btnJumpStart.addEventListener('click', () => replay.seek(0));
  btnJumpEnd.addEventListener('click', () => replay.seek(replay.baseBars.length - 1));

  timelineSlider.addEventListener('input', (e) => {
    const val = parseInt(e.target.value, 10);
    replay.pause();
    replay.seek(val);
  });

  // 倍速切换
  speedButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      const spd = parseFloat(btn.dataset.speed);
      replay.setSpeed(spd);
      speedButtons.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
    });
  });

  // 全局键盘快捷键
  window.addEventListener('keydown', (e) => {
    if (e.target.tagName === 'INPUT') return;
    if (e.code === 'Space') {
      e.preventDefault();
      replay.togglePlay();
    } else if (e.code === 'ArrowLeft') {
      e.preventDefault();
      replay.stepBackward();
    } else if (e.code === 'ArrowRight') {
      e.preventDefault();
      replay.stepForward();
    } else if (e.code === 'Home') {
      e.preventDefault();
      replay.seek(0);
    } else if (e.code === 'End') {
      e.preventDefault();
      replay.seek(replay.baseBars.length - 1);
    }
  });

  // 5. 单面板最大化 / 还原
  document.querySelectorAll('.btn-maximize').forEach(btn => {
    btn.addEventListener('click', () => {
      const targetId = btn.dataset.target;
      const panel = document.getElementById(targetId);
      const isAlreadyMax = panel.classList.contains('is-maximized');

      if (isAlreadyMax) {
        chartsGrid.classList.remove('maximized-mode');
        backtestWorkspace.classList.remove('chart-maximized');
        panel.classList.remove('is-maximized');
        btn.textContent = '⛶';
      } else {
        document.querySelectorAll('.chart-panel').forEach(p => p.classList.remove('is-maximized'));
        chartsGrid.classList.add('maximized-mode');
        backtestWorkspace.classList.add('chart-maximized');
        panel.classList.add('is-maximized');
        btn.textContent = '❐';
      }

      // 重算尺寸
      setTimeout(() => {
        Object.values(charts).forEach(c => c.resize());
        equityChart.resize();
      }, 50);
    });
  });

  // 6. UI 渲染辅助函数
  function updateHeaderMeta(bar) {
    if (!bar) return;
    const d = new Date(bar.open_time_ms);
    const dateStr = d.toISOString().replace('T', ' ').substring(0, 19) + ' UTC';

    document.getElementById('metaTime').textContent = dateStr;
    document.getElementById('metaOpen').textContent = bar.open.toFixed(2);
    document.getElementById('metaHigh').textContent = bar.high.toFixed(2);
    document.getElementById('metaLow').textContent = bar.low.toFixed(2);
    document.getElementById('metaClose').textContent = bar.close.toFixed(2);
    document.getElementById('metaVol').textContent = Math.round(bar.volume).toLocaleString();

    const changePct = ((bar.close - bar.open) / bar.open) * 100;
    const changeEl = document.getElementById('metaChange');
    changeEl.textContent = `${changePct >= 0 ? '+' : ''}${changePct.toFixed(2)}%`;
    changeEl.style.color = changePct >= 0 ? '#00c087' : '#f6465d';
  }

  function updateFormingBadges(multiBars) {
    ['1h', '4h', '1d'].forEach(tf => {
      const bars = multiBars[tf] || [];
      const badge = document.getElementById(`forming-${tf}`);
      const infoEl = document.getElementById(`info-${tf}`);
      if (!badge || bars.length === 0) return;

      const last = bars[bars.length - 1];
      if (last.state === 'forming') {
        badge.textContent = 'FORMING (聚合中)';
        badge.className = 'forming-indicator is-forming';
      } else {
        badge.textContent = 'CLOSED (已收盘)';
        badge.className = 'forming-indicator';
      }

      if (infoEl) {
        infoEl.textContent = `最新: ${last.close.toFixed(2)} (${bars.length} 根)`;
      }
    });

    const info15m = document.getElementById('info-15m');
    const bars15m = multiBars['15m'] || [];
    if (info15m && bars15m.length > 0) {
      info15m.textContent = `最新: ${bars15m[bars15m.length - 1].close.toFixed(2)} (${bars15m.length} 根)`;
    }
  }

  function updateScrubberUI(index, total, currentBar) {
    if (total <= 0) return;
    timelineSlider.max = total - 1;
    timelineSlider.value = index;

    const pct = ((index / (total - 1)) * 100).toFixed(1);
    scrubberFill.style.width = `${pct}%`;

    barIndexText.textContent = `${index + 1} / ${total}`;

    if (currentBar) {
      const d = new Date(currentBar.open_time_ms);
      document.getElementById('lblCurrentTime').textContent =
        `回放时刻: ${d.toISOString().replace('T', ' ').substring(0, 16)} UTC (${pct}%)`;
    }
  }

  function updatePlayControlsUI(isPlaying, speed, currentIndex, totalBars, currentBar) {
    if (isPlaying) {
      btnPlayPause.classList.add('is-playing');
      playIcon.textContent = '❚❚';
      playText.textContent = 'PAUSE';
      replayStatusPill.className = 'replay-status-pill playing';
      replayStatusText.textContent = `回放中 (${speed}x 实时驱动)`;
    } else {
      btnPlayPause.classList.remove('is-playing');
      playIcon.textContent = '▶';
      playText.textContent = 'PLAY';
      replayStatusPill.className = 'replay-status-pill';
      replayStatusText.textContent = '回放暂停 (PAUSED)';
    }

    if (replay.baseBars.length > 0) {
      const first = replay.baseBars[0];
      const last = replay.baseBars[replay.baseBars.length - 1];
      document.getElementById('lblStartTime').textContent = new Date(first.open_time_ms).toISOString().substring(0, 10);
      document.getElementById('lblEndTime').textContent = new Date(last.open_time_ms).toISOString().substring(0, 10);
    }
  }

  function resetBacktestVisualization(message = '等待回测') {
    activeBacktest = null;
    if (metricsBar) metricsBar.style.display = 'none';
    Object.values(charts).forEach(chart => chart.setIndicators([]));
    timeframes.forEach(({ tf }) => {
      renderIndicatorLegend(tf, [], message);
    });
    equityChart.setPoints([], 0);
    equityEmptyState.classList.remove('hidden');
    equityEmptyState.textContent = '暂无资金数据';
    equityStatus.textContent = message;
    equityStatus.className = 'equity-status';
    for (const element of [liveEquity, liveReturn, liveDrawdown, livePosition, liveLeverage]) {
      element.textContent = '--';
      element.className = 'equity-live-value';
    }
  }

  function indicatorColor(key) {
    let hash = 0;
    for (let index = 0; index < key.length; index++) {
      hash = ((hash << 5) - hash + key.charCodeAt(index)) | 0;
    }
    return indicatorColors[Math.abs(hash) % indicatorColors.length];
  }

  function indicatorPane(type, field) {
    const normalizedType = String(type || '').toLowerCase();
    if (normalizedType === 'supertrend') {
      return field === 'value' ? 'price' : 'oscillator';
    }
    return ['ema', 'sma', 'boll'].includes(normalizedType)
      ? 'price'
      : 'oscillator';
  }

  function indicatorValueIndex(tf, bar, currentIndex, baseIndexByTime) {
    if (tf === '15m') {
      return baseIndexByTime.get(bar.open_time_ms) ?? currentIndex;
    }
    if (bar.state === 'forming') return currentIndex;
    const timeframe = timeframes.find(item => item.tf === tf);
    const completedAtOpen = bar.open_time_ms + timeframe.ms - 15 * 60 * 1000;
    const completedIndex = baseIndexByTime.get(completedAtOpen);
    return completedIndex === undefined || completedIndex > currentIndex
      ? currentIndex
      : completedIndex;
  }

  function buildChartIndicatorSeries(tf, chartBars, currentIndex) {
    if (!activeBacktest) return [];
    const reportIndicators = activeBacktest.response.report?.indicators || {};
    const metadata = activeBacktest.indicators.filter(item => (item.timeframe || '15m') === tf);
    const series = [];

    metadata.forEach(item => {
      const fields = reportIndicators[item.id]?.fields || {};
      Object.entries(fields).forEach(([field, reportValues]) => {
        const key = `${item.id}.${field}`;
        const type = String(item.type || '').toLowerCase();
        const reportIndexes = chartBars.map(bar => {
          const reportIndex = indicatorValueIndex(
            tf,
            bar,
            currentIndex,
            activeBacktest.baseIndexByTime,
          );
          return reportIndex;
        });
        const values = reportIndexes.map(reportIndex => {
          const value = reportValues[reportIndex];
          return Number.isFinite(value) ? value : null;
        });
        const directionValues = type === 'supertrend' && field === 'value'
          ? reportIndexes.map(reportIndex => {
              const value = fields.direction?.[reportIndex];
              return Number.isFinite(value) ? value : null;
            })
          : null;
        series.push({
          id: item.id,
          field,
          label: key,
          kind: type,
          pane: indicatorPane(type, field),
          color: type === 'supertrend' && field === 'direction'
            ? '#f5b84b'
            : indicatorColor(key),
          directionValues,
          dashed: type === 'boll' && field === 'middle',
          min: type === 'rsi' ? 0 : (type === 'supertrend' && field === 'direction' ? -1 : undefined),
          max: type === 'rsi' ? 100 : (type === 'supertrend' && field === 'direction' ? 1 : undefined),
          values,
        });
      });
    });
    return series;
  }

  function formatIndicatorValue(value) {
    if (!Number.isFinite(value)) return '--';
    if (Math.abs(value) >= 1000) return value.toFixed(2);
    if (Math.abs(value) < 1) return value.toFixed(4);
    return value.toFixed(2);
  }

  function renderIndicatorLegend(tf, series, emptyMessage = '当前周期未配置策略指标') {
    const legend = document.getElementById(`legend-${tf}`);
    if (!legend) return;
    legend.innerHTML = '';
    if (series.length === 0) {
      const empty = document.createElement('span');
      empty.className = 'legend-empty';
      empty.textContent = emptyMessage;
      legend.appendChild(empty);
      return;
    }

    series.forEach(item => {
      const wrapper = document.createElement('span');
      wrapper.className = 'indicator-legend-item';
      const swatch = document.createElement('span');
      swatch.className = 'indicator-legend-swatch';
      const latestDirection = item.directionValues
        ? ([...item.directionValues].reverse().find(Number.isFinite) ?? null)
        : null;
      swatch.style.backgroundColor = latestDirection === null
        ? item.color
        : (latestDirection >= 0 ? '#2ad3a3' : '#ff6278');
      const label = document.createElement('span');
      label.textContent = item.label;
      const value = document.createElement('span');
      value.className = 'indicator-legend-value';
      const latest = [...item.values].reverse().find(Number.isFinite);
      value.textContent = formatIndicatorValue(latest);
      wrapper.append(swatch, label, value);
      legend.appendChild(wrapper);
    });
  }

  function visibleEquityPointCount(points, cutoffMS) {
    let low = 0;
    let high = points.length;
    while (low < high) {
      const middle = Math.floor((low + high) / 2);
      if (points[middle].time_ms <= cutoffMS) low = middle + 1;
      else high = middle;
    }
    return low;
  }

  function updateLiveEquity(points, visibleCount) {
    if (visibleCount <= 0) return;
    const current = points[visibleCount - 1];
    const initialCash = activeBacktest.initialCash;
    const returnPct = initialCash > 0
      ? ((current.equity - initialCash) / initialCash) * 100
      : 0;
    const peak = activeBacktest.peakEquity[visibleCount - 1] ?? initialCash;
    const drawdown = peak > 0 ? ((peak - current.equity) / peak) * 100 : 0;

    liveEquity.textContent = current.equity.toLocaleString(undefined, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    });
    liveReturn.textContent = `${returnPct >= 0 ? '+' : ''}${returnPct.toFixed(2)}%`;
    liveReturn.className = `equity-live-value ${returnPct >= 0 ? 'positive' : 'negative'}`;
    liveDrawdown.textContent = `-${Math.max(0, drawdown).toFixed(2)}%`;
    liveDrawdown.className = `equity-live-value ${drawdown > 0 ? 'negative' : ''}`;
    livePosition.textContent = Number(current.position_qty || 0).toFixed(4);
    liveLeverage.textContent = `${Number(current.position_leverage || 0).toFixed(2)}x`;
    equityStatus.textContent = `${visibleCount} / ${points.length} 点`;
    equityStatus.className = 'equity-status ready';
  }

  function updateBacktestVisuals(index, currentBar, multiBars) {
    if (!activeBacktest || !currentBar) return;

    timeframes.forEach(({ tf }) => {
      const chartBars = multiBars[tf] || [];
      const series = buildChartIndicatorSeries(tf, chartBars, index);
      charts[tf].setIndicators(series);
      renderIndicatorLegend(tf, series);
    });

    const points = activeBacktest.response.report?.equity_curve || [];
    const cutoffMS = currentBar.open_time_ms + 15 * 60 * 1000;
    const visibleCount = visibleEquityPointCount(points, cutoffMS);
    equityChart.setVisibleCount(visibleCount);
    equityEmptyState.classList.toggle('hidden', visibleCount > 0);
    updateLiveEquity(points, visibleCount);
  }

  // 7. 加载行情数据并启动
  async function loadMarketData(symbol = 'BTC-USDT', datasetID = '') {
    resetBacktestVisualization('等待回测');
    replayStatusText.textContent = '正在获取行情数据...';
    try {
      const data = await Api.fetchMarketBars(symbol, datasetID, true);
      const bars15m = (data.timeframes && data.timeframes['15m']) || data.bars || [];
      if (bars15m.length === 0) {
        throw new Error('No 15m bars returned');
      }

      document.getElementById('currentSymbol').textContent = symbol;
      replay.loadData(bars15m);
      replayStatusText.textContent = '回放就绪 (READY)';
    } catch (err) {
      console.error('Failed to load market data:', err);
      replayStatusText.textContent = '加载失败，请重试';
    }
  }

  btnReloadData.addEventListener('click', () => {
    const sel = document.getElementById('btSelectDataset');
    const opt = sel.options[sel.selectedIndex];
    const datasetId = sel.value === '__sample__' ? '' : sel.value;
    const sym = (opt && opt.dataset.symbol) || 'BTC-USDT';
    loadMarketData(sym, datasetId);
  });

  // 8. 回测中枢：数据源与策略双选择器与执行看板
  const selDataset = document.getElementById('btSelectDataset');
  const selStrategy = document.getElementById('btSelectStrategy');
  const btnRun = document.getElementById('btnRunBacktest');
  const metricsBar = document.getElementById('btMetricsBar');

  async function refreshDatasetOptions() {
    if (!selDataset) return;
    const currentVal = selDataset.value;
    const datasets = await Api.fetchMarketDatasets();

    let html = `<option value="__sample__" data-symbol="BTC-USDT" data-start="0" data-end="0">【仿真演示】BTC-USDT (15m · 7天)</option>`;
    if (datasets && datasets.length > 0) {
      html += datasets
        .map((ds) => {
          const sym = (ds.symbols && ds.symbols[0]) || 'BTC-USDT';
          const count = ds.row_count ? `${ds.row_count.toLocaleString()}根` : '';
          const start = ds.start_ms || 0;
          const end = ds.end_ms || 0;
          return `<option value="${ds.dataset_id}" data-symbol="${sym}" data-start="${start}" data-end="${end}">[本地数据] ${ds.dataset_id} (${sym} · ${count})</option>`;
        })
        .join('');
    }
    selDataset.innerHTML = html;
    if (currentVal && selDataset.querySelector(`option[value="${currentVal}"]`)) {
      selDataset.value = currentVal;
    }
  }

  async function refreshStrategyOptions() {
    if (!selStrategy) return;
    const currentVal = selStrategy.value;
    const strategies = await Api.fetchStrategies();
    strategySummaries = strategies || [];

    if (!strategies || strategies.length === 0) {
      selStrategy.innerHTML = `<option value="">(策略库为空，请前往策略页创建)</option>`;
      return;
    }

    selStrategy.innerHTML = strategies
      .map((s) => {
        const name = s.name ? `${s.name} (${s.id})` : s.id;
        const marketType = s.market_type ? `[${s.market_type.toUpperCase()}]` : '';
        return `<option value="${s.id}">${marketType} ${name}</option>`;
      })
      .join('');

    if (currentVal && selStrategy.querySelector(`option[value="${currentVal}"]`)) {
      selStrategy.value = currentVal;
    }
    updateHeaderMarketType();
  }

  function updateHeaderMarketType() {
    const strategy = strategySummaries.find(item => item.id === selStrategy?.value);
    const tag = document.getElementById('currentMarketType');
    if (tag) {
      tag.textContent = strategy?.market_type
        ? strategy.market_type.toUpperCase()
        : '--';
    }
  }

  // 监听数据集切换：即刻载入该数据集到多周期图表中
  if (selDataset) {
    selDataset.addEventListener('change', () => {
      const opt = selDataset.options[selDataset.selectedIndex];
      const datasetId = selDataset.value === '__sample__' ? '' : selDataset.value;
      const sym = (opt && opt.dataset.symbol) || 'BTC-USDT';
      loadMarketData(sym, datasetId);
    });
  }

  if (selStrategy) {
    selStrategy.addEventListener('change', () => {
      updateHeaderMarketType();
      resetBacktestVisualization('策略已切换');
    });
  }

  // 执行回测按钮
  if (btnRun) {
    btnRun.addEventListener('click', async () => {
      const strategyId = selStrategy.value;
      if (!strategyId) {
        alert('请先选择一个回测策略！若策略库为空，可前往【⚙️ 策略管理】页面新建策略。');
        return;
      }

      const opt = selDataset.options[selDataset.selectedIndex];
      const datasetVal = selDataset.value;
      const sym = (opt && opt.dataset.symbol) || 'BTC-USDT';
      const startMS = parseInt((opt && opt.dataset.start) || '0', 10);
      const endMS = parseInt((opt && opt.dataset.end) || '0', 10);
      let strategyMeta = strategySummaries.find(item => item.id === strategyId);
      if (!strategyMeta || !Array.isArray(strategyMeta.indicators)) {
        strategyMeta = await Api.fetchStrategyById(strategyId);
      }
      const strategyIndicators = strategyMeta?.indicators || [];

      btnRun.disabled = true;
      btnRun.innerHTML = `<span class="spinner-sm"></span> 回测撮合中...`;

      try {
        const payload = {
          dataset_id: datasetVal,
          symbol: sym,
          report: {
            equity_curve: true,
            metrics: [
              'final_equity',
              'total_return_pct',
              'max_drawdown_pct',
              'max_position_leverage',
              'order_count',
              'fill_count'
            ],
            indicators: strategyIndicators.map(indicator => ({ id: indicator.id }))
          }
        };
        if (startMS > 0) payload.start_ms = startMS;
        if (endMS > 0) payload.end_ms = endMS;
        if (datasetVal === '__sample__' && replay.baseBars.length > 0) {
          payload.start_ms = replay.baseBars[0].open_time_ms;
          payload.end_ms = replay.baseBars[replay.baseBars.length - 1].open_time_ms + 15 * 60 * 1000;
        }

        const resp = await Api.runBacktest(strategyId, payload);
        const marketTypeTag = document.getElementById('currentMarketType');
        if (marketTypeTag && resp.strategy?.market_type) {
          marketTypeTag.textContent = resp.strategy.market_type.toUpperCase();
        }
        const equityPoints = resp.report?.equity_curve || [];
        const initialCash = resp.strategy?.initial_cash || 0;
        // 预计算累计峰值权益，回放时按下标 O(1) 读取回撤
        const peakEquity = new Float64Array(equityPoints.length);
        let runningPeak = initialCash;
        for (let i = 0; i < equityPoints.length; i++) {
          const equity = equityPoints[i].equity;
          if (Number.isFinite(equity) && equity > runningPeak) runningPeak = equity;
          peakEquity[i] = runningPeak;
        }
        activeBacktest = {
          response: resp,
          indicators: strategyIndicators,
          initialCash,
          peakEquity,
          baseIndexByTime: new Map(
            replay.baseBars.map((bar, index) => [bar.open_time_ms, index]),
          ),
        };
        equityChart.setPoints(equityPoints, 0);
        // 回测完成后跳转至回放末尾，展示完整资金曲线（游标默认停在第 81 根，策略预热期内曲线恒为初始资金）
        replay.pause();
        replay.seek(replay.baseBars.length - 1);

        // 渲染回测指标看板
        if (metricsBar && resp && resp.report) {
          metricsBar.style.display = 'flex';
          const metrics = resp.report.metrics || {};
          const initialCapital = (resp.strategy && resp.strategy.initial_cash) || 10000;
          const finalEq = resp.final_equity ?? initialCapital;
          const retPct = ((finalEq - initialCapital) / initialCapital) * 100;

          const bmReturn = document.getElementById('bmReturn');
          const bmMaxDD = document.getElementById('bmMaxDD');
          const bmMaxLeverage = document.getElementById('bmMaxLeverage');
          const bmTrades = document.getElementById('bmTrades');
          const bmEquity = document.getElementById('bmEquity');

          if (bmReturn) {
            bmReturn.textContent = `${retPct >= 0 ? '+' : ''}${retPct.toFixed(2)}%`;
            bmReturn.className = `m-val ${retPct >= 0 ? 'm-success' : 'm-danger'}`;
          }
          if (bmMaxDD) {
            const maxDD = metrics.max_drawdown_pct || 0;
            bmMaxDD.textContent = `-${Math.abs(maxDD * 100).toFixed(2)}%`;
          }
          if (bmMaxLeverage) {
            bmMaxLeverage.textContent = `${Number(metrics.max_position_leverage || 0).toFixed(2)}x`;
          }
          if (bmTrades) {
            const trades = resp.fill_count ?? resp.order_count ?? 0;
            bmTrades.textContent = `${trades} 笔`;
          }
          if (bmEquity) {
            bmEquity.textContent = `$${finalEq.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
          }
        }

        replayStatusText.textContent = `回测完成: ${resp.bar_count || 0} 根K线, 产生 ${resp.fill_count || 0} 笔撮合`;
      } catch (err) {
        console.error('Backtest run error:', err);
        alert(`执行回测失败: ${err.message}`);
      } finally {
        btnRun.disabled = false;
        btnRun.innerHTML = `<span class="btn-icon" aria-hidden="true">▶</span> 执行回测`;
      }
    });
  }

  // 9. 多 Tab 导航切换
  const navTabs = document.querySelectorAll('.nav-tab-btn');
  function switchTab(tabId) {
    navTabs.forEach((b) => b.classList.toggle('active', b.dataset.tab === tabId));
    document.querySelectorAll('.tab-pane').forEach((p) => {
      p.classList.toggle('active', p.id === `view-${tabId}`);
    });

    if (tabId === 'backtest') {
      setTimeout(() => {
        Object.values(charts).forEach((c) => c.resize());
        equityChart.resize();
      }, 50);
      refreshDatasetOptions();
      refreshStrategyOptions();
    } else if (tabId === 'data' && window.DataManager) {
      window.DataManager.loadDatasets();
    }
  }

  navTabs.forEach((btn) => {
    btn.addEventListener('click', () => switchTab(btn.dataset.tab));
  });

  // 10. 初始化策略管理工作台 (纯策略创建与编辑)
  const strategyWorkbench = new StrategyWorkbench({
    containerId: 'view-strategies'
  });

  // 11. 初始化历史数据管理控制器 (纯数据下载与查看)
  if (window.DataManager) {
    window.DataManager.init();
  }

  // 导出全局 App 桥接
  window.App = {
    loadMarketBars: (symbol, datasetId) => {
      loadMarketData(symbol, datasetId);
    },
    switchTab: (tabId) => {
      switchTab(tabId);
    },
    refreshDatasets: () => {
      refreshDatasetOptions();
    },
    refreshStrategies: () => {
      refreshStrategyOptions();
    }
  };

  // 首次载入
  await refreshDatasetOptions();
  await refreshStrategyOptions();
  await loadMarketData();
});
