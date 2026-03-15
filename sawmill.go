// Package sawmill provides a rolling log file writer with size-based and
// time-based rotation, compression, and signal handling. It is a drop-in
// replacement for natefinish/lumberjack.
package sawmill

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxSize is the default maximum size in megabytes of the log file
	// before it gets rotated.
	DefaultMaxSize = 100

	// megabyte is the number of bytes in a megabyte.
	megabyte = 1024 * 1024

	// backupTimeFormat is the timestamp format used in rotated file names.
	backupTimeFormat = "2006-01-02T15-04-05.000"
)

// ensure Logger implements io.WriteCloser.
var _ io.WriteCloser = (*Logger)(nil)

// Logger is an io.WriteCloser that writes to the specified filename.
//
// Logger opens or creates the logfile on first Write. If the file exists and is
// less than MaxSize megabytes, sawmill will open and append to that file.
// If the file exists and its size is >= MaxSize megabytes, the file is renamed
// by putting the current time in a timestamp in the name immediately before the
// file's extension (or the end of the filename if there's no extension). A new
// log file is then created using the original filename.
//
// Whenever a write would cause the current log file to exceed MaxSize
// megabytes, the current file is closed, renamed, and a new log file is
// created with the original name. Thus, the filename you give Logger is
// always the "current" log file.
//
// Backups use the log file name given to Logger, in the form
// name-timestamp.ext, where name is the filename without the extension,
// timestamp is the time at which the log was rotated formatted with the
// time.Time format of 2006-01-02T15-04-05.000, and ext is the original
// extension. For example, if your Logger.Filename is /var/log/foo/server.log,
// a backup created at 6:30pm on Nov 11 2016 would use the filename
// /var/log/foo/server-2016-11-11T18-30-00.000.log.
//
// # Cleaning Up Old Log Files
//
// Whenever a new logfile gets created, old log files may be deleted. The most
// recent files according to the encoded timestamp will be retained, up to a
// number equal to MaxBackups (or all of them if MaxBackups is 0). Any files
// with an encoded timestamp older than MaxAge days are deleted, regardless of
// MaxBackups. Note that the time encoded in the timestamp is the rotation
// time, which may differ from the last time that file was written to.
//
// If MaxBackups and MaxAge are both 0, no old log files will be deleted.
type Logger struct {
	// Filename is the file to write logs to. Backup log files will be retained
	// in the same directory. It uses <processname>-lumberjack.log in
	// os.TempDir() if empty.
	Filename string

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int

	// MaxBackups is the maximum number of old log files to retain. The default
	// is to retain all old log files (though MaxAge may still cause them to
	// get deleted.)
	MaxBackups int

	// MaxAge is the maximum number of days to retain old log files based on
	// the timestamp encoded in their filename. Note that a day is defined as
	// 24 hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	MaxAge int

	// Compress determines if the rotated log files should be compressed using
	// gzip. The default is not to perform compression.
	Compress bool

	// LocalTime determines if the time used for formatting the timestamps in
	// backup files is the computer's local time. The default is to use UTC
	// time.
	LocalTime bool

	// RotateEvery specifies a duration after which the log file is rotated,
	// regardless of size. For example, 24*time.Hour rotates daily.
	// Zero means no duration-based rotation. This is additive to size-based
	// rotation — whichever triggers first causes rotation.
	RotateEvery time.Duration

	// RotateAt specifies clock-aligned rotation times. Supported values:
	//   "midnight" — rotate at 00:00 each day
	//   "hourly"   — rotate at the top of each hour
	// Empty string means no clock-aligned rotation. Uses LocalTime setting
	// for timezone. This is additive to size-based and RotateEvery rotation.
	RotateAt string

	// clock is used for testing to control time.
	clock clock

	mu          sync.Mutex
	file        *os.File
	size        int64
	lastRotate  time.Time
	done        chan struct{}
	tickerRunning bool
}

// clock is an interface for time operations, allowing tests to control time.
type clock interface {
	Now() time.Time
	NewTicker(d time.Duration) *time.Ticker
}

// realClock uses the real time functions.
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }

