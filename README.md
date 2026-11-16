# KairosFlux

[简体中文](README.zh-CN.md) · [English documentation](docs/en/README.md)

KairosFlux is a time-series and research-memory store for the QuantScore researcher agent and the QuantBrew quantitative engine. It persists market data, immutable research proposals, versioned state, and backtest evidence, with point-in-time reads and write auditing. Python owns analysis and recommendations; Rust owns position strategies and backtesting. KairosFlux does not decide when either runs or trades.

Standalone WAL + LSM is the default storage path; Raft/sharding is optional. The repository **still contains** a separate Job scheduling command and an opt-in file/ClickHouse delivery loop. Both are slated for removal but have not been removed. Quote keys currently use `quote:<date>:<symbol>`; the single-symbol N-day read path and key layout have **not yet been migrated**.

## Quick check

Go 1.26+ is required. From the repository root:

```sh
go build ./...
cd cmd/kairosflux-server
go run .
```

In another terminal, write a daily quote (the configured server listens on `127.0.0.1:8080` by default):

```sh
python3 client/python/examples/write_quote.py --addr 127.0.0.1:8080 \
  --code 600000 --date 2026-08-17 \
  --open 10 --high 11 --low 9 --close 10.5 --volume 1000000
```

To exercise the Go embedded API without opening a network listener:

```sh
go run ./cmd/kairosflux-sample-embedded -data-dir /tmp/kairosflux-demo
```

Tests: `go test ./...`; Python protocol tests: `python3 -m unittest discover -s client/python -p 'test_*.py'`.

## Documentation

- [Purpose and responsibilities](docs/en/overview.md)
- [Architecture and current state](docs/en/architecture.md)
- [Data and time semantics](docs/en/data-model.md)
- [Operations, backup, and verification](docs/en/operations.md)
- [Kair protocol entry point](docs/en/protocol.md)

The [documentation index](docs/en/README.md) links the corresponding Chinese and English pages. `contracts/` holds machine-readable contracts; `docs/kair/*.json` holds cross-language protocol test vectors, not prose documentation. Older design and iteration notes remain available in Git history. Planned changes are labeled as such rather than described as shipped behavior.
