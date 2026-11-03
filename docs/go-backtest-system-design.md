# Go 回测系统与前端策略构建器设计

## 1. 结论

采用以下职责拆分：

```text
前端策略构建器 -> Go API / Worker -> Datafeed / Dataset -> Parquet + Manifest
                         |                 |
                         +-----------> Backtest Engine -> 回测结果与审计数据
```

- Go 是数据下载、数据清洗、策略定义、指标计算、规则编译、撮合、账户、任务调度和绩效计算的统一后端。
- 前端不生成代码，只根据服务端能力目录构建版本化 `StrategyDefinition` JSON。
- 前端通过 Go API 发起数据下载、数据校验、回测任务、进度查询和报告读取。
- Go 收到 JSON 后先反序列化、校验并编译，再执行回测；不能逐 K 线解释未经校验的动态对象。

该方案适合长期演进。Go 的类型约束、稳定性能、并发任务能力和单二进制部署方式适合同时承担前端控制面、数据流水线和回测执行核心。主要代价是 Go 的技术指标生态弱于脚本生态，因此指标必须有统一接口，并使用固定 golden fixture 做一致性测试。

详细的成交时间、K 线内冲突、资金预留和防未来函数语义继续遵循 [动态规则回测框架设计](./backtest-framework-design.md)。

## 2. 系统边界

### 2.1 Go 数据层

负责：

- 调用交易所 API 下载 OHLCV。
- 分页、限流、重试和断点续传。
- 去重、排序和基础 OHLC 校验。
- 写入统一 Parquet 数据集。
- 生成数据清单、缺口报告和文件校验和。
- 向 API / worker 暴露数据任务状态，支持前端触发和查询。

不负责：

- 计算策略指标。
- 产生交易信号。
- 处理手续费、滑点或持仓。
- 修改回测结果。

指标必须只在 Go 回测核心中计算，数据层只产出标准化 K 线和数据质量元信息，不能提前生成策略特征、信号或交易动作。

### 2.2 Go 服务

负责：

- 暴露指标、操作符和动作能力目录。
- 暴露数据集创建、校验、清单读取和缺口报告接口。
- 保存和版本化策略定义。
- 校验并编译策略。
- 加载 K 线并执行回测。
- 管理异步回测任务。
- 保存结果、运行元数据和审计事件。
- 向前端提供资金曲线、交易明细和指标。

### 2.3 前端

负责：

- 通过表单选择数据集、标的、周期和账户参数。
- 创建策略并添加指标，编辑指标初始化参数和指标周期。
- 通过嵌套条件组构建 `AND/OR/NOT` 规则，规则可引用 15m 与高周期指标。
- 配置开多、开空、平多、平空、仓位比例和保护性条件。
- 保存策略版本，并在执行回测时选择指定策略。
- 展示服务端结构校验和语义校验错误。
- 提交回测任务并展示进度与结果。

前端不能自行决定指标输出、操作符参数类型或默认成交语义，这些能力必须来自 Go 服务的目录接口。

## 3. 数据契约

### 3.1 Parquet K 线字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `open_time_ms` | int64 | UTC Unix 毫秒，表示 K 线开盘时间 |
| `open` | float64 | 开盘价 |
| `high` | float64 | 最高价 |
| `low` | float64 | 最低价 |
| `close` | float64 | 收盘价 |
| `volume` | float64 | 基础资产成交量 |
| `quote_volume` | float64，可选 | 计价资产成交量 |
| `trade_count` | int64，可选 | 成交笔数 |
| `state` | string | K 线状态，历史数据必须为 `closed` |

交易所、标的和周期放在分区路径及 manifest 中，不在每一行重复存储：

```text
data/market/
└── binance-spot-15m-20250101-20250201/
    ├── exchange=binance/
    │   └── symbol=BTC-USDT/
    │       └── timeframe=15m/
    │           ├── year=2025/month=01/part-000.parquet
    │           └── year=2025/month=02/part-000.parquet
    ├── gap_report.json
    └── manifest.json
```

### 3.2 数据清单

每个数据集必须有 `manifest.json`：

```json
{
  "schema_version": 1,
  "dataset_id": "binance-spot-2025-01",
  "exchange": "binance",
  "market_type": "spot",
  "symbols": ["BTC-USDT"],
  "timeframes": ["15m"],
  "start_ms": 1735689600000,
  "end_ms": 1738368000000,
  "row_count": 2976,
  "missing_bars": 0,
  "generated_at": "2026-10-12T08:00:00Z",
  "market_rules": {
    "BTC-USDT": {
      "price_tick": 0.01,
      "quantity_step": 0.00001,
      "min_quantity": 0.00001,
      "min_notional": 5
    }
  },
  "files": [
    {
      "path": "exchange=binance/symbol=BTC-USDT/timeframe=15m/year=2025/month=01/part-000.parquet",
      "sha256": "...",
      "row_count": 2976,
      "start_ms": 1735689600000,
      "end_ms": 1738368000000
    }
  ]
}
```

Go 回测任务保存 manifest 哈希，保证以后可以确认某次回测使用的准确数据版本。

### 3.3 Go 内部 K 线

