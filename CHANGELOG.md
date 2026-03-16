# Changelog

## v0.1.0 — 2026-03-16

Initial release. Drop-in replacement for `natefinch/lumberjack` with additional features.

### Features

- **Core Logger** — `io.WriteCloser` with size-based log file rotation, field-compatible with lumberjack
- **Time-based rotation** — `RotateEvery` (duration) and `RotateAt` (midnight/hourly) with background ticker
- **Gzip compression** — Background compression of rotated files with atomic temp+rename
- **SIGHUP signal handling** — Unix signal-triggered rotation for logrotate integration (no-op on Windows)
- **Compat package** — `github.com/agentine/sawmill/compat` for import-path-only migration from lumberjack
- **Thread-safe** — All operations protected by `sync.Mutex`
- **Zero dependencies** — Standard library only
- **90.1% test coverage** with 4 benchmarks (Write, WriteParallel, Rotation, Compression)
- **Migration guide** — Step-by-step migration from lumberjack in `MIGRATION.md`
- **CI** — Tested on Go 1.22–1.24 with golangci-lint
