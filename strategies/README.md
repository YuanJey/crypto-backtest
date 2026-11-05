# Strategies

策略文件使用版本化 JSON 存储，文件名为 `<strategy_id>.json`。回测入口通过 `-strategy-id` 选择指定策略。

示例 [rsi7-rsi14-daily-cross.json](rsi7-rsi14-daily-cross.json) 使用已收盘日线 RSI(7) / RSI(14)：
金叉开多，死叉全平，不加仓。底层 15m 数据用于撮合，两个指标均声明 `timeframe: "1d"`。
实际 ETH 行情的验证结果见 [回测报告](../daily-rsi-cross-backtest-report.md)。

正式服务模式下，策略保存到 SQLite：

```bash
go run ./cmd/api -addr :8080 -db data/app.db -data-root data/market
```

前端通过 API 创建策略，再通过 `POST /api/v1/backtests` 选择已创建策略和本次 report 配置执行回测。

```bash
go run ./cmd/backtest -strategy-id ema-boll-rsi
go run ./cmd/backtest -db data/app.db -strategy-id ema-boll-rsi
go run ./cmd/backtest -db data/app.db -strategy-id ema-boll-rsi -strategy-revision 2
```

SQLite 保存同 ID 策略时会追加不可变 revision。回测请求省略 revision 时使用最新
版本，也可以通过 `strategy_revision` 或 CLI 的 `-strategy-revision` 精确复跑历史版本。

执行回测时加 `-run`，从策略的 `dataset_id` 对应目录读取 Parquet：

```bash
go run ./cmd/backtest -strategy-id ema-boll-rsi -run -data-root data/market
go run ./cmd/backtest -db data/app.db -strategy-id ema-boll-rsi -run -data-root data/market
```

选择报告输出：

```bash
go run ./cmd/backtest -strategy-id ema-boll-rsi -run \
  -report summary,equity,orders,fills,indicators \
  -report-indicator ema_fast_15m.value,rsi_14_4h.value
```

每个策略可以配置自己的指标列表和初始化参数：

```json
{
  "id": "ema_fast",
  "type": "ema",
  "timeframe": "15m",
  "inputs": [
    {"scope": "market", "name": "bar", "field": "close"}
  ],
  "params": {
    "period": 20
  }
}
```

高周期指标直接声明 `timeframe`，由 15m 基准 K 线聚合后在周期完成时对齐使用：

```json
{
  "id": "ema_slow_1h",
  "type": "ema",
  "timeframe": "1h",
  "inputs": [
    {"scope": "market", "name": "bar", "field": "close"}
  ],
  "params": {
    "period": 60
  }
}
```

开多 / 开空通过规则的 `action` 表达：

```json
{
  "id": "open_short",
  "condition": "and(position.flat(), ema_fast_15m.lt(ema_slow_1h), rsi_14_4h.gt(30))",
  "action": {
    "type": "enter",
    "side": "short",
    "size_pct": 0.2
  }
}
```

首单 `enter` 仅在空仓时执行，开多、开空分别声明自己的 `size_pct`。
首单名义金额为「当前权益 × 规则比例 × `position.position_leverage`」。
加仓使用独立的 `add` 规则，每条规则声明适用的第几次加仓及自身触发条件：

```json
{
  "id": "add_long_1",
  "condition": "position.unrealized_pnl_pct.lte(-0.02)",
  "action": {
    "type": "add",
    "side": "long",
    "addition_index": 1,
    "size_pct": 0.1
  }
}
```

交易规则区的“允许加仓”开关写入 `behavior.allow_additions`。开启后可以选择
加仓计算模式；关闭时保留规则参数和条件，运行时只停止加仓，首单和平仓仍执行。
`add` 仅在开关允许、已有同向持仓且 `entry_count == addition_index` 时执行。
首次成交后 `entry_count` 为 1，因此 `addition_index: 1` 表示第一次加仓；
加仓成交后进入下一次，全部平仓后计数和首单名义金额重置。部分平仓不重置。
多个规则可以竞争同一次加仓，仍按优先级和规则 ID 顺序选择第一条匹配规则，
每根基础 K 线最多产生一个动作，不会自动跳过缺失的加仓次数。

固定比例加仓名义金额为「当前权益 × `action.size_pct`」，不再乘首单杠杆。
倍数模式使用 `size_multiplier`，并通过 `sizing_mode` 选择基准：

| `sizing_mode` | 页面名称 | 本次加仓名义金额 |
| --- | --- | --- |
| `current_position_multiple` | 当前持仓倍增 | 加仓前同向总持仓名义金额 × 倍数 |
| `first_entry_geometric` | 首仓层级倍增 | 本轮首单实际成交名义金额 × 倍数ⁿ |
| `first_entry_multiple` | 首仓固定倍数 | 本轮首单实际成交名义金额 × 倍数 |
| `equity_pct` | 当前权益比例 | 当前权益 × `size_pct` |

