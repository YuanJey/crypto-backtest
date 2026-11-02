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
  const btnReloadData = document.getElementById('btnReloadData');

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
        panel.classList.remove('is-maximized');
        btn.textContent = '⛶';
      } else {
        document.querySelectorAll('.chart-panel').forEach(p => p.classList.remove('is-maximized'));
        chartsGrid.classList.add('maximized-mode');
        panel.classList.add('is-maximized');
        btn.textContent = '❐';
      }

      // 重算尺寸
      setTimeout(() => {
        Object.values(charts).forEach(c => c.resize());
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

  // 7. 加载行情数据并启动
  async function loadMarketData(symbol = 'BTC-USDT', datasetID = '') {
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

      btnRun.disabled = true;
      btnRun.innerHTML = `<span class="spinner-sm"></span> 回测撮合中...`;

      try {
        const payload = {
          dataset_id: datasetVal,
          symbol: sym
        };
        if (startMS > 0) payload.start_ms = startMS;
        if (endMS > 0) payload.end_ms = endMS;

        const resp = await Api.runBacktest(strategyId, payload);


        // 渲染回测指标看板
        if (metricsBar && resp && resp.report) {
          metricsBar.style.display = 'flex';
          const metrics = resp.report.metrics || {};
          const initialCapital = (resp.strategy && resp.strategy.initial_capital) || 10000;
          const finalEq = resp.final_equity || initialCapital;
          const retPct = ((finalEq - initialCapital) / initialCapital) * 100;

          const bmReturn = document.getElementById('bmReturn');
          const bmMaxDD = document.getElementById('bmMaxDD');
          const bmWinRate = document.getElementById('bmWinRate');
          const bmTrades = document.getElementById('bmTrades');
          const bmEquity = document.getElementById('bmEquity');

          if (bmReturn) {
            bmReturn.textContent = `${retPct >= 0 ? '+' : ''}${retPct.toFixed(2)}%`;
            bmReturn.className = `m-val ${retPct >= 0 ? 'm-success' : 'm-danger'}`;
          }
          if (bmMaxDD) {
            const maxDD = metrics.max_drawdown || 0;
            bmMaxDD.textContent = `-${Math.abs(maxDD * 100).toFixed(2)}%`;
          }
          if (bmWinRate) {
            const winRate = metrics.win_rate !== undefined ? metrics.win_rate * 100 : 0;
            bmWinRate.textContent = `${winRate.toFixed(1)}%`;
          }
          if (bmTrades) {
            const trades = resp.fill_count || resp.order_count || 0;
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
        btnRun.innerHTML = `<span class="btn-icon">🚀</span> 执行回测`;
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

