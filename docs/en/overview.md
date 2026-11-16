# Purpose and responsibilities

[简体中文](../zh-CN/overview.md) · [Documentation index](README.md)

## Three layers

| Component | Owns | Does not own |
| --- | --- | --- |
| QuantScore (Python researcher agent) | Market analysis, recommendations, factor research proposals | Claiming deterministic backtest results without verification |
| QuantBrew (Rust quantitative engine) | Position strategies, backtests, position-related computation | Keeping research memory solely in agent chat context |
| KairosFlux (Go database) | Durable market series, research records, versioned state, queryable evidence | Scheduling agents, running Rust programs, authorizing live trades |

“AI native” here means researchers can retrieve time-bounded, attributable context, submit traceable proposals, and inspect experimental or strategy evidence. It does not mean putting an agent runtime or scheduler in the database. The current top-level Go `Engine` exports `BuildContext`, `SubmitProposal`, and `GetAsOf`. The Python client focuses on Kair quote ingestion and a subset of v2 operations; Go API availability must not be mistaken for a complete Python agent toolset.

## Shipped versus planned

- **Current**: standalone persistence, Kair server, embedded Go API, versioned reads/writes, auditing, AI Context/Proposal implementation, and optional Raft/sharding paths.
- **Present but outside the target boundary**: the separate `cmd/kairosflux-jobctl` apply/tick/run/import-registry command and configurable file/ClickHouse delivery. Delivery is off by default.
- **Decided, not yet implemented**: remove Job scheduling/execution, built-in delivery, and delivery-triggered reclamation; retain research state, evidence, and historical reads. Removing code must preserve types and index contracts still consumed by the AI data plane.
- **To design and validate**: single-symbol N-day reads and a symbol-first key layout, with offline conversion and verification of old quotes. Unified contracts for the full research process and trade handoff have not yet been delivered in this repository.

[Architecture and components](architecture.md) · [Data and time semantics](data-model.md)
