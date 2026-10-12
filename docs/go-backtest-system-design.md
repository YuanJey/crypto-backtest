# Go 回测系统与前端策略构建器设计

## 1. 结论

采用以下职责拆分：

```text
Python 数据工具 -> Parquet + Manifest -> Go 回测服务 <- JSON -> 前端策略构建器
                                              |
                                       回测结果与审计数据
```

- Python 只负责从交易所下载、清洗和分区存储历史 K 线。
- Go 是策略定义、指标计算、规则编译、撮合、账户和绩效计算的唯一执行引擎。
- 前端不生成代码，只根据服务端能力目录构建版本化 `StrategyDefinition` JSON。
- Go 收到 JSON 后先反序列化、校验并编译，再执行回测；不能逐 K 线解释未经校验的动态对象。

该方案适合长期演进。Go 的类型约束、稳定性能和并发任务能力适合回测服务；Python 的交易所和数据处理生态适合离线数据采集。主要代价是 Go 的技术指标生态弱于 Python，因此指标必须有统一接口，并使用 Python 参考实现生成 golden data 做一致性测试。

详细的成交时间、K 线内冲突、资金预留和防未来函数语义继续遵循 [动态规则回测框架设计](./backtest-framework-design.md)。

## 2. 系统边界

### 2.1 Python 数据层

负责：

- 调用交易所 API 下载 OHLCV。
- 分页、限流、重试和断点续传。
- 去重、排序和基础 OHLC 校验。
- 写入统一 Parquet 数据集。
- 生成数据清单、缺口报告和文件校验和。

不负责：

- 计算策略指标。
- 产生交易信号。
- 处理手续费、滑点或持仓。
- 修改回测结果。

指标必须只在 Go 中计算，否则 Python 和 Go 的舍入、warm-up 或公式差异会让同一策略出现两套结果。

### 2.2 Go 服务

负责：

- 暴露指标、操作符和动作能力目录。
- 保存和版本化策略定义。
- 校验并编译策略。
- 加载 K 线并执行回测。
- 管理异步回测任务。
- 保存结果、运行元数据和审计事件。
- 向前端提供资金曲线、交易明细和指标。

### 2.3 前端

负责：

- 通过表单选择数据集、标的、周期和账户参数。
- 添加指标并编辑参数。
- 通过嵌套条件组构建 `AND/OR/NOT` 规则。
- 选择开仓、平仓、仓位和保护性条件。
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

交易所、标的和周期放在分区路径及 manifest 中，不在每一行重复存储：

```text
data/market/
└── exchange=binance/
    └── symbol=BTC-USDT/
        └── timeframe=1h/
            ├── year=2025/month=01/part-000.parquet
            └── year=2025/month=02/part-000.parquet
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
  "timeframes": ["1h"],
  "start_ms": 1735689600000,
  "end_ms": 1738364400000,
  "row_count": 744,
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
      "path": "exchange=binance/symbol=BTC-USDT/timeframe=1h/year=2025/month=01/part-000.parquet",
      "sha256": "..."
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
	MarketType    MarketType `json:"market_type"`
	InitialCash   float64    `json:"initial_cash"`
	QuoteCurrency string     `json:"quote_currency"`
}

type BehaviorSpec struct {
	AllowPyramiding      bool `json:"allow_pyramiding"`
	AllowSameBarReversal bool `json:"allow_same_bar_reversal"`
	CooldownBars         int  `json:"cooldown_bars"`
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
	MaxSymbolEquityPct  float64 `json:"max_symbol_equity_pct"`
	MaxTotalExposurePct float64 `json:"max_total_exposure_pct"`
	MaxOpenPositions    int     `json:"max_open_positions"`
	MaxDrawdownHaltPct  float64 `json:"max_drawdown_halt_pct"`
}
```

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
```

引擎内部不得持续传递 `map[string]any`。

### 4.3 表达式 AST

条件使用递归表达式树，前端可直接用树形控件构建：

```go
type ExprKind string

const (
	ExprOperator ExprKind = "operator"
	ExprReference ExprKind = "reference"
	ExprNumber    ExprKind = "number"
	ExprText      ExprKind = "text"
	ExprBoolean   ExprKind = "boolean"
)

