# 模块地图

[English](../en/modules.md) · [文档索引](README.md) · [架构总览](architecture.md)

这是一张**阅读导航图**，不是逐文件实现说明。路径以仓库根目录为基准；“当前”指仓库里已有代码，不表示 QuantScore/QuantBrew 必然调用了每个接口。先看[三层职责](overview.md)与[时间语义](data-model.md)，再按问题进入模块。

## 一条主链路

```text
调用方 → client / kairosflux Go API → kairnet → proto → service → storage
                                       └──────────────→ internal/temporal（时点语义）
```

这只是阅读顺序，不表示每个请求都会经过每一层：嵌入式 Go API 不经网络；Kair 服务端从 `service` 装配业务处理器。Raft/分片为可选路径，Job 调度与内置投递已移除。

## 对外入口与数据契约

| 模块 | 看这里是为了了解什么 |
| --- | --- |
| [仓库根 Go API](../../kairosflux.go) | `Engine` 的嵌入式/网络构造、版本化读写、审计、Context 与 Proposal；单机使用入口。 |
| [client/](../../client/) | Go 的 Kair 客户端；连接、基本 KV 请求和错误语义。 |
| [client/python/](../../client/python/) | 历史类名 `BanDBClient` 的 Python Kair 客户端及跨语言测试；侧重行情摄入，**不是**完整的研究员 Agent 工具层。 |
| [contracts/](../../contracts/) | 机器可读数据契约（包括行情与 AI 对象）；不是每份契约都已由服务端强制校验。 |
| [riskredlines/](../../riskredlines/) | `BuildContext` 读取的风控红线文件；当前是磁盘文件而非版本化账本。 |
| [config/](../../config/) | 单机/集群运行配置、路径和可选功能开关；核对数据目录时先读这里。 |
| [proto/](../../proto/) | Kair 业务消息、扫描与时态操作的请求/响应负载定义；和帧传输层分开。 |
| [predicate/](../../predicate/) | 业务 `SCAN` 的简单谓词。 |

## 网络传输：kairnet

| 模块 | 阅读入口 |
| --- | --- |
| [kairnet/](../../kairnet/) | 服务端及其对外门面；装配各网络子层。 |
| [codec/](../../kairnet/codec/) | v1/v2 帧编码解码。 |
| [negotiate/](../../kairnet/negotiate/) | 连接上的协议版本与 ack 档位协商。 |
| [handler/](../../kairnet/handler/) | 业务处理器、请求与连接的接口契约。 |
| [dispatch/](../../kairnet/dispatch/) | 将已解码请求送至处理器。 |
| [transport/](../../kairnet/transport/) | 连接实现与收发路径。 |
| [lifecycle/](../../kairnet/lifecycle/) | 连接的运行/关闭状态。 |

需要研究历史拆分理由时读[历史 bannet 重构 RFC](../rfc/bannet-重构.md)；协议当前行为以代码和[协议向量](../kair/)为准，不以旧 RFC 取代实现。

## 业务装配与存储

| 模块 | 阅读入口 |
| --- | --- |
| [service/](../../service/) | `Node`、KV 服务、v1/v2 路由、时态存储与任务存储的装配；区分普通 PUT 与版本化操作。 |
| [service/ingesthook/](../../service/ingesthook/) | 写入前帧、值长度和 schema 校验入口；不执行设备字段脱敏或时间戳启发式。这里的 hook **不是** Agent 工具注册。 |
| [service/ingesthook/schema/](../../service/ingesthook/schema/) | 已注册类型的数据契约加载与行情校验。 |
| [storage/](../../storage/) | MemTable、WAL 之外的 LSM/SSTable、范围扫描、压缩；单机 WAL 的装配在 `service/`。 |
| [internal/temporal/](../../internal/temporal/) | 不依赖存储/网络的版本键、as-of 和指纹语义。 |
| [internal/credit/](../../internal/credit/) | 未 flush 数据的字节预算与写入背压。 |
| [internal/metrics/](../../internal/metrics/) | 服务和存储的计数器/运行指标。 |
| [internal/identity/](../../internal/identity/) | Agent 来源角色及 Proposal 键前缀的共享判定。 |
| [internal/aiplane/](../../internal/aiplane/) | Context（含任务进度与待裁决提议）、Proposal、引擎侧 Verdict、相似度与证据图谱的 Go 侧实现；不要理解成现成 Python 工具。 |
| [internal/taskplane/](../../internal/taskplane/) | 纯任务状态机（claim/checkpoint/complete/fail，租约过期是读时判定）；IO 壳在 `service/task_store.go`。 |

## 可选与待移除部分

| 模块 | 当前角色 / 边界 |
| --- | --- |
| [raft/](../../raft/) | 可选 Raft 复制、日志和快照路径；单机默认不启用。 |
| [cluster/](../../cluster/) | 可选归属计算、路由和副本读选择；当前范围 `SCAN` 不跨分片汇总。 |
| [service/shardkv/](../../service/shardkv/) | `experimental` 构建标签下的 Multi-Raft 分片原型，默认构建不包含。 |
| [internal/admission/](../../internal/admission/) | 按配置启用的网关准入/过载控制。 |
| [internal/researchdata/](../../internal/researchdata/) | 研究数据客户端、QuantBrew 实验导入/索引和策略阶段值；无 Job 执行器。 |

## 可执行程序、样例和辅助文件

| 入口 | 用途 |
| --- | --- |
| [cmd/kairosflux-server/](../../cmd/kairosflux-server/) | 生产 Kair 服务端入口。 |
| [cmd/kairosflux-cli/](../../cmd/kairosflux-cli/) | 基本 KV、时点查询与审计命令。 |
| [cmd/kairosflux-import-registry/](../../cmd/kairosflux-import-registry/) | 一次性导入 QuantBrew 实验记录，不负责调度。 |
| [cmd/kairosflux-bench/](../../cmd/kairosflux-bench/) | Kair 协议与存储压测入口。 |
| [cmd/kairosflux-ingest/](../../cmd/kairosflux-ingest/) | 存储引擎摄入基准。 |
| [cmd/kairosflux-crashsim/](../../cmd/kairosflux-crashsim/) | 崩溃/恢复实验辅助进程。 |
| [cmd/kairosflux-sample-embedded/](../../cmd/kairosflux-sample-embedded/) | 无网络 Go Engine 样例。 |
| [cmd/kairosflux-sample-server/](../../cmd/kairosflux-sample-server/) | 网络模式端到端样例。 |
| [cmd/kairosflux-sample-audit/](../../cmd/kairosflux-sample-audit/) | 版本写入和审计导出样例。 |
| [scripts/](../../scripts/) | 基准辅助脚本。 |
| [docs/kair/](../kair/) | Go/Python 共用协议字节向量，是测试资产，非普通文章。 |

**按需求阅读：** 研究员接入看 `kairosflux.go` → `internal/aiplane` → `internal/temporal`；行情/单股序列看 `contracts/quote.schema.json` → `service/ingesthook/schema` → `service` → `storage`；网络问题看 `kairnet` → `proto` → `service`；故障恢复看 `service` → `storage` → [运维说明](operations.md)。