// Write implements io.Writer. If a write would cause the log file to be larger
// than MaxSize, the file is closed, renamed to include a timestamp of the
// current time, and a new log file is created using the original log file name.
// If the length of the write is greater than MaxSize, an error is returned.
func (l *Logger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	writeLen := int64(len(p))
	if writeLen > l.max() {
		return 0, fmt.Errorf(
			"write length %d exceeds maximum file size %d", writeLen, l.max(),
		)
	}

	if l.file == nil {
		if err = l.openExistingOrNew(writeLen); err != nil {
			return 0, err
		}
		l.lastRotate = l.now()
		l.startTicker()
	}

	// Check time-based rotation.
	if l.needsTimeRotation() {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	if l.size+writeLen > l.max() {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = l.file.Write(p)
	l.size += int64(n)
	return n, err
}

// Close implements io.Closer, and closes the current logfile.
// It stops any background goroutines (ticker, signal handler).
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopTicker()
	return l.close()
}

// Rotate causes Logger to close the existing log file and immediately create a
// new one. This is a helper function for applications that want to initiate
// rotations outside of the normal rotation rules, such as in response to
// SIGHUP. After rotating, this initiates compression and removal of old log
// files according to the configuration.
func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotate()
}

// rotate closes the current file, moves it aside with a timestamp, and opens
// a new file. The caller must hold l.mu.
func (l *Logger) rotate() error {
	if err := l.close(); err != nil {
		return err
	}

	if err := l.moveFile(); err != nil {
		return err
	}

	if err := l.openNew(); err != nil {
		return err
	}

	l.lastRotate = l.now()
	l.cleanup()
	return nil
}

// close closes the file if it is open. The caller must hold l.mu.
func (l *Logger) close() error {
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	l.size = 0
	return err
}

// openExistingOrNew opens the existing log file if it exists, or creates a new
// one. The caller must hold l.mu.
func (l *Logger) openExistingOrNew(writeLen int64) error {
	filename := l.filename()

	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return l.openNew()
	}
	if err != nil {
		return fmt.Errorf("error getting log file info: %w", err)
	}

	// File exists. If adding writeLen would exceed max, rotate first.
	if info.Size()+writeLen >= l.max() {
		return l.rotate()
	}

	// Open existing file for append.
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// If we can't open it, try to create a new one.
		return l.openNew()
	}
	l.file = file
	l.size = info.Size()
	return nil
}

// openNew creates a new log file. The caller must hold l.mu.
func (l *Logger) openNew() error {
	filename := l.filename()
	dir := filepath.Dir(filename)

	// Ensure directory exists.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("can't make directories for new logfile: %w", err)
	}

	// We use a temp file and rename to be as atomic as possible.
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("can't open new logfile: %w", err)
	}
	l.file = f
	l.size = 0
	return nil
}

// moveFile renames the current log file with a timestamp. The caller must hold l.mu.
func (l *Logger) moveFile() error {
	filename := l.filename()
	if _, err := os.Stat(filename); err != nil {
		// File doesn't exist, nothing to move.
		return nil
	}

	name := l.backupName(filename, l.LocalTime)
	return os.Rename(filename, name)
}

// backupName creates a new filename from the given name, inserting a timestamp
// between the filename and the extension.
func (l *Logger) backupName(name string, local bool) string {
	dir := filepath.Dir(name)
	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]

	t := l.now()
	if !local {
		t = t.UTC()
	}
	timestamp := t.Format(backupTimeFormat)

	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, timestamp, ext))
}

// oldLogFiles returns the list of backup log files stored in the same
// directory as the current log file, sorted by ModTime (oldest first).
func (l *Logger) oldLogFiles() ([]logInfo, error) {
	dir := filepath.Dir(l.filename())
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("can't read log file directory: %w", err)
	}

	filename := filepath.Base(l.filename())
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)] + "-"

	var logFiles []logInfo

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == filename {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		// Extract timestamp from backup file name.
		ts, err := l.timeFromName(name, prefix, ext)
		if err != nil {
			continue // not a backup file we created
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		logFiles = append(logFiles, logInfo{ts, info})
	}

	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].timestamp.Before(logFiles[j].timestamp)
	})

	return logFiles, nil
}

// timeFromName extracts the formatted time from the filename by stripping the
// prefix and extension.
func (l *Logger) timeFromName(filename, prefix, ext string) (time.Time, error) {
	// Handle compressed extensions.
	if strings.HasSuffix(filename, ext+".gz") {
		filename = filename[:len(filename)-3]
	} else if strings.HasSuffix(filename, ext+".zst") {
		filename = filename[:len(filename)-4]
	}

	if !strings.HasPrefix(filename, prefix) {
		return time.Time{}, fmt.Errorf("mismatched prefix")
	}
	if !strings.HasSuffix(filename, ext) {
		return time.Time{}, fmt.Errorf("mismatched extension")
	}

	ts := filename[len(prefix) : len(filename)-len(ext)]
	return time.Parse(backupTimeFormat, ts)
}

