# 定位与职责

[English](../en/overview.md) · [文档索引](README.md)

## 三层系统

| 组件 | 负责 | 不负责 |
| --- | --- | --- |
| QuantScore（Python 研究员 Agent） | 分析行情、提出推荐和因子研究提议 | 代替确定性回测证明结果 |
| QuantBrew（Rust 量化内核） | 持股策略、回测、持仓相关计算 | 把研究记忆保存在 Agent 聊天上下文中 |
| KairosFlux（Go 数据库） | 持久化时序行情、研究记录、版本化状态和可查询证据 | 调度 Agent、执行 Rust 程序、决定实盘交易 |

KairosFlux 的“AI native”指研究员能取回有时间边界和来源的上下文、提交可追溯的 Proposal，并查询实验/策略证据；不等于把 Agent 运行框架或策略调度器放进数据库。现有顶层 Go `Engine` 已公开 `BuildContext`、`SubmitProposal`、`GetAsOf` 等方法。Python 客户端目前侧重 Kair 行情写入和部分 v2 操作，不能把 Go API 等同于已经交付的 Python Agent 工具集。

## 当前实现与计划

- **当前**：单机持久化、Kair 网络入口、嵌入式 Go API、版本化读写、审计与 AI Context/Proposal 实现；另有可选 Raft/分片路径。
- **当前但不属于目标职责**：独立 `cmd/kairosflux-jobctl` 的 apply/tick/run/import-registry，服务端按配置启用的文件/ClickHouse 投递；投递默认关闭。
- **已决定、尚未实施**：删除 Job 调度/执行、内置投递及“投递完成后回收”的语义；保留研究状态、证据和历史查询。具体代码拆除须保护 AI 数据平面仍在使用的类型与索引。
- **待设计验证**：单只股票过去 N 天的读取与股票优先键布局；旧行情数据需离线转换和核验。新的研究过程、交易 Handoff 记录契约尚未在本仓库统一定义，不应视为已交付接口。

[架构与组件](architecture.md) · [数据与时间语义](data-model.md)
