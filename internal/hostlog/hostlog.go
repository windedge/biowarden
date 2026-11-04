// Package hostlog appends a diagnostic log to one file under the biowarden
// profile directory (internal/paths) and rotates it at 5 MiB. The host and the GUI keep
// their own file (host.log, gui.log): Windows allows neither process to rename
// a file the other holds open, so one shared file would stop rotating for both.
// Redaction happens here, before bytes reach the file, so a call site that logs
// a reply cannot persist key material by mistake.
package hostlog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/windedge/biowarden/internal/paths"
)

const (
	rotatedSuffix = ".1"
	defaultMax    = 5 << 20
)

// Level controls which lines are written. Info is the default; Debug also
// records full frames (already redacted), which is what makes a bug report
// useful.
type Level int

const (
	LevelInfo Level = iota
	LevelDebug
)

// ParseLevel maps BIOWARDEN_LOG to a level; anything but "debug" is info.
func ParseLevel(value string) Level {
	if strings.EqualFold(strings.TrimSpace(value), "debug") {
		return LevelDebug
	}
	return LevelInfo
}

// DefaultPath is the host's log file, paths.HostLog().
func DefaultPath() (string, error) { return paths.HostLog() }

// GUIPath is the GUI process's own log file. The host and the GUI are separate
// processes and Windows allows neither to rename a file the other holds open,
// so one shared file would break rotation for both; two files keep each
// process independent.
func GUIPath() (string, error) { return paths.GUILog() }

// Logger appends timestamped, redacted lines to one file.
type Logger struct {
	mu      sync.Mutex
	path    string
	level   Level
	maxSize int64
	file    *os.File
}

// New opens path for append, creating its directory, and rotates it once it
// reaches 5 MiB.
func New(path string, level Level) (*Logger, error) {
	return newWithMaxSize(path, level, defaultMax)
}

func newWithMaxSize(path string, level Level, maxSize int64) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	l := &Logger{path: path, level: level, maxSize: maxSize}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

// Nop discards everything. Run falls back to it when the real log cannot be
// opened, because a log failure must not block an unlock.
func Nop() *Logger { return &Logger{} }

func (l *Logger) open() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	l.file = f
	return nil
}

func (l *Logger) Info(message string)  { l.write(LevelInfo, "INFO", message) }
func (l *Logger) Error(message string) { l.write(LevelInfo, "ERROR", message) }
func (l *Logger) Debug(message string) { l.write(LevelDebug, "DEBUG", message) }

func (l *Logger) write(level Level, tag, message string) {
	if l == nil || level > l.level {
		return
	}
	line := "[" + time.Now().Format("2006-01-02 15:04:05.000") + "] [" + tag + "] " +
		Redact(message) + "\n"

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	l.rotateLocked()
	if l.file == nil {
		return
	}
	_, _ = l.file.WriteString(line)
}

// rotateLocked keeps one rotated file. The caller holds the lock, so the size
// check and the rename cannot race a write from this logger.
func (l *Logger) rotateLocked() {
	info, err := l.file.Stat()
	if err != nil || info.Size() < l.maxSize {
		return
	}
	_ = l.file.Close()
	l.file = nil
	_ = os.Remove(l.path + rotatedSuffix)
	if err := os.Rename(l.path, l.path+rotatedSuffix); err != nil {
		// Windows refuses the rename while any other handle has the file open
		// (os.OpenFile does not ask for FILE_SHARE_DELETE), so a second writer
		// would leave the log growing without bound. Copy and empty instead.
		l.copyRotate()
	}
	_ = l.open()
}

// copyRotate is the rotation that still works when another writer holds the
// file: copy the oversized content to the rotated name, then truncate the
// original. A line the other writer adds between the copy and the truncate is
// lost, which is the price of never letting the file grow unbounded.
func (l *Logger) copyRotate() {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return
	}
	if err := os.WriteFile(l.path+rotatedSuffix, data, 0o600); err != nil {
		return
	}
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	_ = f.Close()
}

// Close flushes and closes the file; a second call is a no-op.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

var (
	secretField = regexp.MustCompile(`"(userKeyB64|keyB64|user_key|sharedSecret|publicKey|encryptedString|ciphertext|data|iv|mac)"\s*:\s*"[^"]*"`)
	secretBlob  = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
)

// Redact blanks the value of every field that carries key or cipher material,
// then any long base64 run. The field pass is exact for the protocol's JSON
// (e.g. a reply logged before sealing); the blob pass catches bare key strings.
func Redact(message string) string {
	out := secretField.ReplaceAllString(message, `"$1":"<redacted>"`)
	return secretBlob.ReplaceAllString(out, "<redacted>")
}
