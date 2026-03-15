package sawmill

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockClock is a controllable clock for testing time-based rotation.
type mockClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMockClock(t time.Time) *mockClock {
	return &mockClock{now: t}
}

func (m *mockClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

func (m *mockClock) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = t
}

func (m *mockClock) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}

func (m *mockClock) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

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

func TestRotateEvery(t *testing.T) {
	clk := newMockClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	l := newTestLogger(t)
	l.RotateEvery = time.Hour
	l.clock = clk
	defer l.Close()

	// First write — no rotation yet.
	l.Write([]byte("msg1\n"))

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file before rotation, got %d", len(entries))
	}

	// Advance past RotateEvery, next write should trigger rotation.
	clk.Advance(61 * time.Minute)
	l.Write([]byte("msg2\n"))

	entries, _ = os.ReadDir(dir)
	if len(entries) < 2 {
		t.Fatalf("expected rotation after RotateEvery, got %d files", len(entries))
	}

	// Current file should only have msg2.
	data, _ := os.ReadFile(l.Filename)
	if string(data) != "msg2\n" {
		t.Fatalf("expected only post-rotation content, got %q", data)
	}
}

func TestRotateAtMidnight(t *testing.T) {
	// Start at 23:59.
	clk := newMockClock(time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC))
	l := newTestLogger(t)
	l.RotateAt = "midnight"
	l.clock = clk
	defer l.Close()

	l.Write([]byte("before midnight\n"))

	// Advance to 00:01 next day.
	clk.Set(time.Date(2026, 1, 2, 0, 1, 0, 0, time.UTC))
	l.Write([]byte("after midnight\n"))

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	if len(entries) < 2 {
		t.Fatalf("expected rotation at midnight, got %d files", len(entries))
	}

	data, _ := os.ReadFile(l.Filename)
	if string(data) != "after midnight\n" {
		t.Fatalf("expected only post-midnight content, got %q", data)
	}
}

func TestRotateAtHourly(t *testing.T) {
	clk := newMockClock(time.Date(2026, 1, 1, 10, 55, 0, 0, time.UTC))
	l := newTestLogger(t)
	l.RotateAt = "hourly"
	l.clock = clk
	defer l.Close()

	l.Write([]byte("before hour\n"))

	// Advance to next hour.
	clk.Set(time.Date(2026, 1, 1, 11, 0, 1, 0, time.UTC))
	l.Write([]byte("after hour\n"))

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	if len(entries) < 2 {
		t.Fatalf("expected rotation at hour boundary, got %d files", len(entries))
	}
}

func TestNoTimeRotationWhenNotConfigured(t *testing.T) {
	l := newTestLogger(t)
	defer l.Close()

	// No RotateEvery or RotateAt — should behave like lumberjack.
	for i := 0; i < 100; i++ {
		l.Write([]byte("data\n"))
	}

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected no rotation without time config, got %d files", len(entries))
	}
}

func TestRotateEveryAndSizeCombined(t *testing.T) {
	clk := newMockClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	l := newTestLogger(t)
	l.MaxSize = 1 // 1 MB
	l.RotateEvery = time.Hour
	l.clock = clk
	defer l.Close()

	// First, trigger a size-based rotation.
	msg := strings.Repeat("x", 512*1024) + "\n"
	l.Write([]byte(msg))
	l.Write([]byte(msg))
	l.Write([]byte(msg)) // should trigger size rotation

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	if len(entries) < 2 {
		t.Fatalf("expected size-based rotation, got %d files", len(entries))
	}

	// Now trigger a time-based rotation.
	clk.Advance(61 * time.Minute)
	l.Write([]byte("after time\n"))

	entries, _ = os.ReadDir(dir)
	if len(entries) < 3 {
		t.Fatalf("expected both size and time rotations, got %d files", len(entries))
	}
}

func TestCompressRotatedFiles(t *testing.T) {
	l := newTestLogger(t)
	l.Compress = true
	defer l.Close()

	l.Write([]byte("before rotation\n"))
	l.Rotate()
	l.Write([]byte("after rotation\n"))
	l.Close() // waits for compression

	dir := filepath.Dir(l.Filename)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	var hasGz bool
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".gz") {
			hasGz = true
			// Verify it's valid gzip.
			f, err := os.Open(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("open gz file: %v", err)
			}
			gr, err := gzip.NewReader(f)
			if err != nil {
				f.Close()
				t.Fatalf("gzip.NewReader: %v", err)
			}
			data, err := io.ReadAll(gr)
			if err != nil {
				gr.Close()
				f.Close()
				t.Fatalf("read gzip: %v", err)
			}
			gr.Close()
			f.Close()
			if string(data) != "before rotation\n" {
				t.Fatalf("unexpected compressed content: %q", data)
			}
		}
	}
	if !hasGz {
		t.Fatal("expected a .gz backup file")
	}
}

func TestCompressRemovesOriginal(t *testing.T) {
	l := newTestLogger(t)
	l.Compress = true
	defer l.Close()

	l.Write([]byte("data\n"))
	l.Rotate()
	l.Close()

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if name == filepath.Base(l.Filename) {
			continue
		}
		// Backup files should only be .gz, not uncompressed.
		if !strings.HasSuffix(name, ".gz") {
			t.Fatalf("expected only .gz backups, found: %s", name)
		}
	}
}

func TestNoCompressWhenDisabled(t *testing.T) {
	l := newTestLogger(t)
	l.Compress = false
	defer l.Close()

	l.Write([]byte("data\n"))
	l.Rotate()
	l.Close()

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".gz") {
			t.Fatal("did not expect .gz files when Compress=false")
		}
	}
}

func TestCompressMultipleRotations(t *testing.T) {
	l := newTestLogger(t)
	l.Compress = true
	defer l.Close()

	for i := 0; i < 5; i++ {
		l.Write([]byte("data\n"))
		l.Rotate()
		time.Sleep(2 * time.Millisecond) // ensure unique timestamps
	}
	l.Close()

	dir := filepath.Dir(l.Filename)
	entries, _ := os.ReadDir(dir)
	gzCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".gz") {
			gzCount++
		}
	}
	if gzCount < 5 {
		t.Fatalf("expected at least 5 .gz files, got %d", gzCount)
	}
}
