# 动态规则回测框架设计

> 技术方案已调整为“Python 下载数据 + Go 回测服务 + 前端策略构建器”。当前实现架构和 Go 结构体以 [Go 回测系统与前端策略构建器设计](./go-backtest-system-design.md) 为准；本文继续作为成交、账户、风控和防未来函数的行为语义说明，原 Python 技术栈与工程目录不再作为实现依据。

## 1. 目标

构建一个面向 K 线策略的回测框架。使用者通过配置组合 EMA、BOLL、RSI 等预置指标和条件，无需编写交易循环，即可定义开仓、平仓、仓位和风控规则。

首版目标：

- 支持 OHLCV K 线、多个交易标的和统一账户。
- 内置常见技术指标，并允许注册自定义指标。
- 通过声明式规则定义开多、开空、平仓和减仓。
- 支持手续费、滑点、最小下单量、价格精度和资金约束。
- 严格区分信号时间和成交时间，默认不存在未来函数。
- 产出可复现的订单、成交、交易、资金曲线和绩效指标。
- 首版以现货为主，领域模型预留永续合约的杠杆、资金费和强平能力。

首版不包含：

- Tick/订单簿级撮合。
- 实盘交易接入。
- 分布式参数搜索。
- 任意脚本表达式执行。

## 2. 核心决策

### 2.1 技术栈

- Go：策略模型、指标、规则编译、撮合、账户、API 和异步任务
- Python：交易所 K 线下载、清洗、分区和数据清单生成
- JSON：前端和 Go 服务之间的策略定义契约
- Parquet：Python 与 Go 共享的历史行情和结果明细格式
- SQLite：首版策略版本与任务元数据
- Go testing + pytest：Go 引擎测试和 Python 数据工具测试

完整技术架构、Go 结构体和前端 API 见 [Go 回测系统与前端策略构建器设计](./go-backtest-system-design.md)。指标库通过注册表接入，不能把第三方指标库的数据结构扩散到规则引擎。

### 2.2 运行模型

采用 Go 事件驱动模型：

1. Go 数据层按时间顺序加载统一 Parquet K 线。
2. 指标 DAG 逐根 K 线增量更新指标状态和输出槽位。
3. 规则引擎在每根已完成 K 线上读取当时可见的值。
4. 交易引擎依次处理订单、成交、持仓和账户。

增量指标只读取当前及历史输入，天然适合防止未来函数，也可以与以后模拟盘使用同一套运行逻辑。不同回测任务可以并发执行，同一共享账户内的标的按统一时间线确定性处理。

### 2.3 默认时间语义

默认采用 `close_signal_next_open_fill`：

- K 线 `t` 收盘后，指标值才可用于生成信号。
- 信号生成的市价单最早在 K 线 `t+1` 开盘成交。
- 保护性止损/止盈在开仓成交后生效，可使用后续 OHLC 判断触发。
- 高周期指标只能使用已经完整收盘的高周期 K 线。

如果用户显式选择 `current_close_fill`，报告必须标记该模式，并使用单独的收盘成交模型。不能把当前收盘价成交作为默认行为。

## 3. 总体架构

```text
                       StrategyDefinition JSON
                                |
                     Go Validator / Compiler
                                |
                    +-----------+-----------+
                    |                       |
             Indicator Compiler        Rule Compiler
                    |                       |
             Indicator DAG              Rule AST
                    |                       |
Python -> Parquet -> Data Portal -> Timeline / Backtest Engine
                                      |
          +---------------------------+---------------------------+
          |                           |                           |
    Broker Simulator             Portfolio                   Risk Manager
          |                           |                           |
        Orders -> Fills -> Position / Cash / Equity <- Risk Decisions
                                      |
                              Result Collector
                                      |
                  Orders / Trades / Equity / Metrics / Audit Log
```

组件职责：

| 组件 | 职责 |
| --- | --- |
| `DataPortal` | 加载、校验、对齐行情，只向引擎暴露当前时点可见数据 |
| `IndicatorEngine` | 构建指标依赖 DAG、计算指标、处理 warm-up 和缓存 |
| `RuleCompiler` | 把配置转换为类型安全的 AST，检查引用、参数和类型 |
| `RuleEngine` | 在指定时点计算规则，不直接修改账户 |
| `StrategyRuntime` | 管理规则组、冷却期、加仓/反手策略和动作去重 |
| `RiskManager` | 对动作做账户级与标的级约束，生成或拒绝订单 |
| `BrokerSimulator` | 执行订单，应用滑点、手续费、精度和成交规则 |
| `Portfolio` | 维护现金、持仓、已实现/未实现盈亏和净值 |
| `ResultCollector` | 保存审计事件并计算回测结果 |