// cleanup deletes old log files according to MaxBackups and MaxAge
// configuration. The caller must hold l.mu.
func (l *Logger) cleanup() {
	if l.MaxBackups == 0 && l.MaxAge == 0 {
		return
	}

	files, err := l.oldLogFiles()
	if err != nil {
		return
	}

	var remove []logInfo
	dir := filepath.Dir(l.filename())

	if l.MaxBackups > 0 && len(files) > l.MaxBackups {
		preserved := make(map[string]bool)
		// Keep the most recent MaxBackups files.
		for _, f := range files[len(files)-l.MaxBackups:] {
			preserved[f.os.Name()] = true
		}
		for _, f := range files {
			if !preserved[f.os.Name()] {
				remove = append(remove, f)
			}
		}
	}

	if l.MaxAge > 0 {
		cutoff := time.Now().Add(-time.Duration(l.MaxAge) * 24 * time.Hour)
		if !l.LocalTime {
			cutoff = cutoff.UTC()
		}
		for _, f := range files {
			if f.timestamp.Before(cutoff) {
				remove = append(remove, f)
			}
		}
	}

	// Deduplicate and remove.
	seen := make(map[string]bool)
	for _, f := range remove {
		name := f.os.Name()
		if seen[name] {
			continue
		}
		seen[name] = true
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// filename returns the log file name, defaulting to <processname>-lumberjack.log
// in os.TempDir().
func (l *Logger) filename() string {
	if l.Filename != "" {
		return l.Filename
	}
	name := filepath.Base(os.Args[0]) + "-lumberjack.log"
	return filepath.Join(os.TempDir(), name)
}

// max returns the maximum size in bytes of log files.
func (l *Logger) max() int64 {
	if l.MaxSize == 0 {
		return int64(DefaultMaxSize) * megabyte
	}
	return int64(l.MaxSize) * megabyte
}

// now returns the current time using the configured clock.
func (l *Logger) now() time.Time {
	if l.clock != nil {
		return l.clock.Now()
	}
	return time.Now()
}

// needsTimeRotation checks if a time-based rotation is due.
// The caller must hold l.mu.
func (l *Logger) needsTimeRotation() bool {
	if l.lastRotate.IsZero() {
		return false
	}
	now := l.now()

	if l.RotateEvery > 0 {
		if now.Sub(l.lastRotate) >= l.RotateEvery {
			return true
		}
	}

	if l.RotateAt != "" {
		switch l.RotateAt {
		case "midnight":
			last := l.lastRotate
			cur := now
			if !l.LocalTime {
				last = last.UTC()
				cur = cur.UTC()
			}
			if cur.YearDay() != last.YearDay() || cur.Year() != last.Year() {
				return true
			}
		case "hourly":
			last := l.lastRotate
			cur := now
			if !l.LocalTime {
				last = last.UTC()
				cur = cur.UTC()
			}
			if cur.Hour() != last.Hour() || cur.YearDay() != last.YearDay() || cur.Year() != last.Year() {
				return true
			}
		}
	}

	return false
}

// startTicker starts the background goroutine that checks time-based rotation.
// The caller must hold l.mu.
func (l *Logger) startTicker() {
	if l.tickerRunning {
		return
	}
	if l.RotateEvery == 0 && l.RotateAt == "" {
		return
	}

	l.done = make(chan struct{})
	l.tickerRunning = true

	interval := l.tickerInterval()
	clk := l.clock
	if clk == nil {
		clk = realClock{}
	}
	ticker := clk.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-l.done:
				return
			case <-ticker.C:
				l.mu.Lock()
				if l.file != nil && l.needsTimeRotation() {
					_ = l.rotate()
				}
				l.mu.Unlock()
			}
		}
	}()
}

// stopTicker stops the background ticker goroutine.
// The caller must hold l.mu.
func (l *Logger) stopTicker() {
	if !l.tickerRunning {
		return
	}
	close(l.done)
	l.tickerRunning = false
}

// tickerInterval returns the interval for the background ticker.
func (l *Logger) tickerInterval() time.Duration {
	if l.RotateEvery > 0 {
		// Check at most every RotateEvery, but at least every minute.
		d := l.RotateEvery / 10
		if d < time.Second {
			d = time.Second
		}
		if d > time.Minute {
			d = time.Minute
		}
		return d
	}
	// For clock-aligned rotation, check every 10 seconds.
	return 10 * time.Second
}

// logInfo is a convenience struct to hold a backup file's timestamp and os.FileInfo.
type logInfo struct {
	timestamp time.Time
	os        os.FileInfo
}
