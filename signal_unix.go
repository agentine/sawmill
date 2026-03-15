//go:build !windows

package sawmill

import (
	"os"
	"os/signal"
	"syscall"
)

// EnableSignalHandling starts a goroutine that listens for SIGHUP and triggers
// log rotation. This is useful for logrotate integration. The goroutine is
// stopped when Close() is called.
//
// Call this after creating the Logger. It is safe to call multiple times;
// subsequent calls are no-ops if signal handling is already enabled.
func (l *Logger) EnableSignalHandling() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.sigRunning {
		return
	}

	l.sigCh = make(chan os.Signal, 1)
	signal.Notify(l.sigCh, syscall.SIGHUP)
	l.sigRunning = true

	go func() {
		for range l.sigCh {
			_ = l.Rotate()
		}
	}()
}

// stopSignalHandler stops the signal handling goroutine.
// The caller must hold l.mu.
func (l *Logger) stopSignalHandler() {
	if !l.sigRunning {
		return
	}
	signal.Stop(l.sigCh)
	close(l.sigCh)
	l.sigRunning = false
}
