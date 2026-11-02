# Strategies

策略文件使用版本化 JSON 存储，文件名为 `<strategy_id>.json`。回测入口通过 `-strategy-id` 选择指定策略。

```bash
go run ./cmd/backtest -strategy-id ema-boll-rsi
```

执行回测时加 `-run`，从策略的 `dataset_id` 对应目录读取 Parquet：

```bash
go run ./cmd/backtest -strategy-id ema-boll-rsi -run -data-root data/market
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
  "when": {"kind": "boolean", "bool": true},
  "action": {
    "type": "open",
    "side": "short",
    "size_pct": 0.2
  }
}
```

当前内置指标参数：

| type | params | outputs |
| --- | --- | --- |
| `ema` | `period >= 1` | `value` |
| `sma` | `period >= 1` | `value` |
| `rsi` | `period >= 1` | `value` |
| `atr` | `period >= 1` | `value` |
| `boll` | `period >= 1`, `stddev > 0` | `middle`, `upper`, `lower` |
