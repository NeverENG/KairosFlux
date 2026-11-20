# Module map

[简体中文](../zh-CN/modules.md) · [Documentation index](README.md) · [Architecture overview](architecture.md)

This is a **navigation map**, not a file-by-file implementation guide. Paths are relative to the repository root. “Current” means the code exists here; it does not imply every interface is called by QuantScore or QuantBrew. Start with [system responsibilities](overview.md) and [time semantics](data-model.md), then follow the relevant modules.

## Main reading path

```text
caller → client / kairosflux Go API → kairnet → proto → service → storage
                                      └──────────────→ internal/temporal (as-of semantics)
```

This is a reading order, not a claim that every request traverses every layer: the embedded Go API does not use the network, and `service` wires the Kair handlers. Raft/sharding is optional. Job scheduling and built-in delivery have been removed.

## Public entry points and data contracts

| Module | What to read it for |
| --- | --- |
| [Root Go API](../../kairosflux.go) | `Engine` constructors (embedded/network), versioned operations, audit, Context, and Proposal; the standalone library entry point. |
| [client/](../../client/) | Go Kair client, connections, basic KV requests, and error semantics. |
| [client/python/](../../client/python/) | Python Kair client with legacy `BanDBClient` naming and cross-language tests; mainly quote ingestion, **not** a complete researcher-agent tool layer. |
| [contracts/](../../contracts/) | Machine-readable contracts, including quote and AI objects; not every contract is enforced by the server. |
| [riskredlines/](../../riskredlines/) | On-disk risk limits consumed by `BuildContext`; not versioned ledger entries. |
| [config/](../../config/) | Standalone/cluster settings, data paths, and optional feature switches. |
| [proto/](../../proto/) | Kair application messages and scan/temporal request-response payloads, separate from transport framing. |
| [predicate/](../../predicate/) | Simple business `SCAN` predicates. |

## Network transport: kairnet

| Module | Reading entry point |
| --- | --- |
| [kairnet/](../../kairnet/) | Server and public facade wiring the network layers. |
| [codec/](../../kairnet/codec/) | v1/v2 frame encoding and decoding. |
| [negotiate/](../../kairnet/negotiate/) | Protocol version and acknowledgement-mode negotiation. |
| [handler/](../../kairnet/handler/) | Business handler, request, and connection interfaces. |
| [dispatch/](../../kairnet/dispatch/) | Routing decoded requests to handlers. |
| [transport/](../../kairnet/transport/) | Connection implementation and I/O path. |
| [lifecycle/](../../kairnet/lifecycle/) | Connection running/closing states. |

For the original separation rationale, see the [historical bannet refactoring RFC](../rfc/bannet-重构.md). Current protocol behavior is defined by code and [test vectors](../kair/), not by the older proposal alone.

## Service assembly and storage

| Module | Reading entry point |
| --- | --- |
| [service/](../../service/) | `Node`, KV service, v1/v2 routers, temporal-store and task-store assembly; distinguish ordinary PUT from versioned operations. |
| [service/ingesthook/](../../service/ingesthook/) | Pre-write frame, value-size and schema validation; no device-specific redaction or timestamp heuristic. This hook is **not** an agent tool registry. |
| [service/ingesthook/schema/](../../service/ingesthook/schema/) | Loading registered data contracts and validating quotes. |
| [storage/](../../storage/) | LSM/SSTable, range scans, compaction, and MemTable; standalone WAL wiring is in `service/`. |
| [internal/temporal/](../../internal/temporal/) | Storage/network-independent version keys, as-of selection, and fingerprint semantics. |
| [internal/credit/](../../internal/credit/) | Byte budget and write backpressure for unflushed data. |
| [internal/metrics/](../../internal/metrics/) | Server/storage counters and runtime metrics. |
| [internal/identity/](../../internal/identity/) | Shared agent-source role and Proposal key-prefix checks. |
| [internal/aiplane/](../../internal/aiplane/) | Go-side Context, Proposal, similarity, and evidence graph; not pre-built Python tools. |
| [internal/taskplane/](../../internal/taskplane/) | Pure task state machine (claim/checkpoint/complete/fail, lease expiry as a read-time check); IO shell is `service/task_store.go`. |

## Optional and pending removal

| Module | Current role / boundary |
| --- | --- |
| [raft/](../../raft/) | Optional Raft replication, log, and snapshot path; not enabled in standalone mode. |
| [cluster/](../../cluster/) | Optional ownership, routing, and replica read selection; current `SCAN` does not aggregate shards. |
| [service/shardkv/](../../service/shardkv/) | Multi-Raft shard prototype behind the `experimental` build tag; excluded by default. |
| [internal/admission/](../../internal/admission/) | Config-gated gateway admission and overload control. |
| [internal/researchdata/](../../internal/researchdata/) | Research data client, QuantBrew experiment import/index, and strategy phase values; no Job runner. |

## Programs, samples, and supporting files

| Entry point | Purpose |
| --- | --- |
| [cmd/kairosflux-server/](../../cmd/kairosflux-server/) | Production Kair server. |
| [cmd/kairosflux-cli/](../../cmd/kairosflux-cli/) | Basic KV, point-in-time reads, and audit commands. |
| [cmd/kairosflux-import-registry/](../../cmd/kairosflux-import-registry/) | One-shot QuantBrew experiment data import; no scheduler. |
| [cmd/kairosflux-bench/](../../cmd/kairosflux-bench/) | Kair protocol and storage benchmarks. |
| [cmd/kairosflux-ingest/](../../cmd/kairosflux-ingest/) | Storage-engine ingestion benchmark. |
| [cmd/kairosflux-crashsim/](../../cmd/kairosflux-crashsim/) | Crash/recovery experiment helper. |
| [cmd/kairosflux-sample-embedded/](../../cmd/kairosflux-sample-embedded/) | Network-free Go Engine example. |
| [cmd/kairosflux-sample-server/](../../cmd/kairosflux-sample-server/) | Network-mode end-to-end example. |
| [cmd/kairosflux-sample-audit/](../../cmd/kairosflux-sample-audit/) | Version writes and audit-export example. |
| [scripts/](../../scripts/) | Benchmark helper scripts. |
| [docs/kair/](../kair/) | Go/Python byte-level protocol test vectors, not prose documents. |

**Choose a route:** researcher integration: `kairosflux.go` → `internal/aiplane` → `internal/temporal`; quotes/single-symbol series: `contracts/quote.schema.json` → `service/ingesthook/schema` → `service` → `storage`; network: `kairnet` → `proto` → `service`; recovery: `service` → `storage` → [operations](operations.md).