```go
type Bar struct {
	OpenTimeMS  int64
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Volume      float64
	QuoteVolume float64
	TradeCount  int64
}
```

价格和数量使用 `float64` 计算，但所有下单数量和成交价格必须根据 `quantity_step`、`price_tick` 做确定性量化。首版不在热路径使用 `decimal`。

## 4. 策略结构体

`StrategyDefinition` 是前端、API、数据库和 Go 编译器之间的唯一策略契约。

### 4.1 顶层定义

```go
type StrategyDefinition struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Data          DataSelection   `json:"data"`
	Account       AccountSpec     `json:"account"`
	Behavior      BehaviorSpec    `json:"behavior"`
	Position      PositionSpec    `json:"position,omitempty"`
	Execution     ExecutionSpec   `json:"execution"`
	Risk          RiskSpec        `json:"risk"`
	Indicators    []IndicatorSpec `json:"indicators"`
	Rules         []RuleSpec      `json:"rules"`
}

type DataSelection struct {
	DatasetID string    `json:"dataset_id"`
	Symbols   []string  `json:"symbols"`
	Timeframe string    `json:"timeframe"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
}

type AccountSpec struct {
	MarketType      MarketType `json:"market_type"`
	InitialCash     float64    `json:"initial_cash"`
	QuoteCurrency   string     `json:"quote_currency"`
	ExchangeLeverage float64   `json:"exchange_leverage,omitempty"`
}

type BehaviorSpec struct {
	AllowPyramiding      bool `json:"allow_pyramiding"`
	AllowSameBarReversal bool `json:"allow_same_bar_reversal"`
	CooldownBars         int  `json:"cooldown_bars"`
}

type PositionSpec struct {
	InitialSizePct     float64 `json:"initial_size_pct,omitempty"`
	MaxAdditions      int     `json:"max_additions,omitempty"`
	AddSizePct        float64 `json:"add_size_pct,omitempty"`
	AddSizeMultiplier float64 `json:"add_size_multiplier,omitempty"`
	PositionLeverage  float64 `json:"position_leverage,omitempty"`
	LongCapitalDecayPower  float64 `json:"long_capital_decay_power,omitempty"`
	ShortCapitalDecayPower float64 `json:"short_capital_decay_power,omitempty"`
}

type ExitPolicySpec struct {
	ID           string           `json:"id"`
	Side         TradeSide        `json:"side"`
	Purpose      ExitPurpose      `json:"purpose"`
	Anchor       ExitAnchorSpec   `json:"anchor"`
	Distance     ExitDistanceSpec `json:"distance"`
	TriggerPrice ExitTriggerPrice `json:"trigger_price,omitempty"`
	ClosePct     float64          `json:"close_pct,omitempty"`
	Priority     int              `json:"priority,omitempty"`
	Execution    ExitExecution    `json:"execution,omitempty"`
}

type ExecutionSpec struct {
	SignalTime       SignalTime       `json:"signal_time"`
	MarketFillTime   MarketFillTime   `json:"market_fill_time"`
	IntrabarConflict IntrabarConflict `json:"intrabar_conflict"`
	Fee              FeeSpec          `json:"fee"`
	Slippage         SlippageSpec     `json:"slippage"`
}

type FeeSpec struct {
	MakerRate float64 `json:"maker_rate"`
	TakerRate float64 `json:"taker_rate"`
}

type SlippageSpec struct {
	Type  SlippageType `json:"type"`
	Value float64      `json:"value"`
}

type RiskSpec struct {
	MaxOpenPositions   int     `json:"max_open_positions"`
	MaxDrawdownHaltPct float64 `json:"max_drawdown_halt_pct"`
}
```

`BehaviorSpec.AllowPyramiding` 是是否允许同向加仓的唯一权威来源。为兼容已保存的
schema v1 策略，后端仍接受旧的 `position.allow_pyramiding`，但会在规范化阶段迁移
到 `behavior.allow_pyramiding`，编译计划中不保留第二份开关。
旧的 `position.target_leverage` 会迁移为 `position.position_leverage`；
`position.max_position_pct`、`risk.max_symbol_equity_pct` 和
`risk.max_total_exposure_pct` 只用于解码历史版本，不再形成独立运行时上限。

`Start` 包含在回测区间内，`End` 不包含在回测区间内，统一采用 `[start, end)` 语义。

枚举统一定义为有固定白名单的字符串类型：

```go
type MarketType string

const (
	MarketTypeSpot      MarketType = "spot"
	MarketTypePerpetual MarketType = "perpetual"
)
```

不能接受未识别的枚举值，也不能在业务代码中散落字符串比较。

### 4.2 指标定义

```go
type IndicatorSpec struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Timeframe string       `json:"timeframe,omitempty"`
	Inputs []ValueRef      `json:"inputs"`
	Params json.RawMessage `json:"params"`
}