type Expression struct {
	NodeID  string       `json:"node_id"`
	Kind    ExprKind     `json:"kind"`
	Op      Operator     `json:"op,omitempty"`
	Args    []Expression `json:"args,omitempty"`
	Ref     *ValueRef    `json:"ref,omitempty"`
	Number  *float64     `json:"number,omitempty"`
	Text    *string      `json:"text,omitempty"`
	Boolean *bool        `json:"boolean,omitempty"`
}
```

结构校验必须保证每个节点只有一种有效形态：

- `operator`：必须有合法 `op` 和符合签名的 `args`。
- `reference`：必须有 `ref`。
- `number`：必须有 `number`，包括合法的 `0`。
- `text`：必须有 `text`。
- `boolean`：必须有 `boolean`。

使用指针保存常量是为了区分“未提供”和零值。

表达式示例：

```json
{
  "node_id": "entry-root",
  "kind": "operator",
  "op": "all",
  "args": [
    {
      "node_id": "ema-trend",
      "kind": "operator",
      "op": "gt",
      "args": [
        {
          "node_id": "ema-fast-ref",
          "kind": "reference",
          "ref": {
            "scope": "indicator",
            "name": "ema_fast",
            "field": "value"
          }
        },
        {
          "node_id": "ema-slow-ref",
          "kind": "reference",
          "ref": {
            "scope": "indicator",
            "name": "ema_slow",
            "field": "value"
          }
        }
      ]
    },
    {
      "node_id": "rsi-limit",
      "kind": "operator",
      "op": "lt",
      "args": [
        {
          "node_id": "rsi-ref",
          "kind": "reference",
          "ref": {
            "scope": "indicator",
            "name": "rsi_14",
            "field": "value"
          }
        },
        {
          "node_id": "rsi-threshold",
          "kind": "number",
          "number": 70
        }
      ]
    }
  ]
}
```

`NodeID` 由前端生成并在一个策略版本内唯一。服务端校验错误可直接关联到节点，避免前端解析复杂 JSON 路径。

### 4.4 规则和动作

```go
type RuleSpec struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Enabled  bool           `json:"enabled"`
	Priority int            `json:"priority"`
	Trigger  RuleTrigger    `json:"trigger"`
	Position PositionFilter `json:"position"`
	When     Expression     `json:"when"`
	Action   ActionSpec     `json:"action"`
}

type ActionSpec struct {
	Type       ActionType      `json:"type"`
	Side       Side            `json:"side,omitempty"`
	Sizing     *SizingSpec     `json:"sizing,omitempty"`
	Protection *ProtectionSpec `json:"protection,omitempty"`
	ReducePct  *float64        `json:"reduce_pct,omitempty"`
}

type SizingSpec struct {
	Type  SizingType `json:"type"`
	Value float64    `json:"value"`
}

type ProtectionSpec struct {
	StopLoss   *PriceDistanceSpec `json:"stop_loss,omitempty"`
	TakeProfit *PriceDistanceSpec `json:"take_profit,omitempty"`
}

