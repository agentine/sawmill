//go:build windows

package sawmill

import "os"

// EnableSignalHandling is a no-op on Windows. SIGHUP is not available on
// Windows, so signal-based rotation is not supported.
func (l *Logger) EnableSignalHandling() {}

// stopSignalHandler is a no-op on Windows.
func (l *Logger) stopSignalHandler() {}