type ValueRef struct {
	Scope  ValueScope `json:"scope"`
	Name   string     `json:"name"`
	Field  string     `json:"field"`
	Offset int        `json:"offset,omitempty"`
}
```

示例：

```json
{
  "id": "boll_20",
  "type": "boll",
  "timeframe": "15m",
  "inputs": [
    {
      "scope": "market",
      "name": "bar",
      "field": "close"
    }
  ],
  "params": {
    "period": 20,
    "stddev": 2
  }
}
```

`Params` 只在传输结构中使用 `json.RawMessage`。指标工厂收到后必须解码到自己的强类型参数：

```go
type EMAParams struct {
	Period int `json:"period"`
}

type BollParams struct {
	Period int     `json:"period"`
	StdDev float64 `json:"stddev"`
}

type SupertrendParams struct {
	Period     int     `json:"period"`
	Multiplier float64 `json:"multiplier"`
}
```

Supertrend 使用 `high/low/close` 与 Wilder ATR 递推，输出趋势线 `value` 和方向
`direction`；方向值 `1` 表示多头趋势，`-1` 表示空头趋势。默认前端参数为
`period=10`、`multiplier=3`，服务端要求 `period >= 1`、`multiplier > 0`。

指标输出还必须声明语义和可交叉分组：

```go
type OutputDefinition struct {
	Name       string         `json:"name"`
	Type       ValueType      `json:"type"`
	Semantic   OutputSemantic `json:"semantic"`
	CrossGroup string         `json:"cross_group,omitempty"`
}
```

- EMA/SMA 属于 `price_series + moving_average`，可与同组均线形成金叉/死叉。
- RSI 属于 `oscillator_series + rsi`，只有两个 RSI 输出之间可交叉。
- BOLL、ATR 没有 `cross_group`，不提供金叉/死叉。
- Supertrend `direction` 是 `trend_direction`，只允许等于 `1/-1`。
- Supertrend `value` 是 `trend_level`，与市场价格构成支撑/阻力关系。
- `turn_bullish`、`turn_bearish` 仅接受 `trend_direction` 输出，通过上一根与当前
  方向值判断趋势切换；首个有效值和 warm-up 阶段统一返回 `false`。

引擎内部不得持续传递 `map[string]any`。

`timeframe` 为空时默认使用策略数据基准周期 `15m`。若显式配置，必须大于等于基准周期并且是 `15m` 的整数倍，例如 `15m`、`1h`、`4h`、`1d`。高周期指标只能在该高周期 K 线完整收盘后对低周期规则可见。

### 4.3 策略文件与回测选择

前端主流程：

```text
创建策略 -> 添加指标并设置参数/周期 -> 配置开多/开空/平仓规则 -> 保存策略到 SQLite -> 选择策略和 ReportSpec 执行回测 -> 展示结果
```

首版服务使用 SQLite 保存策略定义，只保存策略 JSON 和基础元数据，不保存回测结果文件。回测请求通过 `strategy_id` 选择已创建策略，并携带本次 report 配置。后续多实例部署时可以替换为 PostgreSQL，但策略定义结构、校验规则和编译入口保持不变。

常驻服务入口：

```bash
go run ./cmd/api -addr :8080 -db data/app.db -data-root data/market
```

核心 API：

```http
GET  /api/v1/catalog
POST /api/v1/strategies
GET  /api/v1/strategies
GET  /api/v1/strategies/{id}
POST /api/v1/backtests
```

`POST /api/v1/backtests` 请求示例：

```json
{
  "strategy_id": "ema-boll-rsi",
  "strategy_revision": 2,
  "report": {
    "equity_curve": true,
    "orders": true,
    "fills": true,
    "metrics": ["final_equity", "total_return_pct", "max_drawdown_pct"],
    "indicators": [
      {"id": "ema_fast_15m", "fields": ["value"]},
      {"id": "rsi_14_4h", "fields": ["value"]}
    ]
  }
}
```

`strategy_revision` 可选；省略时读取最新 revision。SQLite 每次保存语义发生变化的
同 ID 策略时追加不可变 revision，并记录规范化定义的 SHA256；重复保存相同定义
不会制造新 revision。

CLI 入口保留为开发调试工具。默认只完成解码、校验和编译：

```bash
go run ./cmd/backtest -strategy-id ema-boll-rsi
go run ./cmd/backtest -db data/app.db -strategy-id ema-boll-rsi
go run ./cmd/backtest -db data/app.db -strategy-id ema-boll-rsi -strategy-revision 2
```

执行回测时增加 `-run`，Go 会按策略里的 `dataset_id`、标的、时间范围从 `data/market` 读取 Parquet K 线：

```bash
go run ./cmd/backtest -strategy-id ema-boll-rsi -run -data-root data/market
go run ./cmd/backtest -db data/app.db -strategy-id ema-boll-rsi -run -data-root data/market
```

报告输出不属于策略定义本身，而属于本次回测请求。同一个策略可以用不同报告视图执行：

```go
type BacktestRequest struct {
	Bars   []market.Bar `json:"-"`
	Report ReportSpec  `json:"report"`
}

type ReportSpec struct {
	EquityCurve bool               `json:"equity_curve,omitempty"`
	Orders      bool               `json:"orders,omitempty"`
	Fills       bool               `json:"fills,omitempty"`
	Metrics     []MetricType       `json:"metrics,omitempty"`
	Indicators  []IndicatorOverlay `json:"indicators,omitempty"`
}

