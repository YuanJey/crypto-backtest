# 🚀 Crypto-Backtest 动态规则回测系统与前端分析终端

一个基于 Go 统一控制面 + Parquet 列式存储 + 原生 Canvas 前端的多周期加密货币规则回测与行情回放框架。

---

## 🌟 核心特性

- **多周期防未来函数引擎**：以 15m 基础 K 线为驱动时钟，逐根增量推进；高周期（1h、4h、1d）在未收盘时标记为 `forming`，收盘瞬间固化为 `closed`，彻底避免时序穿越。
- **纯原生高性能多视口终端**：原生 HTML5 + Vanilla CSS + 2D Canvas 实现，零外部重型依赖，同屏 2x2 网格分屏（15m/1h/4h/1d），支持 Crosshair 十字光标跨视口联动与平滑拖拽缩放。
- **动态行情回放系统**：支持播放、暂停、单步步退（←）、单步步进（→）、多档调速（0.5x ~ 20x）与时间轴任意位置拖拽（Scrubbing）。
- **静态强类型策略编译**：声明式 JSON 策略格式，支持嵌套 `AND/OR/NOT` 逻辑与交叉算子（`cross_up`/`cross_down`），由 Go 编译器在回测前完成静态类型检查与 DAG 构建。
- **杠杆、仓位与退出策略**：交易所杠杆控制总名义仓位上限，实际仓位杠杆和资金衰减控制首仓；结构化退出策略支持开仓/加仓快照 ATR、实时 ATR、百分比、固定价差、分批止盈止损及触发价成交。
- **列式数据流水线**：集成币安 K 线数据下载、清洗去重、按 `exchange/symbol/timeframe` 分区的 Parquet 存储与 `manifest.json` 元数据管理。

---

## 📁 目录结构

```text
crypto-backtest/
├── cmd/
│   ├── api/            # HTTP 控制面与静态资源托管服务入口
│   ├── backtest/       # 本地回测执行与报告生成 CLI
│   └── market-data/    # 行情数据下载与清洗 CLI
├── internal/
│   ├── api/            # REST API 服务层与静态路由
│   ├── backtest/       # 回测业务编排服务
│   ├── compiler/       # 策略 AST 静态编译器
│   ├── datafeed/       # 币安 REST 行情数据抓取客户端
│   ├── dataset/        # 行情清洗、质量检测与 Parquet 读写器
│   ├── engine/         # 事件驱动回测撮合引擎与高周期聚合器
│   ├── indicator/      # 内置指标运行时（EMA/SMA/RSI/ATR/BOLL/Supertrend）与注册表
│   ├── market/         # K 线、交易对、时间周期领域模型
│   └── strategy/       # 策略模型、JSON 解码、静态语义校验器与 SQLite 仓储
├── web/                # 前端多周期图表与回放终端
│   ├── index.html      # 主页面结构
│   ├── styles.css      # 暗黑金融终端样式系统（无 Tailwind）
│   └── src/
│       ├── chart.js    # 原生 Canvas 蜡烛图与成交量副图引擎
│       ├── replay.js   # 严格防未来函数的多周期回放状态机
│       ├── api.js      # 行情 API 客户端与备用仿真数据生成器
│       └── app.js      # 界面交互与快捷键主控
├── strategies/         # 示例策略文件库
└── docs/               # 架构与系统设计文档
```

---

## ⚡ 快速启动

### 1. 启动 API 服务与前端终端

```bash
go run ./cmd/api -addr :8080 -data-root data/market -web-root web
```

打开浏览器访问 `http://localhost:8080` 即可进入多周期行情回放终端。

### 2. 运行单元测试

```bash
go test -v ./...
```

### 3. 数据下载与本地回测执行

```bash
# 下载币安 15m K线
go run ./cmd/market-data download -symbols BTC-USDT -start 2025-01-01T00:00:00Z -end 2025-02-01T00:00:00Z

# 执行本地回测
go run ./cmd/backtest -strategy-id ema-boll-rsi -run -data-root data/market
```

