# Operations, backup, and verification

[简体中文](../zh-CN/operations.md) · [Documentation index](README.md)

## Local startup

Go 1.26.1+ is required. Build from the repository root, then start the server from `cmd/kairosflux-server` so relative data paths in the current `config/config.json` resolve as intended:

```sh
go build ./...
cd cmd/kairosflux-server
go run .
```

In a separate terminal at the repository root, run `python3 client/python/examples/write_quote.py --help` for quote ingestion arguments. That sample uses plain PUT for daily quotes; it is not an agent Proposal example or a single-symbol N-day query test.

Check actual configuration and working directory before production use: `WALPath` and `SSTablePath` in `config/config.json` are relative paths, so changing the startup directory changes where data is stored. Standalone is the default; the example config listens on `127.0.0.1:8080`. Do not assume another client or server default applies. The Go `NewEmbedded` path uses `Options.DataDir` explicitly and has no network listener; check it with `go run ./cmd/kairosflux-sample-embedded -data-dir /tmp/kairosflux-demo` from the root.

## Backup and restore

KairosFlux is not intended to be a “discard after delivery” buffer in the target architecture. **Stop writes and shut down the server** before copying; identify and copy both the actual WAL file and SSTable directory to a separate location. Record the resolved paths, config, and backup time. Do not copy only SSTables or treat an unverified live copy as a consistent snapshot. Restore into an isolated directory, verify representative quotes, version history, and audit queries, then consider switching the production directory.

The current optional `RetentionEnabled` means “reclaim whole SSTable files after the delivery cursor commits,” **not** “expire records by market date or research retention period.” Before removing delivery/reclamation in code, check whether it was ever enabled in the actual runtime configuration; code deletion cannot recover previously reclaimed history.

## Verification and limits

```sh
go test ./...
python3 -m unittest discover -s client/python -p 'test_*.py'
```

Go and Python protocol tests read `docs/kair/vectors.json` and `docs/kair/vectors-v2.json`; do not remove or alter them in a prose-docs reset. `go test -race ./...` provides an additional concurrency check. Server business `SCAN` is local-shard only and defaults to a 10,000-entry result cap. Neither cross-shard nor a complete single-symbol series API is a verified current capability.