type IndicatorOverlay struct {
	ID     string   `json:"id"`
	Fields []string `json:"fields,omitempty"`
}
```

服务层响应必须同时返回本次实际使用的数据来源：

```go
type DataProvenance struct {
	DatasetID      string `json:"dataset_id"`
	Symbol         string `json:"symbol"`
	Timeframe      string `json:"timeframe"`
	StartMS        int64  `json:"start_ms"`
	EndMS          int64  `json:"end_ms"`
	ManifestSHA256 string `json:"manifest_sha256,omitempty"`
	Synthetic      bool   `json:"synthetic"`
}
```

真实数据集读取失败必须终止回测，不能静默降级为仿真数据。只有请求显式指定
`dataset_id="__sample__"` 时才允许使用确定性合成 K 线。

回测响应还必须包含：

```go
type RevisionMetadata struct {
	Revision         int    `json:"revision"`
	DefinitionSHA256 string `json:"definition_sha256"`
}
```

CLI 可用 `-report` 选择输出段，`-report-indicator` 选择具体指标曲线：

```bash
go run ./cmd/backtest -strategy-id ema-boll-rsi -run \
  -report summary,equity,orders,fills,indicators \
  -report-indicator ema_fast_15m.value,rsi_14_4h.value
```

加载顺序：

```text
strategy_id -> FileRepository.LoadByID -> strategy.Decode -> strategy.Validate -> compiler.Compile -> engine.Run
```

`strategy.Validate` 必须完成：

- 策略 ID、schema 版本和数据选择校验。
- 指标 ID 唯一性校验。
- 指标类型存在性校验。
- 指标 `params` 严格解码和参数范围校验。
- 指标 `timeframe` 必须是 15m 基准周期的整数倍。
- 指标依赖必须组成无环 DAG，并按拓扑顺序生成执行计划。
- `ValueRef.Offset <= 0`，禁止正向未来偏移。
- 规则引用的指标 ID 和输出字段必须存在。
- 规则根节点必须是布尔类型，逻辑和数值操作符的参数类型必须匹配。
- MVP 数据选择必须且只能包含一个标的，`max_open_positions` 必须为 `1`。
- 现货账户禁止配置 `short` 方向动作，开空需要 `perpetual` 市场类型。

### 4.4 表达式 AST

条件使用递归表达式树，前端可直接用树形控件构建：

```go
type ExprKind string

const (
	ExprOperator ExprKind = "operator"
	ExprReference ExprKind = "reference"
	ExprNumber    ExprKind = "number"
	ExprBoolean   ExprKind = "boolean"
)

type Expression struct {
	Kind    ExprKind     `json:"kind"`
	Op      Operator     `json:"op,omitempty"`
	Args    []Expression `json:"args,omitempty"`
	Ref     *ValueRef    `json:"ref,omitempty"`
	Number  *float64     `json:"number,omitempty"`
	Bool    *bool        `json:"bool,omitempty"`
}
```

结构校验必须保证每个节点只有一种有效形态：

- `operator`：必须有合法 `op` 和符合签名的 `args`。
- `reference`：必须有 `ref`。
- `number`：必须有 `number`，包括合法的 `0`。
- `boolean`：必须有 `bool`。

使用指针保存常量是为了区分“未提供”和零值。

表达式示例：

```json
{
  "kind": "operator",
  "op": "and",
  "args": [
    {
      "kind": "operator",
      "op": "gt",
      "args": [
        {
          "kind": "reference",
          "ref": {
            "scope": "indicator",
            "name": "ema_fast_15m",
            "field": "value"
          }
        },
        {
          "kind": "reference",
          "ref": {
            "scope": "indicator",
            "name": "ema_slow_1h",
            "field": "value"
          }
        }
      ]
    },
    {
      "kind": "operator",
      "op": "lt",
      "args": [
        {
          "kind": "reference",
          "ref": {
            "scope": "indicator",
            "name": "rsi_14_4h",
            "field": "value"
          }
        },
        {
          "kind": "number",
          "number": 70
        }
      ]
    }
  ]
}
```

### 4.5 规则和动作

```go
type RuleSpec struct {
	ID       string     `json:"id"`
	Name     string     `json:"name,omitempty"`
	Disabled bool       `json:"disabled,omitempty"`
	Priority int        `json:"priority,omitempty"`
	Condition string    `json:"condition,omitempty"`
	When     Expression `json:"when"`
	Action   ActionSpec `json:"action"`
}