---

## 📝 变更日志与追溯记录 (Change Log)

### 2026-11-02
- **新增模块**：前端多周期 K 线图分区（15m/1h/4h/1d）与动态回放终端（`web/index.html`、`web/styles.css`、`web/src/chart.js`、`web/src/replay.js`、`web/src/api.js`、`web/src/app.js`）。
- **扩展接口**：在 `internal/api/server.go` 新增 `GET /api/v1/market/bars` 接口，支持按单周期或 `multi=true` 批量返回 15m/1h/4h/1d 数据；新增静态资源挂载与 `-web-root` 标志。
- **策略管理与 Tab 导航系统**：
  - 新增顶部/右侧全局 Tab 导航，实现“📈 行情回测”与“⚙️ 策略管理”无缝切换与视口平滑自适应。
  - 新增策略管理工作台（`web/src/strategy.js`），支持策略列表/指标目录联动、可视化看板、Rule AST 条件树展示、JSON 源码编辑器、“校验并保存”以及“一键载入到回测”。
  - 在 `cmd/api/main.go` 增加默认示例策略自动种子初始化。
- **纯鼠标可视化策略构建器 (Visual Strategy Builder)**：
  - 账户与标的可视化表单改造（策略名、资金、市场类型、标的实时双向绑定）。
  - 指标可视化管理器：联动服务端 `/api/v1/catalog` 模态弹窗增删改，动态适配 EMA/SMA/RSI/ATR/BOLL 参数与输入源。
  - 规则条件树交互编排器：金叉/死叉强制选择两个不同指标输出；普通比较支持常数/序列切换，并提供 AND/OR 逻辑门与仓位滑块。
  - 前端 AST 编解码器：实现 UI 状态与 Go 强类型 AST 双向即时编译，无需编写任何 JSON 即可完成策略构建与保存。
- **影响范围**：
  - `cmd/api/main.go`
  - `internal/api/server.go`
  - `internal/api/server_test.go`
  - `web/index.html`
  - `web/styles.css`
  - `web/src/api.js`
  - `web/src/app.js`
  - `web/src/strategy.js`
- **历史数据下载与数据集管理 (Market Data Downloader & Manager)**：
  - 后端 API 扩展：
    - `POST /api/v1/market/download`：支持标的（BTC-USDT等）、现货/合约类型、起止时间戳，强制校验并锁死 15m 基准周期；自动调用 `binance.Client` 分页拉取、`dataset.CleanBars` 清洗插值并由 `dataset.Writer` 落地为 Parquet 与 `manifest.json`。
    - `GET /api/v1/market/datasets`：检索本地已落地的数据集元数据清单。
  - 前端“📥 历史数据”控制台：
    - 新增全局 Tab 标签与 `#view-data` 面板（`web/src/data.js`）。
    - 提供标的快捷 Chip（BTC/ETH/SOL/BNB/DOGE）与自由输入、市场类型切换、15m 基准驱动锁死、时间范围快捷区间（近 7/14/30/90 天）与自动 15m 对齐。
    - 实时反馈数据下载流水线状态、K 线行数统计、缺失补全统计及 Parquet 分区信息。
    - 本地已构建数据集卡片库，支持一键在 2x2 多周期图表中预览加载与一键注入策略回测。
- **历史数据下载实时流式进度条 (Real-Time Download Progress Bar)**：
  - 后端 SSE 流式推送协议与流水线回调：
    - `internal/datafeed/client.go` 与 `internal/datafeed/binance/client.go`：定义 `ProgressCallback`，在 Binance 每批 1000 根 K 线的拉取循环中动态计算进度百分比（0% ~ 80%）与当前已获取 K 线条数。
    - `internal/dataset/pipeline.go`：在数据清洗、防未来函数校验与 Parquet 分区落地阶段分别上报 86% 与 94% 进度，最后在 manifest 写入完成后触发 100% 结束包。
    - `internal/api/server.go`：`POST /api/v1/market/download` 支持 `stream=true` 参数与 `text/event-stream` 协议，通过 `http.Flusher` 实时下发 SSE 进度事件。
  - 前端流式消费与动态进度条组件：
    - `web/src/api.js`：实现 `downloadMarketDataStream`，利用 `ReadableStream` 增量消费并分发事件。
    - `web/src/data.js`：替换旧版静态 Spinner，构建三阶段流水线指示器（抓取 -> 清洗 -> 落地）、高精度动态百分比（0.0% ~ 100.0%）、已抓取行数统计与流光动画轨道，构建完成后平滑切入质量看板。
    - `web/styles.css`：实现深色金融终端规范的进度条、光晕动画与状态胶囊。
