# 核心回测链路解耦完成记录

日期：2026-11-05。代码基线：`96911ac`。

本轮完成现有 V1、单标的 NET 回测链路的模块解耦与完整运行闭环：策略编辑 → 固定版本 → 独立实验配置 → 数据预检与快照 → 编译 / 独立运行实例 → 风险检查 → 撮合 / 账本 → 结果归档 → 查看 / 快照重跑。该结论覆盖现有执行能力；完整架构方案中的新增策略能力仍按阶段扩展。

## 已实现边界

| 边界 | 实现与行为 |
|---|---|
| 草稿 / 保存版本 | `strategy-store.js` 独立持有来源、草稿、revision 与列表；异步切换忽略旧响应，保存提交独立副本，保存期间的新编辑保留 |
| 前端编辑与能力 | `strategy-editors.js` 持有可视化组件与事件；`strategy-contract-adapter.js` 负责兼容；`capability-catalog.js` 独立持有能力元数据；`strategy.js` 控制器由原 2,554 行缩至 124 行 |
| 实验与任务界面 | `backtest-run-controller.js` / `backtest-run-panel.js` 绑定版本并管理提交、进度、取消、历史读取与快照重跑；资金、日期、费用、滑点、杠杆和风险限额可独立设置 |
| 配置合约 | `LogicDefinition` / `RunConfig` 类型化分离；纯 `AdaptV1` 拥有副本后适配；显式运行配置只验证，不覆盖用户风险参数；禁止新旧覆盖字段混用 |
| 编译 / Runtime | 不可变 `Plan` 保存编译元数据与工厂；`Snapshot()` 返回独立副本；每次执行创建独立指标、加仓状态、退出状态和历史窗口 |
| 数据提供 | `DataProvider` 可注入；文件实现校验实际读取字节的 SHA-256、文件行数、连续区间、闭合状态、市场类型和目录边界 |
| 风险层 | 交易所上限与用户风险预算分开；风险层决定新仓准入额度和回撤停机，不修改策略公式 |
| 撮合 / 账本 | 下一开盘和触发价退出统一经过 Broker 报价 → Fill → Ledger；现金、数量、成交层数和成本基准由有效成交更新 |
| 报告 | 指标、订单、成交、资金曲线和账本按 ReportSpec 输出；Ledger 可独立重放核对余额，默认不输出以兼容旧响应 |
| 运行管理 | API 异步任务独立于 HTTP 连接；2 个执行槽、最多 16 个已接纳任务；同步兼容接口同样归档；取消后不能写入完整结果 |
| 持久化 | DDL 先行；SQLite 保存运行、输入 / 结果产物与有序生命周期事件；终态和结果事务提交，读取校验产物哈希，重启后未完成任务标为 interrupted |
| 归档查看 / 重跑 | 保存固定 revision、配置、计划哈希、引擎模型版本、实际 K 线及数据哈希；结果展示加载归档 K 线，重跑不依赖当前策略或数据文件 |
| CLI | 预检后固定已解析 revision；支持严格 JSON `-run-config` 与 `-report ledger`；CLI 直接执行并输出结果，持久化任务管理由 API 提供 |

所有配置继续严格拒绝未知字段与非法引用；信号仍在收盘产生，默认市价成交不早于下一根开盘，高周期值完成后才可见。持仓负偏移保留第一阶段修复后的历史收盘快照语义。

## 接口与使用

| 接口 | 行为 |
|---|---|
| `GET /api/v1/strategies/{id}/snapshot?revision=N` | 获取固定版本、逻辑与运行默认配置，省略 N 解析当前最新 revision |
| `POST /api/v1/backtest-runs` | 预检输入并接纳异步任务，返回 202 与 run ID |
| `GET /api/v1/backtest-runs?strategy_id=...` | 查询最近 50 次运行 |
| `GET /api/v1/backtest-runs/{id}` | 查询状态、进度与版本来源 |
| `POST /api/v1/backtest-runs/{id}/cancel` | 取消活动任务；已完成任务保持终态 |
| `GET /api/v1/backtest-runs/{id}/result` | 读取完整归档结果，未完成返回 409 |
| `GET /api/v1/backtest-runs/{id}/snapshot` | 读取哈希校验后的本次输入及实际 K 线 |
| `GET /api/v1/backtest-runs/{id}/events` | 读取生命周期事件 |
| `POST /api/v1/backtest-runs/{id}/replay` | 使用归档输入重新执行，创建新 run ID |
| `POST /api/v1/backtests` | 保留旧同步入口与旧数据覆盖字段，同时写入运行归档 |

前端选择已保存策略后显示固定 revision 与定义哈希。展开“本次回测配置与运行历史”调整本次实验，执行后可读取历史或连接活动任务。选择其他策略 / 数据源会隔离旧请求的展示；服务端任务继续运行，可从历史管理。UTC 日期使用半开区间 `[start,end)`。