type ActionSpec struct {
	Type    ActionType `json:"type"`
	Side    TradeSide  `json:"side"`
	SizePct float64    `json:"size_pct"`
}
```

推荐动作模型：

| `type` | 必填字段 | 说明 |
| --- | --- | --- |
| `open` | `side`、`size_pct` | 开多或开空 |
| `close` | `side`、`size_pct` | 平多或平空 |

`open_long`、`close_long` 等名称适合作为 UI 文案，不建议在底层定义为多个动作类型。统一的动作加 `side` 更容易扩展。保护性止损、止盈和减仓将作为 `ActionSpec` 的后续字段扩展。

`ActionSpec.size_pct` 是规则级覆盖值。若开仓 action 不提供 `size_pct`，engine 使用策略级 `PositionSpec`：

- 首次开仓用 `initial_size_pct`。
- 加仓优先用 `add_size_pct`。
- 若未配置 `add_size_pct` 且配置 `add_size_multiplier`，使用
  `first_entry_notional * add_size_multiplier^entry_count`。
- `max_additions` 限制当前持仓轮次内的加仓次数，不包含首次开仓。
- 首仓基础名义金额为 `min(equity, equity * size_pct * position_leverage)`。
- 根据开仓方向使用 `long_capital_decay_power` 或 `short_capital_decay_power`：
  `decay = min(1, (initial_cash / equity) ^ power)`，实际首仓名义金额为基础值乘以
  `decay`。默认幂次为 `0.5`；盈利后缩仓，亏损时不放大。
- 规则级或 `add_size_pct` 固定加仓只按当前权益比例计算，不重复应用
  `position_leverage`。
- 倍增加仓以本轮首次订单的实际名义金额为基数，因此自动继承
  `position_leverage` 和资金衰减的缩放结果。
- `exchange_leverage` 是总名义仓位的唯一杠杆上限；`position_leverage`
  用于首仓规模计算，二者未配置时均按 `1x` 兼容。
- 达到 `risk.max_drawdown_halt_pct` 后禁止新开仓，但仍允许平仓。
- 每个资金曲线点返回 `position_leverage = abs(position_notional) / equity`，并提供
  `max_position_leverage` 报告指标。该实际杠杆是动态结果，不是策略配置参数。
- `exit_policies` 是独立的持仓退出层。价格锚点、距离、指标采样、触发字段与执行
  方式正交组合，避免为每一种止盈止损新增动作类型。
- ATR 采样 `entry` 使用首次开仓前最新的已闭合指标值；`each_entry` 在每次成交
  加仓后刷新；`live` 每根闭合 K 线刷新并从下一根 K 线起生效，不使用未来数据。
- `trigger_price` 执行在 K 线触及目标时成交；发生跳空时使用该根 K 线开盘价。
- 同根 K 线有多个退出策略命中时，按 `priority` 从高到低选择一个；同优先级采用
  保守原则让止损优先。

规则条件可以直接保存为 AST，也可以保存为公共链式表达式字符串，由后端统一解析为 AST：

```json
{
  "id": "open_long",
  "condition": "and(ema_fast_15m.gte(ema_slow_1h), rsi_14_4h.lt(70))",
  "action": {
    "type": "open",
    "side": "long",
    "size_pct": 0.2
  }
}
```

链式表达式规则：

- `ema_15m.gte(2000)` 等价于引用 `indicator.ema_15m.value >= 2000`。
- `boll_15m.upper.gte(market.close)` 可引用指定指标输出字段。
- `ema_fast_15m.cross_up(ema_slow_1h)` 表示两个不同指标输出形成金叉。
- `cross_up`、`cross_down` 的两个参数必须都是指标引用，不能与常数或自身交叉。
- `position.flat()`、`position.long()`、`position.short()` 可在指标条件前显式判断当前持仓状态。
- `position.quantity.gte(1)`、`position.abs_quantity.gt(0)` 可引用当前持仓数量状态。
- `position.entry_count.lt(3)` 可限制当前持仓轮次内的加仓次数。
- `position.avg_entry_price`、`position.last_entry_price`、`position.unrealized_pnl_pct` 可用于加仓、止损或退出条件。
- 顶层支持 `and(...)`、`or(...)`、`not(...)`。
- 若同时提交 `condition` 和 `when`，服务端拒绝，避免一条规则有两套条件来源。

当前 engine 还保留执行层安全门控：当 `behavior.allow_pyramiding=false` 且当前已有同向持仓时，即使规则信号再次满足也不会继续开仓。策略层推荐显式写状态条件，例如：

```json
{
  "id": "open_long",
  "condition": "and(position.flat(), ema_fast_15m.gte(ema_slow_1h))",
  "action": {"type": "open", "side": "long", "size_pct": 0.2}
}
```

`behavior.cooldown_bars` 从最近一次成交所在的基础 K 线开始计数，冷却窗口内禁止
新开仓但不阻止平仓。当前 next-bar-open 引擎不支持同 Bar 反转，因此
`behavior.allow_same_bar_reversal=true` 会在校验阶段被拒绝，而不是静默忽略。

分层加仓逻辑使用同一状态模型，包括 `position.entry_count`、`position.last_entry_price`、`position.unrealized_pnl_pct` 等字段。它不代表系统内置了某种特定资金管理策略。

分层加仓示例：

```json
{
  "id": "add_long",
  "condition": "or(position.flat(), and(position.long(), position.entry_count.lt(3), position.avg_entry_price.gte(market.close)))",
  "action": {"type": "open", "side": "long"}
}
```

### 4.6 策略 JSON 示例

```json
{
  "schema_version": 1,
  "id": "ema-boll-rsi",
  "name": "EMA BOLL RSI",
  "data": {
    "dataset_id": "binance-perpetual-15m-202501",
    "symbols": ["BTC-USDT"],
    "timeframe": "15m",
    "start": "2025-01-01T00:00:00Z",
    "end": "2025-02-01T00:00:00Z"
  },
  "account": {
    "market_type": "perpetual",
    "initial_cash": 100000,
    "quote_currency": "USDT"
  },
  "behavior": {
    "allow_pyramiding": false,
    "allow_same_bar_reversal": false,
    "cooldown_bars": 1
  },
  "execution": {
    "signal_time": "bar_close",
    "market_fill_time": "next_bar_open",
    "intrabar_conflict": "conservative",
    "fee": {
      "maker_rate": 0.001,
      "taker_rate": 0.001
    },
    "slippage": {
      "type": "bps",
      "value": 2
    }
  },
  "risk": {
    "max_open_positions": 1,
    "max_drawdown_halt_pct": 0.25
  },
  "indicators": [
    {
      "id": "ema_fast_15m",
      "type": "ema",
      "timeframe": "15m",
      "inputs": [
        {
          "scope": "market",
          "name": "bar",
          "field": "close"
        }
      ],
      "params": {
        "period": 20
      }
    },
    {
      "id": "ema_slow_1h",
      "type": "ema",
      "timeframe": "1h",
      "inputs": [
        {
          "scope": "market",
          "name": "bar",
          "field": "close"
        }
      ],
      "params": {
        "period": 60
      }
    },
    {
      "id": "rsi_14_4h",
      "type": "rsi",
      "timeframe": "4h",
      "inputs": [
        {
          "scope": "market",
          "name": "bar",
          "field": "close"
        }
      ],
      "params": {
        "period": 14
      }
    }
  ],
  "rules": [
    {
      "id": "open_long",
      "name": "Open long on trend confirmation",
      "priority": 100,
      "when": {
        "kind": "operator",
        "op": "and",
        "args": [
          {
            "kind": "operator",
            "op": "gt",
            "args": [
              {
                "kind": "reference",
                "ref": {
                  "scope": "indicator",
                  "name": "ema_fast_15m",
                  "field": "value"
                }
              },
              {
                "kind": "reference",
                "ref": {
                  "scope": "indicator",
                  "name": "ema_slow_1h",
                  "field": "value"
                }
              }
            ]
          },
          {
            "kind": "operator",
            "op": "lt",
            "args": [
              {
                "kind": "reference",
                "ref": {
                  "scope": "indicator",
                  "name": "rsi_14_4h",
                  "field": "value"
                }
              },
              {
                "kind": "number",
                "number": 70
              }
            ]
          }
        ]
      },
      "action": {
        "type": "open",
        "side": "long",
        "size_pct": 0.2
      }
    }
  ]
}
```

## 5. 指标注册机制

### 5.1 服务端定义

```go
type IndicatorDefinition struct {
	Type        string            `json:"type"`
	Label       string            `json:"label"`
	Category    string            `json:"category"`
	Inputs      []InputDefinition `json:"inputs"`
	Params      []ParamDefinition `json:"params"`
	Outputs     []OutputDefinition `json:"outputs"`
}