- **页面职责边界重构与行情回测调度中枢 (Page Responsibility Decoupling & Backtest Hub)**：
  - 核心职责解耦规范：
    - 数据页（`view-data`）：纯粹用于数据抓取与本地 Parquet 数据集浏览，彻底移除跨域跳页与侵入回测按钮。
    - 策略页（`view-strategies`）：纯粹用于可视化策略创建、指标配置、条件树编排与强类型 JSON 保存，彻底移除直接发起回测跳转。
    - 行情回测页（`view-backtest`）：升级为综合调度中枢，在 2x2 多周期图表上方配备专属控制栏，提供【行情数据源下拉框】与【策略选择下拉框】，支持自由组合任何数据集与策略。
  - 后端回测服务适配（`internal/backtest/service.go`）：
    - `backtest.Request` 扩展支持动态覆盖 `DatasetID`、`Symbol`、`StartMS` 与 `EndMS`，彻底解耦策略与特定静态数据集的硬编码绑定。
  - 回测绩效看板实时渲染：
    - 点击【🚀 执行回测】即刻完成撮合引擎运算，并在控制条右侧实时展示收益率、最大回撤、胜率、总交易笔数与最终权益。
- **指标自动绑定 K 线结构体与交互去冗余 (Indicator Inputs Auto-Inference)**：
  - 前端模态框净化（`web/src/strategy.js`）：
    - 彻底删除“输入价格字段”下拉框，用户无需手动选择任何价格列，消除认知负担。
    - 保存指标时由系统依据指标语义自省填充（ATR 自动绑定 `high/low/close`，EMA/SMA/RSI/BOLL 自动默认绑定 `close`）。
    - 指标概览卡片移除冗余的 `field: close` 视觉噪音，强化纯粹的量化参数属性呈现。
  - 后端规范化层自动补全（`internal/strategy/normalize.go`）：
    - 策略在 `Normalize` 阶段支持 `inputs` 为空时的自动推断与标准 K 线字段补齐，通过新单测 `TestValidateStrategyWithEmptyInputsAutoInferred` 确保极简策略定义 100% 兼容。
- **全局模块路径重命名 (Go Module Path Refactor)**：
  - 将根模块定义从 `github.com/bytedance/crypto-backtest` 全量重构为 `github.com/YuanJey/crypto-backtest`。
  - 同步原子更新 `cmd/` 下 3 个入口（`api`, `backtest`, `market-data`）及 `internal/` 下全部 23 个核心子包文件，全库 13 个包编译与单测 100% 通过。
- **回测数据源解析与显式仿真演示 (Backtest Dataset Resolution)**：
  - 公共仿真 K 线生成下沉（`internal/market/bar.go`）：
    - 提炼公开的 `market.GenerateSampleBars(symbol, startMS, endMS)` 函数，基于多谐波与趋势漂移算法提供 672 根 15m 逼真仿真 K 线，兼顾时间窗口与起止边界容错。
    - `internal/api/server.go` 统一接入该标准生成器。
  - 回测执行器智能自适应（`internal/backtest/service.go`）：
    - 显式支持 `dataset_id == "__sample__"` 仿真数据集回测。
    - 真实数据集读取失败时直接返回错误，禁止静默切换数据源；仿真数据仅在显式选择 `__sample__` 时使用。
  - 前端回测调度交互规范化（`web/src/app.js`）：
    - 数据集下拉框精确绑定 `data-start` 与 `data-end`；回测发起时显式传递 `__sample__` 或目标本地数据集 ID 与对应起止时间。
