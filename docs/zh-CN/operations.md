# 运行、备份与验证

[English](../en/operations.md) · [文档索引](README.md)

## 本地启动

Go 1.26+。从仓库根目录构建，再从 `cmd/kairosflux-server` 启动，以便仓库现有 `config/config.json` 中的相对数据路径按预期解析：

```sh
go build ./...
cd cmd/kairosflux-server
go run .
```

另一个终端回到仓库根目录，运行 `python3 client/python/examples/write_quote.py --help` 查看行情写入参数。该示例是普通 PUT 日线快照，不演示 Agent Proposal，也不验证单股 N 天查询。

生产前核对实际配置与工作目录：`config/config.json` 的 `WALPath`、`SSTablePath` 是相对路径；从不同目录启动会解析到不同位置。默认是 standalone，示例配置监听 `127.0.0.1:8080`；不要假设 Python 样例或服务端使用了另一个默认端口。`NewEmbedded` 的数据目录通过 Go `Options.DataDir` 显式指定，不需要网络监听；用 `go run ./cmd/kairosflux-sample-embedded -data-dir /tmp/kairosflux-demo` 验证该路径。

## 备份与恢复

KairosFlux 不是当前架构下“投递后即可丢弃”的缓存。备份时**先停止写入并关闭服务**，核实实际 WAL 文件与 SSTable 目录，再一起复制到单独位置；记录数据目录、配置和备份时间。不要只备份 SSTable，也不要在未验证一致性的情况下把在线目录复制当成安全快照。用独立目录启动恢复副本，检查样例行情、版本历史和审计查询，然后才考虑替换生产目录。

现有可选 `RetentionEnabled` 的语义是“投递游标提交后按 SSTable 整文件回收”，**不是按交易日期或研究保留期清理**。在计划删除投递与该回收链路之前，先检查实际运行配置是否曾开启；已回收的数据不会因代码删除而恢复。

## 验证与已知边界

```sh
go test ./...
python3 -m unittest discover -s client/python -p 'test_*.py'
```

Go/Python 协议测试依赖 `docs/kair/vectors.json` 与 `docs/kair/vectors-v2.json`，重构说明文档时不得删改它们。`go test -race ./...` 可作更严格的并发回归检查。服务端业务范围 `SCAN` 只扫本地，默认结果上限为 10,000；跨分片与完整的单股序列接口均不应视为已验证能力。