规则引擎只产生 `ActionIntent`，不能直接改变持仓。持仓变化只能由 `Fill` 驱动，避免信号、订单和实际成交混为一体。

## 4. 工程目录

```text
crypto-backtest/
├── cmd/
│   ├── api/
│   ├── worker/
│   └── backtest/
├── internal/
│   ├── strategy/
│   ├── catalog/
│   ├── compiler/
│   ├── data/
│   ├── indicator/
│   ├── rule/
│   ├── engine/
│   ├── execution/
│   ├── portfolio/
│   ├── risk/
│   ├── reporting/
│   └── job/
├── tools/
│   └── market-data/
│       └── market_data/
├── web/
├── strategies/
├── testdata/
└── docs/
```

详细包职责见 Go 系统设计。Go 引擎不依赖 HTTP、数据库或 Python 工具，保证 CLI、API 和 worker 可以复用同一内核。

## 5. 行情数据契约

标准 K 线字段：

| 字段 | 类型 | 约束 |
| --- | --- | --- |
| `timestamp` | UTC datetime | 表示 K 线开始时间；同一标的严格递增且唯一 |
| `symbol` | string | 使用框架内部标准名称，如 `BTC-USDT` |
| `timeframe` | string | 如 `1m`、`1h`、`1d` |
| `open/high/low/close` | float64 | 大于 0，且满足 OHLC 关系 |
| `volume` | float64 | 大于等于 0 |

`timestamp` 统一表示 K 线开盘时间，`bar_close_time = timestamp + timeframe`。规则的 `evaluation_time` 使用 `bar_close_time`，成交事件使用实际模拟成交时点，避免同一个 `timestamp` 同时被解释成开盘和收盘。

加载阶段必须校验：

- 重复、乱序、缺失 K 线和非法 OHLC。
- 时间戳时区与周期边界。
- `high >= max(open, close)`、`low <= min(open, close)`。
- 数据范围是否覆盖指标 warm-up。

缺失 K 线默认记录告警但不自动填充。自动填充会虚构可成交行情；若策略确实需要连续时间轴，应由数据配置显式选择填充规则。

多周期数据使用“已完成周期向后对齐”。例如，在 `10:00-10:59` 的 1 小时 K 线于 `11:00` 完成前，低周期策略不能读取它的最终值。

## 6. 指标系统

### 6.1 指标定义

```go
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

每个指标可返回一个或多个具名输出：

- EMA：`value`
- BOLL：`middle`、`upper`、`lower`
- MACD：`macd`、`signal`、`histogram`
- STOCH：`k`、`d`

首批内置指标：

- 趋势：SMA、EMA、WMA、MACD、ADX
- 动量：RSI、STOCH、ROC、CCI
- 波动率：BOLL、ATR、标准差
- 成交量：OBV、成交量均线
- 基础滚动值：最高、最低、均值、求和、收益率

### 6.2 指标配置

以下 YAML 仅用于紧凑展示指标语义；前端和 Go 服务实际使用结构化 `StrategyDefinition` JSON。

```yaml
indicators:
  - id: ema_fast
    type: ema
    input: market.close
    params:
      period: 20

  - id: ema_slow
    type: ema
    input: market.close
    params:
      period: 60

  - id: boll_20
    type: boll
    input: market.close
    params:
      period: 20
      stddev: 2.0

  - id: rsi_14
    type: rsi
    input: market.close
    params:
      period: 14