type IndicatorFactory interface {
	Definition() IndicatorDefinition
	Build(params json.RawMessage) (RuntimeIndicator, error)
}

type RuntimeIndicator interface {
	Warmup() int
	Update(inputs []float64) []float64
	Reset()
}
```

指标采用逐 K 线增量计算：

- 天然只能读取当前和历史值，降低未来函数风险。
- 同一套运行逻辑以后可用于模拟盘或实盘。
- 每个回测任务持有独立指标实例，不共享可变状态。
- 指标 DAG 决定每根 K 线的计算顺序。

首版内置 EMA、SMA、BOLL、RSI、ATR。所有指标都用固定 golden fixture 与可信参考结果对比。

### 5.2 能力目录

```http
GET /api/v1/catalog
```

响应包含：

```json
{
  "catalog_version": 1,
  "indicators": [
    {
      "type": "ema",
      "label": "EMA",
      "category": "trend",
      "inputs": [
        {
          "name": "source",
          "value_type": "number_series",
          "required": true
        }
      ],
      "params": [
        {
          "name": "period",
          "value_type": "integer",
          "required": true,
          "default": 20,
          "min": 1,
          "max": 10000
        }
      ],
      "outputs": [
        {
          "name": "value",
          "value_type": "number_series"
        }
      ]
    }
  ],
  "operators": [
    {
	      "type": "cross_up",
      "args": ["indicator_series", "indicator_series"],
      "returns": "boolean"
    }
  ],
  "actions": []
}
```

前端根据该目录动态生成参数表单和可选引用。新增指标时，后端注册后即可出现在前端，不需要前端为每个指标编写独立表单。

## 6. 前端构建流程

建议页面结构：

```text
策略基本信息
├── 数据集 / 标的 / 周期 / 时间范围
├── 账户与撮合
├── 指标
│   ├── EMA20 15m
│   ├── EMA60 1h
│   └── RSI14 4h
├── 规则
│   ├── 入场规则
│   │   └── AND
│   │       ├── EMA20 15m > EMA60 1h
│   │       └── RSI14 4h < 70
│   ├── 开空规则
│   └── 平仓规则
├── 风控
└── 运行回测
```

交互约束：

- 条件组使用树结构，支持添加条件、添加组、切换 `AND/OR` 和删除。
- 左值选定后，根据类型过滤可用操作符和右值。
- 指标引用只能选择当前策略已添加指标的已声明输出。
- 现货模式不显示开空动作。
- `risk_pct` 仓位模式必须配置止损。
- 前端做即时校验，但服务端始终重新校验。
- 服务端错误返回字段路径，前端定位到具体控件。

前端状态直接维护 `StrategyDefinition`，提交前不再做另一套格式转换，减少 UI 模型和执行模型不一致。

## 7. 校验与编译

请求进入 Go 后依次执行：

```text
JSON
 -> Strict Decode
 -> Structural Validation
 -> Catalog/Semantic Validation
 -> Indicator DAG Compile
 -> Expression Type Compile
 -> Execution Plan
 -> Backtest Engine
