# KairosFlux

[简体中文](README.zh-CN.md) · [English documentation](docs/en/README.md)

**KairosFlux is the time-series and research-memory layer for the QuantScore researcher agent and the QuantBrew quantitative engine.** It durably stores market data, versioned research records, and backtest evidence so a researcher can inspect what was known at a past point in time, with attributable writes instead of memory living only in an agent chat. The default is a standalone Go database (WAL + LSM); distributed paths are optional.

| System | Responsibility |
| --- | --- |
| **QuantScore · Python** | Market analysis, stock recommendations, factor research and weighting; deciding when to start research |
| **QuantBrew · Rust** | Position strategies, quantitative backtesting, position-related computation |
| **KairosFlux · Go** | Persisting and querying quotes, proposals, research state, outcomes and evidence; not scheduling either upstream system or deciding trades |

> **Separate shipped code from direction:** versioned point-in-time reads, auditing, and Go `Context`/`Proposal` APIs exist. That does not mean a complete Python agent tool interface, a unified research-process/trade-handoff schema, or a dedicated N-day single-symbol query is already shipped. The repository still contains a separate Job scheduler command and opt-in background delivery; later code changes are intended to remove them. This documentation change does not alter those paths.

## Why persist research history?

A recommendation should answer: “Which market data was known? What did the agent propose? Which Rust experiment tested it? Why was it adopted or rejected?” The KairosFlux versioned path supports:

- `PUT_VERSIONED` appends an immutable version of a logical key. Ordinary `PUT` instead overwrites a literal key.
- `GET_AS_OF(key, t)` selects a version known by **write time** `t`, preventing later-ingested corrections from silently changing the account of a past decision. Market date and ingestion time are different concepts.
- `LIST_VERSIONS`, `LIST_WRITES`, and `REPLAY_FINGERPRINT` expose history, write provenance, and deterministic replay checks.
- The Go `Engine.BuildContext` assembles research context, while `Engine.SubmitProposal` records traceable agent proposals. The current Python client is oriented toward quote ingestion; these Go methods are not already available as equivalent Python agent tools.

Store research progress and negative evidence, but let external Python/Rust programs decide when to launch an agent, run a backtest, retry, or approve live trading. See [data and time semantics](docs/en/data-model.md) for the exact boundaries.

## Implemented components

```text
QuantScore (Python) ─┐
                     ├─ quotes / proposals / backtest evidence / research state ─▶ Kair server ─▶ WAL + MemTable + SSTable
QuantBrew (Rust) ────┘                                       │
                                 embedded Go Engine ────────┘
```

| Capability | Current scope |
| --- | --- |
| Persistence | Standalone WAL + LSM (MemTable, SSTable, compaction, range scan), restart recovery; optional Raft/sharding paths |
| Data entry | Kair v1 basic KV/SCAN and v2 versioned reads/writes and audit; gRPC is a benchmark/comparison endpoint |
| Contracts | Server startup loads `contracts/*.schema.json` for registered type validation; the embedded Go `Engine` does not load them automatically |
| AI data plane | Existing Go embedded `BuildContext`/`SubmitProposal` and proposal/evidence logic; not a complete network-facing agent toolset |

**Important limits:** the daily quote example uses ordinary PUT at `quote:<date>:<symbol>`; one symbol's N-day quotes are not contiguous in this key order. Business `SCAN` returns at most 10,000 entries and does not gather across shards. A symbol-first layout, complete series API, and offline conversion of old data remain to be designed and verified—not shipped features. See [architecture and status](docs/en/architecture.md).

## Quick start

Requires **Go 1.26.1+**. Build at the repository root:

```sh
go build ./...
```

In one terminal, start the server from its command directory. The checked-in `config/config.json` uses relative data paths: **working directory affects where WAL and SSTable files land**. Resolve those paths before production use.

```sh
cd cmd/kairosflux-server
go run .
```

In a second terminal at the repository root, write and read back a daily quote (the checked-in config listens on `127.0.0.1:8080`):

```sh
python3 client/python/examples/write_quote.py --addr 127.0.0.1:8080 \
  --code 600000 --date 2026-08-17 \
  --open 10 --high 11 --low 9 --close 10.5 --volume 1000000
```

This sample uses the legacy-named Python `BanDBClient` class and Kair v1 ordinary PUT, not Agent Proposal or immutable quote versions. To exercise the **separate** versioned path:

```sh
go run ./cmd/kairosflux-cli -addr 127.0.0.1:8080 put-versioned research:demo '{"idea":"v1"}' researcher
go run ./cmd/kairosflux-cli -addr 127.0.0.1:8080 put-versioned research:demo '{"idea":"v2"}' researcher
go run ./cmd/kairosflux-cli -addr 127.0.0.1:8080 list-versions research:demo
go run ./cmd/kairosflux-cli -addr 127.0.0.1:8080 fingerprint research:
```

`list-versions` prints `write_nanos` for each version. Supply one of those nanosecond timestamps to `get-as-of research:demo <as_of_nanos>` to check the version visible at that point. `research:demo` is an illustrative key, **not** a published research-process schema. To try the Go API without a network listener:

```sh
go run ./cmd/kairosflux-sample-embedded -data-dir /tmp/kairosflux-demo
```

## Current limits and next steps

1. **Single-symbol series reads:** measure correctness and scan costs for real N-day reads, then define a symbol-first layout. Freeze writes, back up, convert, verify, then switch old quotes. Do not add dual-write or rewrite LSM up front.
2. **Responsibility cleanup:** the separate Job scheduler/executor and built-in file/ClickHouse delivery still exist. Delivery-cursor-triggered SSTable reclamation is not a long-term research-memory retention policy and cannot be reused unchanged.
3. **Distribution:** keep it optional, but current range scans are local to one shard, not cross-shard symbol-series queries.
4. **Agent access:** Go Context/Proposal methods exist; direct Python researcher tools and complete research/handoff contracts still need validation against actual integration paths.

## Documentation, historical RFCs, and verification

- [Bilingual documentation index](docs/en/README.md): [purpose](docs/en/overview.md), [architecture](docs/en/architecture.md), [data model](docs/en/data-model.md), [operations](docs/en/operations.md), [protocol entry point](docs/en/protocol.md).
- [Historical RFC index](docs/rfc/README.md): Kair v2, bannet restructuring, temporal M0, and key-space partitioning. **Past proposals are not current implementation claims.**
- `contracts/` contains machine-readable data contracts; `docs/kair/vectors*.json` holds Go/Python protocol test vectors, not prose documentation.

```sh
go test ./...
python3 -m unittest discover -s client/python -p 'test_*.py'
```

See [operations, backup, and verification](docs/en/operations.md) for details.