```

规则中的引用分别为：

- `indicator.ema_fast.value`
- `indicator.boll_20.upper`
- `indicator.boll_20.lower`
- `indicator.rsi_14.value`

指标引擎根据输入引用构建 DAG，禁止循环依赖。相同类型、输入和参数的指标只计算一次。

### 6.3 Warm-up

任一引用值为 `NaN` 时，比较和交叉条件统一返回 `false`，不得把缺失值转换为 0。正式回测区间之前可额外加载 warm-up 数据，但 warm-up 区间不能成交，也不能进入绩效统计。

## 7. 规则 DSL

### 7.1 原则

- 配置最终编译为受控 AST，不执行任意字符串代码。
- 所有引用在启动时完成解析和类型检查。
- 操作符是白名单，并可通过注册表扩展。
- 条件只返回布尔值，动作只描述意图。
- 配置包含 `schema_version`，为后续兼容迁移保留空间。

### 7.2 值引用

可用命名空间：

| 命名空间 | 示例 |
| --- | --- |
| 行情 | `market.open`、`market.close`、`market.volume` |
| 指标 | `indicator.ema_fast.value`、`indicator.boll_20.upper` |
| 持仓 | `position.side`、`position.quantity`、`position.unrealized_pnl_pct` |
| 账户 | `account.equity`、`account.available_cash`、`account.drawdown_pct` |
| 常量 | `30`、`0.02`、`true` |

首版规则不能直接引用未来偏移。历史引用仅通过受控操作符实现，且偏移必须小于等于 0。

### 7.3 操作符

布尔操作：

- `all`、`any`、`not`

比较操作：

- `gt`、`gte`、`lt`、`lte`、`eq`

时序操作：

- `cross_above`、`cross_below`
- `rising`、`falling`
- `consecutive`
- `within_last`

算术操作：

- `add`、`sub`、`mul`、`div`、`abs`

`cross_above(left, right)` 的固定语义：

```text
left[t-1] <= right[t-1] AND left[t] > right[t]
```

必须同时具备 `t-1` 和 `t` 的有效值，否则返回 `false`。

### 7.4 规则配置示例

以下策略表达：

- EMA20 位于 EMA60 上方，收盘价向上突破 BOLL 中轨，且 RSI 小于 70 时开多。
- 收盘价跌破 EMA20、RSI 高于 80 或收盘价触及 BOLL 上轨时平多。
- 每次使用 20% 权益，附带 3% 止损和 6% 止盈。

以下 YAML 同样是便于阅读的语义示例，不是 API 请求格式。

```yaml
schema_version: 1

strategy:
  id: ema_boll_rsi
  symbols: [BTC-USDT]
  timeframe: 1h
  allow_pyramiding: false
  allow_same_bar_reversal: false
  cooldown_bars: 1

rules:
  - id: open_long
    priority: 100
    when:
      all:
        - op: gt
          left: indicator.ema_fast.value
          right: indicator.ema_slow.value
        - op: cross_above
          left: market.close
          right: indicator.boll_20.middle
        - op: lt
          left: indicator.rsi_14.value
          right: 70
    action:
      type: open_long
      sizing:
        type: equity_pct
        value: 0.20
      protection:
        stop_loss:
          type: entry_pct
          value: 0.03
        take_profit:
          type: entry_pct
          value: 0.06

  - id: close_long
    priority: 200
    when:
      any:
        - op: cross_below
          left: market.close
          right: indicator.ema_fast.value
        - op: gt
          left: indicator.rsi_14.value
          right: 80
        - op: gte
          left: market.close
          right: indicator.boll_20.upper
    action:
      type: close_long
