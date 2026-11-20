# Kair protocol entry point

[简体中文](../zh-CN/protocol.md) · [Documentation index](README.md)

Kair is the production network entry point; the embedded Go `Engine` does not use the network. This page is a **navigation map** of the current protocol, not a substitute for its implementation or a complete rewrite of the former field-by-field RFCs.

| Layer | Current source |
| --- | --- |
| v1 framing and client | `kairnet/codec/datapack.go`, `proto/codes.go`, `client/` |
| v2 framing, negotiation, dispatch | `kairnet/codec/v2.go`, `kairnet/negotiate/`, `service/router_v2.go` |
| Temporal request/response payloads | `proto/temporal_frame.go`, `service/temporal_store.go` |
| Task plane control payloads (hand-written TLV, opaque checkpoint/outcome blobs) | `proto/task_frame.go`, `internal/taskplane`, `service/task_store.go` |
| Fixed Go/Python cross-language byte vectors | `docs/kair/vectors.json`, `docs/kair/vectors-v2.json` |

v1 provides basic PUT/GET/DEL/SCAN; v2 includes versioned writes, as-of reads, version listing, and auditing. `TASK_CLAIM/CHECKPOINT/RENEW/COMPLETE/FAIL` coordinate tasks through versioned state (lease expiry is judged at read time; no scheduler runs inside the database). Plain PUT overwrites a literal key and is not the same as immutable PUT_VERSIONED. Network agent-source checks primarily apply to v2 `PUT_VERSIONED`; v1 PUT/DEL do not carry the same source-role field, and in-process Go APIs do not traverse network role checks. Do not describe all agent writes as protected by an identical authorization policy.

Check both the actual codecs and fixed test vectors before changing wire formats; this page alone is insufficient to implement a new client. The [historical Kair 2 RFC](../rfc/Kair-2.md) and [bannet restructuring notes](../rfc/bannet-重构.md) remain available as historical rationale, not as substitutes for current code.