- **影响范围**：
  - `internal/market/bar.go`
  - `internal/api/server.go`
  - `internal/backtest/service.go`
  - `internal/backtest/service_test.go`
  - `web/src/app.js`
  - `README.md`
- **架构约束修正**：
  - 指标按依赖 DAG 拓扑编译并拒绝循环依赖，规则 AST 执行前完成布尔/数值类型检查。
  - 风险暴露、最大回撤停机、加仓次数、成交冷却和现货现金约束进入实际执行路径。
  - SQLite 策略保存改为不可变 revision，回测响应记录策略定义哈希与数据 manifest 哈希。
  - 真实数据读取失败不再静默降级到仿真数据；仿真回测必须显式选择 `__sample__`。
- **回测可视化增强**：
  - 多周期 K 线按指标所属周期叠加策略线，EMA/SMA/BOLL 使用价格区，RSI/ATR 使用副图区。
  - 新增回放同步资金曲线，当前权益、收益、回撤和持仓随 15m 驱动时钟逐点更新。
  - 指标和资金数据严格截断到当前回放 Bar，高周期形成中阶段只读取最近已完成周期值。
  - 界面统一为中性深色量化终端风格，压缩结果卡片层级并优化 1080px 与窄屏布局。
- **回测后资金曲线不刷新修复 (Equity Curve Visibility Fix)**：
  - 根因：`ReplayEngine.loadData` 默认游标停在索引 80，回测完成后仅 `emitTick()`，资金曲线按游标截断只显示前 81 点；策略预热期（1h EMA60 / 4h RSI14）内无成交，曲线恒为初始资金，表现为“修改策略后曲线不变”。
  - 修复：回测完成后执行 `replay.pause()` + `replay.seek(末尾)`，完整展示资金曲线与指标，同时保留按回放游标截断的防未来函数语义（拖动进度条或回放仍逐点可见）。
  - 影响范围：`web/src/app.js`、`README.md`。
- **资金曲线大样本栈溢出修复 (Equity Curve Stack Overflow Fix)**：
  - 根因：全量数据集回测（239,800 点）跳至回放末尾后，`Math.min(...equities)` 参数展开超出 V8 参数栈上限，抛出 `Maximum call stack size exceeded`。
  - `web/src/chart.js`：新增 `finiteExtent` 单次循环求最值，替换副图指标与资金曲线共 4 处展开调用；新增 `downsampleEquityPoints` 按像素桶保留 min/max 与首尾点，绘制点数由 239,800 降至约 1,500，极值与时间顺序无损。
  - `web/src/app.js`：回测返回后一次性预计算累计峰值 `peakEquity`（`Float64Array`），回放 tick 内回撤计算由 O(n) `slice + reduce` 降为 O(1)。
  - 验证：ETH 全量回测资金曲线全量/降采样最值一致，计算耗时 79ms。
  - 影响范围：`web/src/chart.js`、`web/src/app.js`、`README.md`。
- **资金曲线固定全局时间轴与生长模式 (Equity Curve Growth Mode)**：
  - 根因：原 X 轴分母使用当前回放点数（`visibleCount - 1`），导致任意时刻折线均被 100% 强行拉伸铺满全屏，单位时间物理像素持续变化，缺乏全局时间参照且纵轴网格缩放晃动。
  - `web/src/chart.js`：
    - X 轴坐标映射分母锁定为回测全生命周期总点数 `this.points.length - 1`，画布 100% 对应回测完整起止区间；
    - 折线随回放平滑向右生长，未走到的未来区间右侧留空；
    - Y 轴刻度锁定为全周期收益极值 `this.globalExtent`，消除回放中纵轴刻度的频繁跳动；
    - 当前点附带发光指示标记与垂直指示虚线，底轴呈现起始日期、结束日期与当前时刻标签。
  - 影响范围：`web/src/chart.js`、`README.md`。