```

### 7.5 规则编译结果

编译阶段生成：

```text
Rule
├── id
├── priority
├── predicate: BooleanExpr
├── action: ActionSpec
├── dependencies: Set[ValueRef]
└── required_history: int
```

启动时即拒绝：

- 未注册指标或操作符。
- 指标输出字段拼写错误。
- RSI 周期为负数等非法参数。
- 布尔值和数值错误比较。
- 动作和市场类型不兼容，例如现货 `open_short`。
- 正向时间偏移。
- 重复规则 ID 或指标 ID。

## 8. 动作、仓位和风控

### 8.1 动作类型

首版动作：

- `open_long`
- `open_short`，仅在市场配置允许时
- `close_long`
- `close_short`
- `reduce_position`
- `close_position`

动作只生成意图：

```go
type ActionIntent struct {
	Timestamp  time.Time
	Symbol     string
	RuleID     string
	ActionType ActionType
	Sizing     *SizingSpec
	Protection *ProtectionSpec
	Priority   int
}
```

`StrategyRuntime` 会在动作产生前应用持仓状态前置条件：禁止加仓时，`open_long` 只对空仓生效；`close_long` 只对多仓生效。状态不匹配需要记录为 `action_skipped`，不要求用户在每条规则里重复编写持仓判断。

### 8.2 仓位计算

首版支持：

- `fixed_quantity`
- `fixed_notional`
- `equity_pct`
- `available_cash_pct`
- `risk_pct`

`risk_pct` 按止损距离计算：

```text
risk_cash = account_equity * risk_pct
quantity = risk_cash / abs(entry_price - stop_price)
```

之后仍需应用可用资金、最大仓位、最小下单量和数量步长约束。

### 8.3 风控顺序

`RiskManager` 按以下顺序处理：

1. 市场能力检查，例如现货不能裸开空。
2. 当前持仓状态检查，例如禁止重复开仓或加仓。
3. 冷却期和交易频率限制。
4. 单笔风险和单标的最大仓位。
5. 账户总敞口、最大持仓数和最大回撤熔断。
6. 最小名义价值、数量步长和价格精度。
7. 可用现金或保证金检查。

拒绝订单必须产生带 `reason_code` 的审计事件，不能静默丢弃。

## 9. 订单与撮合

### 9.1 领域对象

```text
Signal/ActionIntent -> Order -> Fill -> Position + LedgerEntry
```

- `Order`：策略希望执行的交易请求。
- `Fill`：模拟交易所实际执行的结果。
- `Position`：由成交推导出的当前持仓。
- `LedgerEntry`：现金、手续费、已实现盈亏等账务变化。

首版订单类型只要求 `market`。`limit`、`stop_market` 和 `stop_limit` 可以在保护性订单实现时加入，但不能用简单的收盘价比较冒充真实触发。

### 9.2 每根 K 线处理顺序

对时间戳 `t`，同一时刻所有标的按以下阶段处理：

1. **OPEN**：用 `open[t]` 处理上一根 K 线收盘后排队的市价单。
2. **INTRABAR**：用 `open/high/low` 检查已生效的止损、止盈和强平。
3. **MARK**：用 `close[t]` 统一更新全部持仓市值和账户权益。
4. **INDICATOR**：暴露截至 `close[t]` 已完成的数据和指标。
5. **RULE**：计算退出规则，再计算入场规则。
6. **RISK**：校验动作并生成待执行订单。
7. **SNAPSHOT**：记录账户、规则命中和订单审计快照。

同一时间戳必须先完成所有标的的 `MARK`，再评估账户级规则，避免处理标的顺序影响可用权益。

同一时间戳产生的动作按批次进入风控。买入订单获批时立即预留预计成交额和费用，后续订单只能使用扣除预留后的资金；待平仓订单在实际成交前不会提前释放资金。首版按动作优先级、规则 ID、标的名称稳定排序分配资金，后续可增加按比例分配模式。

### 9.3 成交价格

默认市价买单：

```text
fill_price = open_price * (1 + slippage_rate)
```

默认市价卖单：

```text
fill_price = open_price * (1 - slippage_rate)
```

滑点模型是接口，首版实现：

- 固定百分比
- 固定基点
- 基于成交额/当根成交量的冲击模型

手续费模型至少区分 maker/taker；首版市价单统一按 taker 费率。

### 9.4 跳空与同 K 线歧义

- 如果开盘已经越过止损价，按开盘价叠加滑点成交，不能按更优的止损价成交。
- 如果同一根 K 线的高低价同时触发止盈和止损，OHLC 无法确定先后。
- 默认使用 `worst_case`：选择对当前持仓最不利的成交。
- 可配置 `best_case` 或更低周期数据重放，但报告必须记录歧义次数和采用的模型。

退出优先级固定为：

```text
强平 > 保护性止损 > 保护性止盈 > 规则平仓 > 减仓 > 新开仓
```

同优先级按规则 `priority` 降序、规则 ID 和标的名称稳定排序，保证结果可复现。默认禁止同一根信号 K 线同时平仓和反手。

## 10. 账户与持仓

首版使用平均成本法维护仓位，但订单、成交和账本必须完整保留，以便以后增加 FIFO 报表。

核心不变量：

- 只有 `Fill` 可以改变持仓数量。
- 手续费始终进入账本并减少权益。
- `equity = cash + position_market_value`，合约账户使用独立公式。
- 已实现盈亏与未实现盈亏不能重复计入。
- 订单被拒绝或取消不能影响账户余额。
- 数量和价格统一经过交易规则量化。

价格和数量约束由 Python 下载数据时写入数据集 manifest，手续费由策略执行配置指定。两者都必须随回测结果保存，不能在 Go 代码中硬编码：

```yaml
market_rules:
  BTC-USDT:
    price_tick: 0.01
    quantity_step: 0.00001
    min_quantity: 0.00001
    min_notional: 5
