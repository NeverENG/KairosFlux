# 架构与实现现状

[English](../en/architecture.md) · [文档索引](README.md)

```text
QuantScore (Python, 分析/推荐) ─┐
                               ├─ 行情、研究提议、证据、状态 ─▶ KairosFlux
QuantBrew (Rust, 回测/持仓) ────┘                             │
                                        Kair 网络入口 / Go 嵌入式 Engine
                                                │
                                        WAL → MemTable → SSTable (LSM)
```

此图描述职责，不表示 Python/Rust 所有研究对象都已有专用网络接口。数据库负责保存和查询；何时调用模型、回测或交易由数据库外部决定。

## 当前入口

- `cmd/kairosflux-server` 构造 `service.Node`：加载 `contracts/*.schema.json`，接上 v1/v2 Kair 处理器、采集过滤器和配置门控能力。`service.KVServer` 在 standalone 模式经 WAL 持久化写入，存储层负责 MemTable、SSTable、范围扫描与压缩。
- 仓库根的 Go `Engine` 提供 `NewEmbedded`、`Open`、`Serve`；嵌入式形态不开放网络，仅支持 standalone。它与生产 `Node` 共用存储及核心路由，但**不自动加载** `contracts/` 校验文件；使用者不能把两种启动形态的输入校验误认为完全相同。
- `client/python/bandb_client.py` 可通过 Kair 写行情和执行部分 v2 操作；`internal/kvgrpc` 是基准/协议对照入口，不是生产主入口。AI Context/Proposal 的 Go API 见 `kairosflux.go`；Python 侧不应凭此假设已有同名高级调用。

## 可选与待移除能力

`raft/` 和 `cluster/` 提供可选分布式路径；当前服务端 `SCAN` 只扫描本地，不做跨分片 scatter-gather，不能把它当成完整的分布式时间范围查询。

`cmd/kairosflux-jobctl` 是**进程外独立命令**，含执行/调度能力，不是服务端自动运行的研究调度器。服务端 `StartDeliveryFromConfig` 可启动文件/ClickHouse 后台投递，默认关闭。删除这两条链路是**后续代码变更**，尚未发生。AI 数据平面对 `internal/jobctl` 的共用类型和索引存在依赖，不能整包盲删。

[模块地图与阅读路线](modules.md) · [数据和查询边界](data-model.md) · [运行与备份](operations.md)
