# Migrating from lumberjack to sawmill

## Quick migration (import path only)

Use the `compat` package for a zero-change migration:

```diff
- import "gopkg.in/natefinish/lumberjack.v2"
+ import "github.com/agentine/sawmill/compat"

  logger := &compat.Logger{
      Filename:   "/var/log/myapp/server.log",
      MaxSize:    500,
      MaxBackups: 3,
      MaxAge:     28,
      Compress:   true,
  }
```

All fields and methods work identically. No code changes needed.

## Full migration (recommended)

Switch to the `sawmill` package directly to access new features:

```diff
- import "gopkg.in/natefinish/lumberjack.v2"
+ import "github.com/agentine/sawmill"

- logger := &lumberjack.Logger{
+ logger := &sawmill.Logger{
      Filename:   "/var/log/myapp/server.log",
      MaxSize:    500,
      MaxBackups: 3,
      MaxAge:     28,
      Compress:   true,
  }
```

## New features available after migration

### Time-based rotation

```go
logger := &sawmill.Logger{
    Filename:    "/var/log/myapp/server.log",
    RotateEvery: 24 * time.Hour,
}
```

### Clock-aligned rotation

```go
logger := &sawmill.Logger{
    Filename: "/var/log/myapp/server.log",
    RotateAt: "midnight", // or "hourly"
}
```

### SIGHUP signal handling

```go
logger := &sawmill.Logger{
    Filename: "/var/log/myapp/server.log",
}
logger.EnableSignalHandling()
defer logger.Close()
```

## What's fixed

- Goroutine leaks: `Close()` properly stops all background goroutines
- File descriptor leaks: all opened files are tracked and closed
- Compression race conditions: uses atomic temp file + rename
- Proper lifecycle management via `Close()`