`n` 为 `addition_index`，第一次加仓从 1 开始。当前持仓基准按下一根 K 线开盘的
实际成交价计算，等价于在无额度裁剪时新增「加仓前持仓数量 × 倍数」。
首仓基准保留本轮首次成交的实际名义金额，部分平仓不修改此基准。
例如首仓 100、倍数 2，前三次加仓：当前持仓模式为 200、600、1800，
首仓层级模式为 200、400、800（价格不变且额度充足）。

```json
{
  "behavior": {"allow_additions": true},
  "rules": [
    {
      "id": "add_long_2",
      "condition": "position.unrealized_pnl_pct.lte(-0.04)",
      "action": {
        "type": "add",
        "side": "long",
        "addition_index": 2,
        "sizing_mode": "first_entry_geometric",
        "size_multiplier": 2
      }
    }
  ]
}
```

该规则的加仓金额是首仓实际名义金额的 4 倍。切换为
`current_position_multiple` 时则为加仓前总持仓名义金额的 2 倍。
比例和倍数必须二选一；新首单、固定比例加仓均要求比例在 `(0, 1]`，
允许任意小的正数。

全局杠杆配置为 `account.exchange_leverage` 和 `position.position_leverage`；
现货固定为 1x。首单和加仓均受交易所杠杆的总名义金额上限约束，
实际杠杆由报告动态计算。比例、模式和次数仍配置在各条交易规则中。

首仓基础名义金额为 `min(equity, equity * size_pct * position_leverage)`。
`position.long_capital_decay_power` 和 `position.short_capital_decay_power`
分别控制多空首仓的盈利衰减，实际首仓再乘以
`min(1, (initial_cash / current_equity) ^ power)`；默认值均为 `0.5`，
权益低于初始资金时不会放大仓位。首仓倍数模式继承实际首单金额，
固定比例加仓不重复应用杠杆或衰减系数。

平仓 `close` 使用当前持仓数量为基准，`size_pct: 1` 表示全部平仓，
例如 `size_pct: 0.25` 表示平掉四分之一。旧 `close` 的缺省或 0 仍表示全部平仓。

后端保留 `open` 及旧策略级仓位字段的原执行语义，以便复跑历史 revision。
旧定义没有 `allow_additions` 时保持原执行行为：显式 `add` 允许加仓，
旧 `open` 仍读取原 `allow_pyramiding`。旧 `add` 未指定 `sizing_mode` 时，
有 `size_multiplier` 表示首仓固定倍数，否则表示权益比例，不会重复计算次方。
新开关显式配置后统一覆盖上述加仓许可，默认新策略关闭加仓。
进入可视化编辑器或保存旧 JSON 时，旧 `open` 会拆成首单及逐次加仓规则：
显式规则比例覆盖全局比例，旧倍增系数第 n 次转成该系数的 n 次方；
原条件、禁用状态和优先级保留，`source_rule_id` 保存不可变来源及同优先级排序。
它是历史来源标识，不是对现有规则的依赖，来源规则重命名或删除后仍可保留。
转换只影响正在编辑的定义，保存时追加新 revision，不批量改写历史版本。

打开可视化编辑器不会重写 `when`。嵌套 AND/OR、NOT、持仓引用及暂不支持的条件
以原样预览保留，可切换 JSON 源码编辑；修改简单条件时保留其他节点和原历史偏移。
链式 `condition` 原样保存，不会自动补入一个与它冲突的 `when`。

止盈止损建议使用顶层 `exit_policies`。它支持持仓均价、首次价、最后加仓价和
入场后高低点锚点，支持百分比、固定价差与 ATR 倍数距离。ATR 可以在首次开仓
冻结、每次加仓刷新或随最新闭合 K 线更新（下一根 K 线生效）；`close_pct` 可用于分批退出，
`execution` 可选下一根开盘或触发价/跳空价成交。

内置 `extreme` 指标同时输出滚动窗口的 `highest` 与 `lowest`。例如配置
`timeframe: "1d"`、`params.period: 5` 可得到 5 日最高/最低价。退出策略可将
`anchor.type` 设为 `indicator` 并引用对应输出，配合 `distance.type: "direct"`
直接在该极值价格止盈或止损。指标锚点使用上一根已闭合指标值，下一根 K 线生效。

当前内置指标参数：

| type | params | outputs |
| --- | --- | --- |
| `ema` | `period >= 1` | `value` |
| `sma` | `period >= 1` | `value` |
| `rsi` | `period >= 1` | `value` |
| `atr` | `period >= 1` | `value` |
| `boll` | `period >= 1`, `stddev > 0` | `middle`, `upper`, `lower` |
| `supertrend` | `period >= 1`, `multiplier > 0` | `value`, `direction` |