type PriceDistanceSpec struct {
	Type  PriceDistanceType `json:"type"`
	Value float64           `json:"value"`
}
```

推荐动作模型：

| `type` | 必填字段 | 说明 |
| --- | --- | --- |
| `open_position` | `side`、`sizing` | 开多或开空 |
| `close_position` | 无 | 平掉规则作用标的的当前仓位 |
| `reduce_position` | `reduce_pct` | 按当前仓位比例减仓 |

`open_long`、`close_long` 等名称适合作为 UI 文案，不建议在底层定义为多个动作类型。统一的动作加 `side` 更容易扩展。

### 4.5 策略 JSON 示例

```json
{
  "schema_version": 1,
  "id": "ema-boll-rsi",
  "name": "EMA BOLL RSI",
  "data": {
    "dataset_id": "binance-spot-2025-01",
    "symbols": ["BTC-USDT"],
    "timeframe": "1h",
    "start": "2025-01-01T00:00:00Z",
    "end": "2025-02-01T00:00:00Z"
  },
  "account": {
    "market_type": "spot",
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
    "intrabar_conflict": "worst_case",
    "fee": {
      "maker_rate": 0.001,
      "taker_rate": 0.001
    },
    "slippage": {
      "type": "basis_points",
      "value": 2
    }
  },
  "risk": {
    "max_symbol_equity_pct": 0.3,
    "max_total_exposure_pct": 1,
    "max_open_positions": 5,
    "max_drawdown_halt_pct": 0.25
  },
  "indicators": [
    {
      "id": "ema_fast",
      "type": "ema",
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
      "id": "boll_20",
      "type": "boll",
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
    },
    {
      "id": "rsi_14",
      "type": "rsi",
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
      "id": "entry-long",
      "name": "趋势做多",
      "enabled": true,
      "priority": 100,
      "trigger": "bar_close",
      "position": "flat",
      "when": {
        "node_id": "entry-root",
        "kind": "operator",
        "op": "all",
        "args": [
          {
            "node_id": "price-above-ema",
            "kind": "operator",
            "op": "gt",
            "args": [
              {
                "node_id": "close-ref",
                "kind": "reference",
                "ref": {
                  "scope": "market",
                  "name": "bar",
                  "field": "close"
                }
              },
              {
                "node_id": "ema-ref",
                "kind": "reference",
                "ref": {
                  "scope": "indicator",
                  "name": "ema_fast",
                  "field": "value"
                }
              }
            ]
          },
          {
            "node_id": "price-cross-boll-middle",
            "kind": "operator",
            "op": "cross_above",
            "args": [
              {
                "node_id": "close-boll-ref",
                "kind": "reference",
                "ref": {
                  "scope": "market",
                  "name": "bar",
                  "field": "close"
                }
              },
              {
                "node_id": "boll-middle-ref",
                "kind": "reference",
                "ref": {
                  "scope": "indicator",
                  "name": "boll_20",
                  "field": "middle"
                }
              }
            ]
          },
          {
            "node_id": "rsi-under-70",
            "kind": "operator",
            "op": "lt",
            "args": [
              {
                "node_id": "rsi-ref",
                "kind": "reference",
                "ref": {
                  "scope": "indicator",
                  "name": "rsi_14",
                  "field": "value"
                }
              },
              {
                "node_id": "rsi-limit",
                "kind": "number",
                "number": 70
              }
            ]
          }
        ]
      },
      "action": {
        "type": "open_position",
        "side": "long",
        "sizing": {
          "type": "equity_pct",
          "value": 0.2
        },
        "protection": {
          "stop_loss": {
            "type": "entry_pct",
            "value": 0.03
          },
          "take_profit": {
            "type": "entry_pct",
            "value": 0.06
          }
        }
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

首版内置 EMA、SMA、BOLL、RSI、ATR。所有指标都用固定数据与可信 Python 实现对比。

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
      "type": "cross_above",
      "args": ["number_series", "number_series"],
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
│   ├── EMA20
│   └── RSI14
├── 规则
│   ├── 入场规则
│   │   └── AND
│   │       ├── close cross_above EMA20
│   │       └── RSI14 < 70
│   └── 出场规则
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
- 服务端错误返回 `node_id` 或字段路径，前端定位到具体控件。

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
      "node_id": "rsi-ref",
      "message": "indicator rsi_14 has no output named upper"
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
│   └── backtest/
│       └── main.go
├── internal/
│   ├── api/
│   ├── strategy/
│   │   ├── model.go
│   │   ├── validation.go
│   │   └── repository.go
│   ├── catalog/
│   ├── compiler/
│   ├── data/
│   │   ├── bar.go
│   │   ├── parquet.go
│   │   └── manifest.go
│   ├── indicator/
│   │   ├── registry.go
│   │   └── builtin/
│   ├── rule/
│   ├── engine/
│   ├── execution/
│   ├── portfolio/
│   ├── risk/
│   ├── reporting/
│   └── job/
├── tools/
│   └── market-data/
│       ├── pyproject.toml
│       └── market_data/
├── web/
├── testdata/
├── strategies/
└── docs/
```

边界要求：

- `internal/strategy` 只定义持久化和 API 使用的策略模型。
- `internal/compiler` 将策略模型转换为运行模型。
- `internal/engine` 不依赖 HTTP、数据库或前端模型。
- `internal/indicator` 不依赖账户与撮合。
- `internal/execution` 只通过订单、成交和市场规则交互。

MVP 可以让 API 和 worker 在同一进程运行，但代码边界需要从第一版分开，后续才能独立扩容回测 worker。

## 10. 持久化与可复现性

策略版本保存：

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

### 11.2 跨语言数据测试

- Python 写出的 Parquet 由 Go 读取并逐字段对比。
- manifest 文件数、行数和 SHA256 一致。
- 时间戳、空值和可选列覆盖边界场景。

### 11.3 指标一致性测试

- Python 使用可信指标实现生成固定输入和预期输出。
- Go 只读取固定 golden fixture，不在测试运行时依赖 Python。
- 明确比较 warm-up、NaN、舍入和边界值。

### 11.4 端到端测试

- 前端示例 JSON通过校验。
- 编译产出固定定义哈希和 warm-up。
- 5 至 20 根人工 K 线得到固定订单与成交序列。
- 修改未来 K 线不能改变此前信号和订单。

## 12. 实施顺序

### Phase 1：先打通无前端内核

1. 定义 Go 结构体、严格 JSON 解码和校验错误模型。
2. 实现 Python 下载器和 Parquet/manifest 契约。
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
- 由 Python 计算一部分指标、Go 计算另一部分指标。
- 修改已有策略 revision，导致历史回测不可复现。
- 当前 K 线收盘产生信号后又默认按当前收盘价成交。
- 只保存最终绩效，不保存订单、成交和规则命中审计。
