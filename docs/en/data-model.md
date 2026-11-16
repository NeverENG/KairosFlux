# Data and time semantics

[简体中文](../zh-CN/data-model.md) · [Documentation index](README.md)

## Quotes and research objects

- The existing daily-quote example and `contracts/quote.schema.json` use `quote:<YYYY-MM-DD>:<symbol>`. The payload includes `code`, `date`, OHLC, `volume`, and other fields. The sample ingester uses ordinary `PUT` against a literal key; it does **not** automatically append a temporal version on every write.
- The temporal path uses `PUT_VERSIONED` to append immutable versions of a logical key. Storage writes monotonically numbered `:v...` keys and a `:current` pointer; `GET_AS_OF`, `LIST_VERSIONS`, `LIST_WRITES`, and replay fingerprints provide historical reads, auditing, and state verification. Plain quote `PUT` and versioned research writes have distinct semantics.
- Agent `Proposal` covers factors, hypotheses, experiments, recommendations, and reviews; the Go `SubmitProposal` method is its constrained entry point. `BuildContext` gathers existing experiments and strategy states plus on-disk contracts/risk files. Unified schemas for the full research process, trade handoff, and recommendation records are **not yet defined**. Proposed lifecycle stages must not be presented as shipped contracts.

## Two kinds of time

Market/event time (`date`, possibly a trade timestamp in the payload) describes when an observation occurred. A version's `write_ts` describes **when the system knew about it**. `GET_AS_OF(key, t)` selects by write time and helps exclude later corrections from historical research; it is not an existing “fetch N days for symbol” API. Ledger reads in `BuildContext` observe as-of semantics, but on-disk `contracts/` and `riskredlines/` files do not have the same historical version guarantee.

## Single-symbol N-day reads: target, not shipped behavior

Date-first lexical ordering groups a full trading day, not multiple days for one symbol. Define the intended query's symbol, market-time bounds, ordering, page/limit contract, missing days, and corrected data handling before measuring scans, latency, and memory on real samples. The current server business `SCAN` returns at most 10,000 entries and can truncate; it also hides temporal internal version keys. It is not an unrestricted history export.

A symbol-first layout such as `quote:<symbol>:<date>` is **not yet implemented**. If adopted, choose daily versus intraday timestamp precision and the distributed shard boundary first; freeze writes, back up, convert, verify, and only then switch old data. There is currently no promised secondary index or migration tool.
