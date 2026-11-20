# Kair 协议入口

[English](../en/protocol.md) · [文档索引](README.md)

Kair 是生产网络入口；Go 嵌入式 `Engine` 不经过网络。本页是当前协议的**导航**，不替代实现，也不承诺旧 RFC 的所有逐字段细节已在此重写。

| 层 | 当前来源 |
| --- | --- |
| v1 帧与客户端 | `kairnet/codec/datapack.go`、`proto/codes.go`、`client/` |
| v2 帧、协商与请求分派 | `kairnet/codec/v2.go`、`kairnet/negotiate/`、`service/router_v2.go` |
| 时态请求/响应负载 | `proto/temporal_frame.go`、`service/temporal_store.go` |
| 任务控制负载（手写 TLV，checkpoint/outcome 为不透明块） | `proto/task_frame.go`、`internal/taskplane`、`service/task_store.go` |
| Go/Python 跨语言固定字节向量 | `docs/kair/vectors.json`、`docs/kair/vectors-v2.json` |

v1 提供基础 PUT/GET/DEL/SCAN；v2 包含版本化写入、as-of 读、版本清单与审计等操作。`TASK_CLAIM/CHECKPOINT/RENEW/COMPLETE/FAIL` 通过版本化状态协调任务（租约过期是读时判定，数据库内不运行调度器）。普通 PUT 是字面量键覆盖写，与 PUT_VERSIONED 的不可变版本不同。网络协议中的 Agent 来源约束主要在 v2 `PUT_VERSIONED` 路径；v1 PUT/DEL 不携带同样的来源角色字段，进程内 Go API 也不经过网络角色检查。不能把这些入口一概描述成“所有 Agent 写操作都受同一授权策略保护”。

改动帧格式时应以实际编解码实现和静态测试向量共同校验兼容性；仅凭本页描述不足以实现新客户端。[Kair 2 历史 RFC](../rfc/Kair-2.md) 和 [bannet 重构记录](../rfc/bannet-重构.md) 已作为历史设计资料保留，不应代替当前实现。
