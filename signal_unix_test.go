//go:build !windows

package sawmill

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSIGHUPTriggersRotation(t *testing.T) {
	l := newTestLogger(t)
	defer l.Close()

	l.EnableSignalHandling()
	l.Write([]byte("before sighup\n"))

	// Send SIGHUP to self.
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := proc.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("Signal: %v", err)
	}

	// Give the signal handler time to process.
	time.Sleep(100 * time.Millisecond)

	l.Write([]byte("after sighup\n"))

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	if len(entries) < 2 {
		t.Fatalf("expected rotation after SIGHUP, got %d files", len(entries))
	}

	data, _ := os.ReadFile(l.Filename)
	if string(data) != "after sighup\n" {
		t.Fatalf("expected only post-sighup content, got %q", data)
	}
}

func TestEnableSignalHandlingIdempotent(t *testing.T) {
	l := newTestLogger(t)
	defer l.Close()

	// Calling EnableSignalHandling multiple times should not panic.
	l.EnableSignalHandling()
	l.EnableSignalHandling()
	l.EnableSignalHandling()

	l.Write([]byte("ok\n"))
}

func TestCloseStopsSignalHandler(t *testing.T) {
	l := newTestLogger(t)
	l.EnableSignalHandling()
	l.Write([]byte("data\n"))

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Double close should be fine.
	if err := l.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