```

### 7.1 严格解码

使用 `json.Decoder.DisallowUnknownFields()`。未知字段必须报错，避免前端拼错字段后被静默忽略。

### 7.2 结构校验

- 必填字段、数值范围和枚举值。
- ID 格式与唯一性。
- 每个表达式节点的联合类型是否合法。
- 操作符参数数量是否正确。
- 动作所需字段是否完整。

### 7.3 语义校验

- 数据集是否包含所选标的、周期和时间范围。
- 指标类型、参数和输出是否存在。
- 指标依赖是否成环。
- 表达式左右值类型是否匹配。
- 引用是否存在，历史偏移是否小于等于 0。
- 现货是否尝试开空。
- 仓位和风控参数是否互相冲突。

错误格式：

```json
{
  "valid": false,
  "errors": [
    {
      "code": "unknown_indicator_output",
      "path": "/rules/0/when/args/1/ref/field",
      "message": "indicator rsi_14_4h has no output named upper"
    }
  ]
}
```

### 7.4 编译结果

配置不能直接进入交易循环。编译器需要：

1. 将字符串指标引用解析成整数 `SlotID`。
2. 对指标拓扑排序并计算总 warm-up。
3. 将表达式操作符转换为内部 `OpCode`。
4. 预先检查操作数类型。
5. 按优先级稳定排序规则。
6. 生成不可变 `CompiledStrategy`。

```go
type CompiledStrategy struct {
	DefinitionHash string
	WarmupBars     int
	Indicators     []CompiledIndicator
	Rules          []CompiledRule
}

type CompiledRule struct {
	ID       string
	Priority int
	Position PositionFilter
	Root     CompiledNodeID
	Action   CompiledAction
}
```

交易循环只处理整数槽位和已验证节点，避免每根 K 线重复解析字符串、查注册表或检查参数。

## 8. API 设计

### 8.1 策略

```text
GET    /api/v1/catalog
POST   /api/v1/strategies/validate
POST   /api/v1/strategies
GET    /api/v1/strategies/{id}
POST   /api/v1/strategies/{id}/revisions
GET    /api/v1/strategies/{id}/revisions/{revision}
```

策略保存后采用不可变 revision。修改策略创建新 revision，历史回测始终能找到当时的原始定义。

### 8.2 回测任务

```text
POST /api/v1/backtests
GET  /api/v1/backtests/{job_id}
GET  /api/v1/backtests/{job_id}/summary
GET  /api/v1/backtests/{job_id}/equity
GET  /api/v1/backtests/{job_id}/trades
GET  /api/v1/backtests/{job_id}/orders
GET  /api/v1/backtests/{job_id}/events
```

创建任务：

```json
{
  "strategy_id": "ema-boll-rsi",
  "strategy_revision": 3
}
```

任务状态：

```text
queued -> validating -> loading_data -> running -> persisting -> completed
                                                   \-> failed
                                                   \-> cancelled
