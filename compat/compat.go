// Package compat provides a drop-in replacement for lumberjack.Logger,
// allowing migration from gopkg.in/natefinch/lumberjack.v2 by changing
// only the import path.
//
// Usage:
//
//	// Before:
//	// import "gopkg.in/natefinch/lumberjack.v2"
//	// After:
//	import "github.com/agentine/sawmill/compat"
//
//	logger := &compat.Logger{
//	    Filename:   "/var/log/myapp/server.log",
//	    MaxSize:    500, // megabytes
//	    MaxBackups: 3,
//	    MaxAge:     28, // days
//	    Compress:   true,
//	}
package compat

import "github.com/agentine/sawmill"

// Logger provides lumberjack.Logger API compatibility. It wraps
// sawmill.Logger with the same field names and method signatures.
//
// All fields work identically to lumberjack.Logger:
//   - Filename: log file path (defaults to <processname>-lumberjack.log in os.TempDir())
//   - MaxSize: maximum size in megabytes before rotation (default 100)
//   - MaxBackups: maximum number of old log files to retain (0 = retain all)
//   - MaxAge: maximum days to retain old log files (0 = no age limit)
//   - Compress: compress rotated files with gzip (default false)
//   - LocalTime: use local time for timestamps (default UTC)
type Logger struct {
	// Filename is the file to write logs to.
	Filename string

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int

	// MaxBackups is the maximum number of old log files to retain.
	MaxBackups int

	// MaxAge is the maximum number of days to retain old log files.
	MaxAge int

	// Compress determines if the rotated log files should be compressed.
	Compress bool

	// LocalTime determines if the time used for formatting the timestamps in
	// backup files is the computer's local time.
	LocalTime bool

	inner *sawmill.Logger
}

func (l *Logger) logger() *sawmill.Logger {
	if l.inner == nil {
		l.inner = &sawmill.Logger{
			Filename:   l.Filename,
			MaxSize:    l.MaxSize,
			MaxBackups: l.MaxBackups,
			MaxAge:     l.MaxAge,
			Compress:   l.Compress,
			LocalTime:  l.LocalTime,
		}
	}
	return l.inner
}

// Write implements io.Writer.
func (l *Logger) Write(p []byte) (n int, err error) {
	return l.logger().Write(p)
}

// Close implements io.Closer.
func (l *Logger) Close() error {
	if l.inner == nil {
		return nil
	}
	return l.inner.Close()
}

// Rotate causes the logger to close the existing log file and create a new one.
func (l *Logger) Rotate() error {
	return l.logger().Rotate()
}
