# Sawmill — Drop-in Replacement for natefinch/lumberjack

## Overview

**Replaces:** [natefinch/lumberjack](https://github.com/natefinch/lumberjack) (5.4k stars, 8,936 importers, effectively unmaintained)
**Also replaces:** [lestrrat-go/file-rotatelogs](https://github.com/lestrrat-go/file-rotatelogs) (archived, "DO NOT USE")
**Package:** `github.com/agentine/sawmill`
**Language:** Go (minimum Go 1.22)

lumberjack is the dominant log rotation library in Go, used by virtually every major logging framework (zap, zerolog, slog, logrus). Its last release was v2.2.1 in February 2023 — over 3 years ago. It has 64 open issues and 37 unmerged PRs with no maintainer activity. Users are openly asking "is this maintained?" (issue #227). Known bugs include goroutine leaks, file descriptor leaks, and gzip compression failures.

The other major Go log rotation library, lestrrat-go/file-rotatelogs, was archived in July 2021 with an explicit "DO NOT USE" notice.

Sawmill provides **lumberjack API compatibility** so existing projects can switch with a single import path change, while adding the most-requested features (time-based rotation, better concurrency).

## Why Replace

- **No maintenance:** 3+ years since last release, no security patches, no PR reviews
- **64 open issues:** Includes goroutine leaks, file descriptor leaks, compression failures
- **37 unmerged PRs:** Active contributors but no maintainer to merge
- **8,936 importers:** Critical infrastructure dependency for Go logging
- **Missing time-based rotation:** Most-requested feature since 2015 (issues #17, #54), never implemented
- **No well-adopted replacement:** Timberjack fork has only 121 stars (created April 2025)
- **file-rotatelogs also dead:** The alternative for time-based rotation is archived

## Architecture

### Core Components

1. **Logger** — Rolling log file writer implementing `io.WriteCloser`
   - Size-based rotation (MaxSize in MB, lumberjack-compatible)
   - Time-based rotation (RotateEvery duration — hourly, daily, etc.)
   - Clock-aligned rotation (RotateAt for exact schedule marks)
   - Maximum backup retention by count (MaxBackups) and age (MaxAge)
   - Optional gzip/zstd compression of rotated files
   - Atomic file operations to prevent corruption
   - Graceful handling of disk-full conditions

2. **Rotation Engine** — Manages file lifecycle
   - Size check on every write (existing lumberjack behavior)
   - Time-based check via background ticker (new)
   - Signal-based re-opening (SIGHUP support for logrotate integration)
   - Cleanup of old files by age and count
   - Concurrent-safe rotation (fixes known lumberjack race conditions)

3. **Compression** — Background compression of rotated files
   - Gzip compression (lumberjack-compatible)
   - Zstd compression (new, faster and better ratio)
   - Non-blocking: compression runs in background goroutine
   - Proper cleanup on context cancellation

4. **Compatibility Layer** — `github.com/agentine/sawmill/compat`
   - Type alias for `lumberjack.Logger` configuration struct
   - Same field names: Filename, MaxSize, MaxBackups, MaxAge, Compress, LocalTime
   - Enables migration via import path replacement only

### Key Design Decisions

- **`io.WriteCloser` interface:** Same as lumberjack, plugs into any logger (zap, zerolog, slog, stdlib log)
- **lumberjack field compatibility:** All lumberjack.Logger fields work identically
- **New fields are additive:** Time-based rotation fields are optional; omitting them gives lumberjack behavior
- **Zero dependencies:** No external dependencies beyond Go stdlib
- **Concrete `Logger` struct:** Same pattern as lumberjack (not an interface)
- **`sync.Mutex` for writes:** Thread-safe Write/Rotate/Close (fixes lumberjack race conditions)
- **Background goroutine lifecycle:** Proper cleanup via `Close()` — no goroutine leaks

## Deliverables

1. Core Logger with size-based rotation (lumberjack parity)
2. Time-based rotation (RotateEvery, RotateAt)
3. Gzip and zstd compression
4. Signal handling (SIGHUP re-open)
5. Compatibility package for drop-in migration from lumberjack
6. Comprehensive test suite (>90% coverage)
7. Benchmark suite comparing against lumberjack
8. Migration guide documentation

## Improvements Over lumberjack

- Active maintenance and security patches
- Time-based rotation (hourly, daily, weekly — the #1 requested feature)
- Clock-aligned rotation schedules
- Zstd compression option (faster, better ratio than gzip)
- SIGHUP signal handling for logrotate integration
- Fixed goroutine and file descriptor leaks
- Fixed gzip compression race conditions
- Proper `Close()` lifecycle management
- Atomic file operations to prevent corruption
- Better error messages and error wrapping
- Go 1.22+ minimum
- Zero dependencies