```

## 11. 回测配置全貌

实际配置使用版本化 `StrategyDefinition` JSON。完整 Go 结构体、JSON 示例和字段约束见 [策略结构体](./go-backtest-system-design.md#4-策略结构体)。数据通过 `dataset_id` 引用，不允许前端提交任意服务器文件路径。

Go 代码入口：

```go
definition, err := strategy.Decode(rawJSON)
plan, err := compiler.Compile(definition)
result, err := engine.Run(ctx, plan)
```

HTTP 和 CLI 入口：

```text
POST /api/v1/strategies/validate
POST /api/v1/backtests
crypto-backtest validate strategies/ema_boll_rsi.json
crypto-backtest run strategies/ema_boll_rsi.json
```

## 12. 结果与可复现性

每次运行保存：

```text
runs/<run_id>/
├── resolved_config.json
├── run_metadata.json
├── metrics.json
├── equity.parquet
├── orders.parquet
├── fills.parquet
├── trades.parquet
└── rule_events.parquet
```

`run_metadata.json` 至少包含：

- 框架版本和 Git commit
- 数据文件指纹
- 配置哈希
- 运行起止时间
- 时区
- 随机种子
- 成交和 K 线歧义模型
- 告警和歧义事件数量

绩效指标：

- 总收益、年化收益、基准收益
- 最大回撤、回撤持续时间
- Sharpe、Sortino、Calmar
- 胜率、盈亏比、Profit Factor、单笔期望
- 总手续费、总滑点成本、换手率
- 多空交易数、平均持仓时间、市场暴露率

Sharpe 等年化指标必须根据 timeframe 或实际时间间隔计算，不能固定假定日频。

## 13. 测试策略

### 13.1 单元测试

- 每个指标与可信实现的 golden data 对比。
- 每个操作符做真值表和边界测试。
- `cross_above/cross_below` 覆盖相等、NaN 和首个有效值。
- 数量/价格量化、手续费和滑点计算。
- 平均成本、部分平仓、反向成交和账本守恒。

### 13.2 场景测试

使用 5 至 20 根人工 K 线，精确断言每一步：

- 信号出现在 `t`，成交只能出现在 `t+1`。
- 开盘跳空穿越止损。
- 同一 K 线同时命中止盈止损。
- 退出和入场规则同时命中。
- 两个标的竞争同一账户现金。
- warm-up 阶段不产生订单。

### 13.3 防未来函数测试

- 修改回测时点之后的数据，不得改变该时点之前的指标、规则或订单。
- 高周期未收盘值不得进入低周期规则。
- 所有值引用记录 `as_of` 时间，调试模式下断言 `as_of <= evaluation_time`。

### 13.4 回归测试

固定小型数据集、配置和随机种子，对以下结果做快照：

- 订单和成交序列
- 最终现金与持仓
- 资金曲线哈希
- 核心绩效指标

## 14. 实施路线

### Phase 1：可用的单标的 MVP

- 数据契约和 CSV/Parquet 加载
- EMA、BOLL、RSI、ATR
- 规则 AST、编译器和基础操作符
- 单标的现货、市价单、手续费、固定滑点
- 开多/平多、固定金额和权益比例仓位
- 交易明细、资金曲线和基础指标
- 防未来函数与核心场景测试

验收标准：示例 EMA+BOLL+RSI 策略可以仅靠 JSON 执行，订单逐笔可解释且结果确定。

### Phase 2：完整研究能力

- 多标的共享账户
- 开空、减仓、加仓策略
- 风险比例仓位、止盈止损和追踪止损
- 多周期指标
- 参数网格搜索和训练/验证区间
- HTML 报告

### Phase 3：合约与高性能

- 永续合约、杠杆、资金费、维持保证金和强平
- 限价单和部分成交
- 成交量冲击模型
- 列式读取、内存复用和指标计算性能优化
- Walk-forward、Monte Carlo 和并行任务

## 15. 需要保持稳定的边界

实现时优先保证以下边界，避免规则配置与底层执行耦合：

1. 指标通过注册表扩展，规则只引用指标输出。
2. 操作符通过 AST 节点扩展，不允许配置直接执行代码。
3. 规则产生动作，风控产生订单，撮合产生成交。
4. 账户只消费成交和账本事件。
5. 报告只读取运行产物，不反向影响回测。
6. 配置、数据和引擎版本共同决定一次可复现运行。

这套边界允许以后替换指标库、数据源、撮合模型或报告层，而不需要改写已有策略配置。
