# KairosFlux

[English](README.md) · [中文文档](docs/zh-CN/README.md)

**KairosFlux 是 QuantScore 研究员 Agent 与 QuantBrew 量化内核的时序数据和研究记忆层。** 它在本地持久化行情、版本化研究记录与回测证据，让研究过程可以按时点回看、按来源核对，而不是只存在 Agent 的聊天上下文里。默认形态是单机 Go 数据库（WAL + LSM）；分布式路径可选。

| 系统 | 边界 |
| --- | --- |
| **QuantScore · Python** | 分析行情、推荐股票、研究因子及配比；决定何时发起研究 |
| **QuantBrew · Rust** | 持股策略、量化回测与持仓相关计算 |
| **KairosFlux · Go** | 保存和查询行情、提议、研究状态、结果及证据；不替前两者调度任务或决定交易 |

> **现状与方向分开看：** 仓库已有版本化时点读、审计和 Go `Context`/`Proposal` API；并不表示完整的 Python Agent 工具接口、研究过程/Handoff 统一 schema、或按单股 N 天的专用查询已经实现。Job 调度/执行已移除；默认关闭的后台投递仍存在，待独立清理。

## 为什么要保存研究过程

一个推荐必须能回答：“当时系统知道哪些行情？Agent 提出什么假设？Rust 用哪个实验结果验证？后来为什么采纳或否决？” KairosFlux 的版本化路径支持：

- `PUT_VERSIONED` 给逻辑键追加不可变版本；与普通 `PUT` 覆盖字面量键的语义不同。
- `GET_AS_OF(key, t)` 按**写入时刻**读取 `t` 时已知的版本，避免用未来才入库的修正数据解释历史决策。交易日期和写入日期并不是同一种时间。
- `LIST_VERSIONS`、`LIST_WRITES` 和 `REPLAY_FINGERPRINT` 提供版本历史、写入来源审计与确定性重放核对。
- Go `Engine.BuildContext` 组装研究上下文，`Engine.SubmitProposal` 写入可追溯的 Agent 提议。当前 Python 客户端偏向行情摄入，不能把 Go API 直接当成已存在的 Python 工具方法。

研究进度和失败证据值得存储；何时启动 Agent、执行回测、重试、放行实盘则由 Python/Rust 外部程序负责。[时间语义与数据边界](docs/zh-CN/data-model.md)有更准确的说明。

## 已实现的组成

```text
QuantScore（Python）─┐
                   ├─行情 / 提议 / 回测证据 / 研究状态─▶ Kair 网络入口 ─▶ WAL + MemTable + SSTable
QuantBrew（Rust）───┘                                      │
                              Go 嵌入式 Engine ────────────┘
```

| 能力 | 当前范围 |
| --- | --- |
| 持久化存储 | 单机 WAL、LSM（MemTable、SSTable、压缩、范围扫描），重启后恢复；可选 Raft/分片路径 |
| 数据入口 | Kair v1 基础 KV/SCAN、v2 版本化读写和审计；gRPC 入口用于基准/对照 |
| 契约 | 服务端启动加载 `contracts/*.schema.json` 并校验已注册类型；Go 嵌入式 `Engine` 不自动加载这些文件 |
| AI 数据平面 | Go 嵌入式 `BuildContext`/`SubmitProposal`、提议与证据查询的现有实现；不是完整的网络侧 Agent 工具集 |

**明确边界：** 行情示例使用 `quote:<日期>:<代码>` 的普通 PUT；单股过去 N 天的记录在键空间不连续。当前业务 `SCAN` 最多返回 10,000 条，且跨分片扫描不会自动汇总。股票优先布局、完整的序列查询与旧数据离线迁移仍在设计/验证阶段；这里不把目标当作已交付特性。[架构与现状](docs/zh-CN/architecture.md)列出更多实现边界。

## 快速开始

需要 **Go 1.26.1+**。先在仓库根目录构建：

```sh
go build ./...
```

在一个终端从服务端目录启动。仓库现有 `config/config.json` 使用相对数据路径，因此**工作目录会影响 WAL/SSTable 存放位置**；生产使用前请核对实际路径。

```sh
cd cmd/kairosflux-server
go run .
```

另一个终端从仓库根目录写入并读回一条示例日线快照（配置文件监听 `127.0.0.1:8080`）：

```sh
python3 client/python/examples/write_quote.py --addr 127.0.0.1:8080 \
  --code 600000 --date 2026-08-17 \
  --open 10 --high 11 --low 9 --close 10.5 --volume 1000000
```

此示例通过 Python `BanDBClient` 的历史类名调用 Kair v1 普通 PUT，不是 Agent Proposal 或不可变行情版本写入。用 CLI 演示**另一条**版本化路径：

```sh
go run ./cmd/kairosflux-cli -addr 127.0.0.1:8080 put-versioned research:demo '{"idea":"v1"}' researcher
go run ./cmd/kairosflux-cli -addr 127.0.0.1:8080 put-versioned research:demo '{"idea":"v2"}' researcher
go run ./cmd/kairosflux-cli -addr 127.0.0.1:8080 list-versions research:demo
go run ./cmd/kairosflux-cli -addr 127.0.0.1:8080 fingerprint research:
```

`list-versions` 输出每版的 `write_nanos`；取其中一个纳秒时间戳作为 `get-as-of research:demo <as_of_nanos>` 的参数，可以检查该时点的可见版本。`research:demo` 只是演示键，**不是**已发布的研究过程 schema。若只想验证无网络的 Go API：

```sh
go run ./cmd/kairosflux-sample-embedded -data-dir /tmp/kairosflux-demo
```

## 当前约束与后续工作

1. **单股时间序列读取**：先测真实 N 天查询的完整性和扫描成本，再定义股票优先布局；旧数据将在停写、备份、转换、核验后切换，不先上双写或重做 LSM。
2. **职责瘦身**：Job 调度/执行已移除，内置文件/ClickHouse 投递仍待删除。与投递游标绑定的 SSTable 回收也不是长期记忆层的保留期策略，不能直接沿用。
3. **分布式**：保留可选能力，但当前范围扫描只查本地，不能宣称已支持跨分片单股序列查询。
4. **Agent 接入**：Go 的 Context/Proposal 已有实现；Python 研究员使用的直接工具层与完整研究/Handoff 契约仍需沿实际链路核对。

## 文档、历史 RFC 与验证

- [双语文档索引](docs/zh-CN/README.md)与[模块地图](docs/zh-CN/modules.md)：[定位](docs/zh-CN/overview.md)、[架构](docs/zh-CN/architecture.md)、[数据模型](docs/zh-CN/data-model.md)、[运行与备份](docs/zh-CN/operations.md)、[协议入口](docs/zh-CN/protocol.md)。
- [历史 RFC 索引](docs/rfc/README.md)：保留 Kair v2、bannet 重构、时态内核 M0 与键空间分区的设计背景；**历史方案不是当前实现声明**。
- `contracts/` 是机器可读数据契约；`docs/kair/vectors*.json` 是 Go/Python 协议测试向量，不是说明文档。

```sh
go test ./...
python3 -m unittest discover -s client/python -p 'test_*.py'
```

完整运维注意事项见[运行、备份与验证](docs/zh-CN/operations.md)。
