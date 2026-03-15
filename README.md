# sawmill

[![CI](https://github.com/agentine/sawmill/actions/workflows/ci.yml/badge.svg)](https://github.com/agentine/sawmill/actions/workflows/ci.yml)

Drop-in replacement for [natefinish/lumberjack](https://github.com/natefinish/lumberjack). Rolling log file writer for Go with size-based and time-based rotation, compression, and signal handling.

## Why sawmill?

lumberjack is effectively unmaintained (last release Feb 2023, 64 open issues, 37 unmerged PRs). sawmill provides full API compatibility while fixing known bugs and adding the most-requested features:

- **Time-based rotation** — rotate hourly, daily, or on any duration (the #1 requested lumberjack feature since 2015)
- **Clock-aligned schedules** — rotate at midnight, top of hour
- **SIGHUP signal handling** — for logrotate integration
- **Fixed goroutine and fd leaks** — proper Close() lifecycle
- **Fixed compression race conditions** — atomic temp file + rename
- **Zero dependencies** — only Go stdlib

## Installation

```bash
go get github.com/agentine/sawmill
```

## Usage

### stdlib log

```go
log.SetOutput(&sawmill.Logger{
    Filename:   "/var/log/myapp/server.log",
    MaxSize:    500, // megabytes
    MaxBackups: 3,
    MaxAge:     28, // days
    Compress:   true,
})
```

### slog

```go
w := &sawmill.Logger{
    Filename: "/var/log/myapp/server.log",
    MaxSize:  100,
}
defer w.Close()

logger := slog.New(slog.NewJSONHandler(w, nil))
slog.SetDefault(logger)
```

### zap

```go
w := &sawmill.Logger{
    Filename: "/var/log/myapp/server.log",
    MaxSize:  100,
    Compress: true,
}

core := zapcore.NewCore(
    zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
    zapcore.AddSync(w),
    zap.InfoLevel,
)
logger := zap.New(core)
```

### zerolog

```go
w := &sawmill.Logger{
    Filename: "/var/log/myapp/server.log",
    MaxSize:  100,
    Compress: true,
}

logger := zerolog.New(w).With().Timestamp().Logger()
```

### Time-based rotation

```go
w := &sawmill.Logger{
    Filename:    "/var/log/myapp/server.log",
    RotateEvery: 24 * time.Hour, // rotate daily
    MaxBackups:  7,
    Compress:    true,
}
```

### Clock-aligned rotation

```go
w := &sawmill.Logger{
    Filename:   "/var/log/myapp/server.log",
    RotateAt:   "midnight", // or "hourly"
    MaxBackups: 30,
    Compress:   true,
}
```

### SIGHUP handling

```go
w := &sawmill.Logger{
    Filename: "/var/log/myapp/server.log",
}
w.EnableSignalHandling() // rotates on SIGHUP
defer w.Close()
```

## Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| Filename | string | `<process>-lumberjack.log` in temp dir | Log file path |
| MaxSize | int | 100 | Max size in MB before rotation |
| MaxBackups | int | 0 (keep all) | Max old log files to retain |
| MaxAge | int | 0 (no limit) | Max days to retain old files |
| Compress | bool | false | Compress rotated files with gzip |
| LocalTime | bool | false | Use local time for timestamps |
| RotateEvery | time.Duration | 0 (disabled) | Duration-based rotation |
| RotateAt | string | "" (disabled) | Clock-aligned rotation ("midnight", "hourly") |

## License

MIT
