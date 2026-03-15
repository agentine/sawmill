package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestLogger(t *testing.T) *Logger {
	t.Helper()
	dir := t.TempDir()
	return &Logger{
		Filename: filepath.Join(dir, "test.log"),
	}
}

func TestWriteAndRead(t *testing.T) {
	l := newTestLogger(t)
	defer l.Close()

	msg := "hello from compat\n"
	n, err := l.Write([]byte(msg))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(msg) {
		t.Fatalf("Write returned %d, want %d", n, len(msg))
	}

	data, err := os.ReadFile(l.Filename)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != msg {
		t.Fatalf("content = %q, want %q", data, msg)
	}
}

func TestRotate(t *testing.T) {
	l := newTestLogger(t)
	defer l.Close()

	l.Write([]byte("before\n"))
	if err := l.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	l.Write([]byte("after\n"))

	data, _ := os.ReadFile(l.Filename)
	if string(data) != "after\n" {
		t.Fatalf("expected only post-rotation content, got %q", data)
	}

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	if len(entries) < 2 {
		t.Fatalf("expected backup file, got %d files", len(entries))
	}
}

func TestClose(t *testing.T) {
	l := newTestLogger(t)
	l.Write([]byte("data\n"))
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Double close should be fine.
	if err := l.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestMaxSizeRotation(t *testing.T) {
	l := newTestLogger(t)
	l.MaxSize = 1 // 1 MB
	defer l.Close()

	msg := strings.Repeat("x", 1024) + "\n"
	for i := 0; i < 1100; i++ {
		if _, err := l.Write([]byte(msg)); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	if len(entries) < 2 {
		t.Fatalf("expected rotation, got %d files", len(entries))
	}
}

func TestCompat_FieldsPropagated(t *testing.T) {
	l := newTestLogger(t)
	l.MaxSize = 50
	l.MaxBackups = 3
	l.MaxAge = 7
	l.Compress = true
	l.LocalTime = true
	defer l.Close()

	l.Write([]byte("test\n"))

	inner := l.logger()
	if inner.MaxSize != 50 {
		t.Fatalf("MaxSize not propagated: %d", inner.MaxSize)
	}
	if inner.MaxBackups != 3 {
		t.Fatalf("MaxBackups not propagated: %d", inner.MaxBackups)
	}
	if inner.MaxAge != 7 {
		t.Fatalf("MaxAge not propagated: %d", inner.MaxAge)
	}
	if !inner.Compress {
		t.Fatal("Compress not propagated")
	}
	if !inner.LocalTime {
		t.Fatal("LocalTime not propagated")
	}
}
