# Architecture and implementation status

[简体中文](../zh-CN/architecture.md) · [Documentation index](README.md)

```text
QuantScore (Python, research/recommendations) ─┐
                                                ├─ quotes, proposals, evidence, state ─▶ KairosFlux
QuantBrew (Rust, backtests/positions) ──────────┘                                      │
                                                   Kair server / embedded Go Engine
                                                               │
                                                   WAL → MemTable → SSTable (LSM)
```

This illustrates ownership, not a promise that every Python/Rust research object has its own network API. The database stores and queries; model invocation, backtesting, and trading are decided outside it.

## Current entry points

- `cmd/kairosflux-server` builds `service.Node`: it loads `contracts/*.schema.json` and wires v1/v2 Kair handlers, the ingestion filter, and config-gated features. In standalone mode `service.KVServer` durably writes through WAL; storage handles MemTable, SSTable, range scans, and compaction.
- The root Go `Engine` exports `NewEmbedded`, `Open`, and `Serve`. Embedded mode opens no network listener and supports standalone only. It shares the storage and core routing path with the production `Node`, but **does not automatically load** the `contracts/` validation files. Do not assume the two startup modes validate all inputs identically.
- `client/python/bandb_client.py` supports Kair quote ingestion and some v2 operations. The Go AI Context/Proposal entry points are in `kairosflux.go`; this does not imply equivalent high-level Python methods already exist.

## Optional and pending removal

`raft/` and `cluster/` contain optional distributed paths. Server-side `SCAN` currently reads only the local shard; it does not perform cross-shard scatter-gather and is not a complete distributed time-range query.

The Job scheduler/executor and its CLI have been removed. `cmd/kairosflux-import-registry` retains the versioned import of QuantBrew experiment records consumed by the AI data plane. Built-in file/ClickHouse delivery and delivery-triggered SSTable reclamation have been removed; the database only persists and queries records.

[Module map and reading routes](modules.md) · [Data and query boundaries](data-model.md) · [Operations and backup](operations.md)
