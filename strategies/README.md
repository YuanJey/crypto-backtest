# Strategies

策略文件使用版本化 JSON 存储，文件名为 `<strategy_id>.json`。回测入口通过 `-strategy-id` 选择指定策略。

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
    "type": "open",
    "side": "short",
    "size_pct": 0.2
  }
}
```

通用的分层加仓可以通过 position 状态控制：

```json
{
  "id": "add_long",
  "condition": "or(position.flat(), and(position.long(), position.entry_count.lt(3), position.avg_entry_price.gte(market.close)))",
  "action": {
    "type": "open",
    "side": "long"
  }
}
```

对应策略级仓位配置：

```json
{
  "behavior": {
    "allow_pyramiding": true
  },
  "position": {
    "initial_size_pct": 0.2,
    "max_additions": 2,
    "add_size_multiplier": 2,
    "max_position_pct": 1
  }
}
```

`behavior.allow_pyramiding` 是是否允许同向加仓的唯一权威开关。旧策略中的
`position.allow_pyramiding` 仍可读取，但会在编译前迁移到 `behavior`。

当前内置指标参数：

| type | params | outputs |
| --- | --- | --- |
| `ema` | `period >= 1` | `value` |
| `sma` | `period >= 1` | `value` |
| `rsi` | `period >= 1` | `value` |
| `atr` | `period >= 1` | `value` |
| `boll` | `period >= 1`, `stddev > 0` | `middle`, `upper`, `lower` |
