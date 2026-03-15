package sawmill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestLogger creates a Logger pointing at a temp directory.
func newTestLogger(t *testing.T) *Logger {
	t.Helper()
	dir := t.TempDir()
	return &Logger{
		Filename: filepath.Join(dir, "test.log"),
	}
}

func TestNewLogger(t *testing.T) {
	l := newTestLogger(t)
	defer l.Close()

	_, err := l.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, err := os.ReadFile(l.Filename)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestWriteAppendsToExisting(t *testing.T) {
	l := newTestLogger(t)
	defer l.Close()

	l.Write([]byte("first\n"))
	l.Close()

	// Reopen by writing again.
	l.Write([]byte("second\n"))
	l.Close()

	data, err := os.ReadFile(l.Filename)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "first\nsecond\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestMaxSizeRotation(t *testing.T) {
	l := newTestLogger(t)
	l.MaxSize = 1 // 1 MB
	defer l.Close()

	// Write enough to trigger rotation (>1MB total).
	msg := strings.Repeat("x", 1024) + "\n"
	for i := 0; i < 1100; i++ {
		if _, err := l.Write([]byte(msg)); err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	// Should have rotated at least once.
	dir := filepath.Dir(l.Filename)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 files (current + backup), got %d", len(entries))
	}
}

func TestWriteExceedingMaxSize(t *testing.T) {
	l := newTestLogger(t)
	l.MaxSize = 1 // 1 MB
	defer l.Close()

	// A single write larger than MaxSize should fail.
	msg := strings.Repeat("x", int(l.max())+1)
	_, err := l.Write([]byte(msg))
	if err == nil {
		t.Fatal("expected error for oversized write")
	}
}

func TestRotate(t *testing.T) {
	l := newTestLogger(t)
	defer l.Close()

	l.Write([]byte("before rotation\n"))
	if err := l.Rotate(); err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}
	l.Write([]byte("after rotation\n"))

	// Current file should only have post-rotation content.
	data, err := os.ReadFile(l.Filename)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "after rotation\n" {
		t.Fatalf("unexpected content: %q", data)
	}

	// Should have a backup file.
	dir := filepath.Dir(l.Filename)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 files, got %d", len(entries))
	}
}

func TestMaxBackups(t *testing.T) {
	l := newTestLogger(t)
	l.MaxSize = 1 // 1 MB
	l.MaxBackups = 2
	defer l.Close()

	// Create several rotations.
	msg := strings.Repeat("x", 512*1024) + "\n" // ~512KB
	for i := 0; i < 10; i++ {
		l.Write([]byte(msg))
		l.Write([]byte(msg)) // triggers rotation
		time.Sleep(2 * time.Millisecond)
	}

	files, err := l.oldLogFiles()
	if err != nil {
		t.Fatalf("oldLogFiles failed: %v", err)
	}
	// After cleanup we should have at most MaxBackups backup files.
	if len(files) > l.MaxBackups {
		t.Fatalf("expected at most %d backups, got %d", l.MaxBackups, len(files))
	}
}

func TestMaxAge(t *testing.T) {
	l := newTestLogger(t)
	defer l.Close()

	// Create a fake old backup.
	dir := filepath.Dir(l.Filename)
	ext := filepath.Ext(filepath.Base(l.Filename))
	prefix := strings.TrimSuffix(filepath.Base(l.Filename), ext)
	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	oldName := prefix + "-" + oldTime.Format(backupTimeFormat) + ext
	os.WriteFile(filepath.Join(dir, oldName), []byte("old"), 0644)

	l.MaxAge = 1 // 1 day
	l.Write([]byte("trigger\n"))
	l.Rotate()

	// The old backup should have been cleaned up.
	if _, err := os.Stat(filepath.Join(dir, oldName)); !os.IsNotExist(err) {
		t.Fatal("expected old backup to be deleted")
	}
}

func TestClose(t *testing.T) {
	l := newTestLogger(t)
	l.Write([]byte("data\n"))
	if err := l.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	// Double close should be fine.
	if err := l.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{
		Filename: filepath.Join(dir, "sub", "deep", "test.log"),
	}
	defer l.Close()

	_, err := l.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("Write failed (should create dirs): %v", err)
	}
}

func TestDefaultFilename(t *testing.T) {
	l := &Logger{}
	name := l.filename()
	if name == "" {
		t.Fatal("expected non-empty default filename")
	}
	if !strings.Contains(name, "lumberjack.log") {
		t.Fatalf("expected default filename to contain lumberjack.log, got %s", name)
	}
}

func TestDefaultMaxSize(t *testing.T) {
	l := &Logger{}
	if l.max() != int64(DefaultMaxSize)*megabyte {
		t.Fatalf("expected default max %d, got %d", int64(DefaultMaxSize)*megabyte, l.max())
	}
}

func TestBackupNameFormat(t *testing.T) {
	l := &Logger{}
	name := l.backupName("/var/log/server.log", false)
	if !strings.HasPrefix(name, "/var/log/server-") {
		t.Fatalf("unexpected backup name: %s", name)
	}
	if !strings.HasSuffix(name, ".log") {
		t.Fatalf("unexpected backup name suffix: %s", name)
	}
}

func TestLocalTime(t *testing.T) {
	l := newTestLogger(t)
	l.LocalTime = true
	defer l.Close()

	l.Write([]byte("data\n"))
	l.Rotate()

	files, err := l.oldLogFiles()
	if err != nil {
		t.Fatalf("oldLogFiles failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one backup")
	}
}