```

进度首版可轮询，后续再增加 SSE。不要让 HTTP 请求一直阻塞到回测完成。

## 9. Go 工程目录

```text
crypto-backtest/
├── cmd/
│   ├── api/
│   │   └── main.go
│   ├── worker/
│   │   └── main.go
│   ├── backtest/
│   │   └── main.go
│   └── market-data/
│       └── main.go
├── internal/
│   ├── api/
│   ├── datafeed/
│   │   ├── client.go
│   │   └── binance/
│   ├── dataset/
│   │   ├── cleaner.go
│   │   ├── manifest.go
│   │   ├── pipeline.go
│   │   └── writer.go
│   ├── market/
│   │   ├── bar.go
│   │   ├── symbol.go
│   │   └── timeframe.go
│   ├── strategy/
│   │   ├── model.go
│   │   ├── validate.go
│   │   └── repository.go
│   ├── catalog/
│   ├── compiler/
│   ├── indicator/
│   │   ├── registry.go
│   │   └── builtin.go
│   ├── rule/
│   ├── engine/
│   ├── execution/
│   ├── portfolio/
│   ├── risk/
│   ├── reporting/
│   └── job/
├── web/
├── testdata/
├── strategies/
│   ├── README.md
│   └── ema-boll-rsi.json
└── docs/
```

边界要求：

- `internal/strategy` 只定义持久化和 API 使用的策略模型。
- `internal/datafeed` 只负责交易所原始 K 线获取和分页重试。
- `internal/dataset` 只负责清洗校验、缺口报告、Parquet 分区和 manifest。
- `internal/market` 存放行情领域基础类型，供数据层和回测层共同使用。
- `internal/compiler` 将策略模型转换为运行模型。
- `internal/engine` 不依赖 HTTP、数据库或前端模型；只接收编译计划和已加载的 15m K 线。
- `internal/api` 编排 SQLite 策略仓库、策略编译、数据读取和回测执行，不把 HTTP 逻辑扩散到 engine。
- `internal/indicator` 不依赖账户与撮合。
- `internal/execution` 只通过订单、成交和市场规则交互。

当前 engine MVP：

- 按 15m K 线顺序推进。
- 基础 K 线必须是 `closed` 状态，显式 `forming` K 线不能进入回测规则。
- 本根 K 线收盘后更新指标并评估规则。
- 市价动作最早在下一根 K 线开盘成交。
- 高周期指标由 15m 聚合；聚合窗口内部是 `forming`，只有完整高周期 K 线转为 `closed` 后才更新可见值。
- 首版支持单标的、单账户、`open/close + long/short + size_pct` 动作。

MVP 可以让 API 和 worker 在同一进程运行，但代码边界需要从第一版分开，后续才能独立扩容回测 worker。

## 10. 持久化与可复现性

策略版本保存（当前 SQLite 仓储已实现）：

- 原始 JSON。
- 规范化 JSON。
- 定义哈希。
- schema 版本。
- catalog 版本。
- 创建时间。

每次回测保存：

- 策略 ID、revision 和定义哈希。
- 数据集 ID 和 manifest 哈希。
- Go 引擎版本和 Git commit。
- 成交、滑点和 K 线内冲突模型。
- 开始、完成时间和任务状态。
- 订单、成交、交易、资金曲线和规则命中事件。

MVP 可使用 SQLite 保存策略和任务元数据，结果明细保存为 Parquet/JSON 文件。多实例部署时再迁移到 PostgreSQL 和对象存储，不让存储选型进入引擎核心。

## 11. 测试

### 11.1 Go 单元测试

- 指标、操作符、规则编译器。
- 手续费、滑点、价格和数量量化。
- 持仓平均成本、部分平仓和账户守恒。
- 严格 JSON 解码和每条校验规则。

### 11.2 数据契约与存储测试

- Go 写出的 Parquet 由 Go 数据层读取并逐字段对比。
- manifest 文件数、行数和 SHA256 一致。
- 时间戳、空值和可选列覆盖边界场景。
- 下载分页、重复 K 线、乱序 K 线、非法 OHLC 和缺口检测覆盖边界场景。

### 11.3 指标一致性测试

- 使用固定输入和预期输出的 golden fixture 验证指标实现。
- Go 测试只读取固定 fixture，不在测试运行时依赖外部脚本或网络。
- 明确比较 warm-up、NaN、舍入和边界值。

### 11.4 端到端测试

- 前端示例 JSON通过校验。
- 编译产出固定定义哈希和 warm-up。
- 5 至 20 根人工 K 线得到固定订单与成交序列。
- 修改未来 K 线不能改变此前信号和订单。

## 12. 实施顺序

### Phase 1：先打通无前端内核

1. 定义 Go 结构体、严格 JSON 解码和校验错误模型。
2. 实现 Go `market-data` CLI、Binance 下载器、清洗器和 Parquet/manifest 契约。
3. 实现 EMA、BOLL、RSI、ATR 与指标注册表。
4. 实现表达式编译器、单标的现货和市价单撮合。
5. 使用固定 JSON 策略通过 CLI 完成端到端回测。

验收：同一配置和数据重复运行，订单、成交和资金曲线完全一致。

### Phase 2：接入前端构建器

1. 实现 catalog 和 strategy validate API。
2. 实现指标编辑器和条件树。
3. 实现策略 revision 和异步回测任务。
4. 展示资金曲线、指标、交易和规则命中详情。

验收：用户不编辑 JSON，也能构建并运行 EMA+BOLL+RSI 策略。

### Phase 3：扩展交易能力

- 多标的共享账户。
- 多周期指标。
- 开空、永续合约、资金费和强平。
- 限价单、部分成交和成交量冲击。
- 参数搜索和多 worker 并发。

## 13. 必须避免的实现方式

- 前端和 Go 后端各自硬编码一份指标参数。
- 用字符串 `eval` 或脚本执行策略条件。
- 在逐 K 线循环中反复解析 JSON。
- 用 `map[string]any` 贯穿编译器和执行引擎。
- 由数据下载清洗链路提前计算策略指标或交易信号。
- 修改已有策略 revision，导致历史回测不可复现。
- 当前 K 线收盘产生信号后又默认按当前收盘价成交。
- 只保存最终绩效，不保存订单、成交和规则命中审计。
