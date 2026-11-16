# KairosFlux

[English](README.md) · [中文文档](docs/zh-CN/README.md)

KairosFlux 是面向 QuantScore 研究员 Agent 与 QuantBrew 量化内核的时序数据与研究记忆层。它保存行情、不可变的研究提议、版本化状态和回测证据，并支持按写入时点回看与审计。Python 层负责分析与推荐，Rust 层负责持股策略和回测；KairosFlux 不应替它们决定何时运行或交易。

项目目前以单机 WAL + LSM 为默认路径，Raft/分片为可选能力。仓库中**仍有**独立的 Job 调度命令和默认关闭的文件/ClickHouse 后台投递；这两部分已列为待移除项，尚未删掉。股票行情现有键为 `quote:<日期>:<代码>`，单股过去 N 天查询的键布局与读取路径尚待优化，**尚未迁移**。

## 快速验证

需要 Go 1.26+。在仓库根目录运行：

```sh
go build ./...
cd cmd/kairosflux-server
go run .
```

另开终端，写入一条日线快照（服务端默认监听 `127.0.0.1:8080`，按实际配置调整）：

```sh
python3 client/python/examples/write_quote.py --addr 127.0.0.1:8080 \
  --code 600000 --date 2026-08-17 \
  --open 10 --high 11 --low 9 --close 10.5 --volume 1000000
```

不启动网络服务的 Go 嵌入式样例：

```sh
go run ./cmd/kairosflux-sample-embedded -data-dir /tmp/kairosflux-demo
```

运行测试：`go test ./...`；Python 协议测试：`python3 -m unittest discover -s client/python -p 'test_*.py'`。

## 文档

- [定位与职责](docs/zh-CN/overview.md)
- [架构与现状](docs/zh-CN/architecture.md)
- [数据与时间语义](docs/zh-CN/data-model.md)
- [运行、备份与验证](docs/zh-CN/operations.md)
- [Kair 协议入口](docs/zh-CN/protocol.md)

中英文文档一一对应，入口见[文档索引](docs/zh-CN/README.md)。`contracts/` 是机器可读契约；`docs/kair/*.json` 是 Go/Python 共用的协议测试向量，不是说明文档。历史设计与迭代记录可从 Git 历史查看。本文档只描述当前实现及明确标记的待办，不把目标架构写成现状。