CLI 示例：

```bash
go run ./cmd/backtest -db .tmp-architecture-validation/preview.db \
  -strategy-id verify_addition_long_atr_each_entry -strategy-revision 1 \
  -run -run-config .tmp-architecture-validation/run-config.json \
  -report summary,equity,orders,fills,ledger
```

## 验证结果

| 验证 | 结果 |
|---|---|
| Go 全量竞态 `go test -p 1 -race ./...` | 通过 |
| Go 全量静态 `go vet -p 1 ./...` | 通过 |
| 前端 `node --test web/tests/*.test.js` | 76 条通过，保留原 63 条；覆盖选择竞态、提交快照、取消 / 完成竞态、过期结果及实验配置隔离 |
| JavaScript 语法与 `git diff --check` | 通过 |
| 同一编译计划并发执行 | 24 个 Runtime 隔离验证通过，修改 Snapshot 不影响其他运行 |
| API 全链路 | 固定历史 revision、独立资金 / 风险、快照读取、事件读取、归档重跑、晚到取消及未知字段拒绝均通过 |
| 账本正确性 | 现货 / 永续、多空、部分 / 全平、重复 / 篡改事件、余额与盈亏重放通过 |
| 任务恢复 | HTTP 断开不取消异步任务；活动任务取消不产出完整结果；重启中断标记与完成结果保留通过 |
| 数据完整性 | 错误哈希、行数、缺失 / 重复 / 未闭合 K 线、市场不匹配、路径与符号链接逃逸均被拒绝 |
| CLI / API 一致性 | 同 revision 与实验配置最终权益均为 `24683.209021659102`，包含 67 条账本事件 |

真实行情：ETH-USDT 永续、15m，2026-10-01 至 2026-10-15；每组 1,344 根。数据集 `binance-perpetual-15m-20260805140000-20261103140000`；manifest SHA-256 为 `61f247bd0c69998c29fafcbaca997c8d3d09dfa3704477df875c7e500dfd3423`。

- 14 组多空、百分比 / 固定价差 / 三种 ATR 采样及间距递增组合，订单、成交、资金曲线、指标和最终权益的 SHA-256 全部与第一阶段证据一致。
- 两组持仓负偏移案例仍各有 896 笔信号、895 笔成交；1,792 笔信号逐笔满足上一根收盘持仓条件。
- 上述 16 组共重放核验 2,772 条账本事件；16 份归档响应逐项相等。
- 独立实验将资金改为 25,000，单标的 / 总敞口上限改为 0.1 / 0.15，原策略 snapshot 与 revision 不变；最大新仓准入比例为 `0.10000000000000002`。

风险额度沿用 V1 的成交前权益准入语义。成交后价格和费用可改变敞口比例，此实验最大收盘实际杠杆为 `0.10136094099605678`，不会自动调仓回限额。

浏览器验收使用实际 UI 操作：

- 25,000 初始资金的仿真回测最终权益为 25,147.49；“快照重跑”得到相同结果。
- 选择 ETH 真实行情与 ATR 加仓策略，设置上述区间后，页面显示 1,344 / 1,344 根、69 笔成交、权益 9,795.41，与基线一致；ATR 图层展示正确。
- 策略管理、能力目录及运行历史可读取；790 像素视口下未出现页面水平溢出。

机器可读证据：[strategy-architecture-completion-validation.json](strategy-architecture-completion-validation.json)。复核脚本：[scripts/validate_architecture.py](scripts/validate_architecture.py)。哈希采用排序、紧凑 UTF-8 JSON；比较剔除新增 ledger 字段，新增运行来源另行记录。脚本优先复用已完成归档，缺少案例时仅写隔离预览数据库。

## 当前能力边界

本轮没有新增 Schema V2 的通用算术 / 时间表达式、声明状态机、动作批次、限价 / 挂单生命周期、HEDGE、多标的组合、资金费 / 强平 / 交易精度模型、期权、样本外参数实验或完整诊断原因码。能力目录目前管理指标与已实现的执行能力元数据，通用字段 Schema 与组件注册仍可继续扩展。

仍使用单进程 SQLite 与 JSON 产物，报告内部保留完整指标历史；本轮未宣称长区间内存或性能改善。重跑通过当前引擎实现重新编译，未归档可执行引擎二进制以保证跨未来版本完全一致。恢复策略为标记 interrupted 并允许显式快照重跑，未实现断点续算。

本地预览：[http://127.0.0.1:8083](http://127.0.0.1:8083)。使用 `.tmp-architecture-validation/preview.db`，验证未写入正式 `data/app.db`。本轮代码未提交或推送。
