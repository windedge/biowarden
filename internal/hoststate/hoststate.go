// Package hoststate records which browser's extension last reached the host,
// so the GUI can show a per-browser "connected" indicator. The state lives
// outside host.log: the log is a human-facing diagnostic that rotates, while
// this needs to survive and be read back cheaply.
package hoststate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Browser ids the host records. They name the engine that launched the host,
// not the extension origin: Bitwarden uses the same extension id in every
// browser, so the origin argument cannot tell Chrome from Edge.
const (
	BrowserChrome   = "chrome"
	BrowserChromium = "chromium"
	BrowserEdge     = "edge"
	BrowserFirefox  = "firefox"
	BrowserBrave    = "brave"
	BrowserVivaldi  = "vivaldi"
	BrowserUnknown  = "unknown"
)

const stateDirName = "connections"

// Identify maps a launcher executable name to a browser id, reporting "" for
// an engine biowarden does not know. Chrome and Chromium stay distinct here
// even though biowarden registers both through one Chrome row: the executable
// name is the signal that separates them.
func Identify(exe string) string {
	name := strings.ToLower(strings.TrimSpace(exe))
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "chrome":
		return BrowserChrome
	case "chromium":
		return BrowserChromium
	case "msedge":
		return BrowserEdge
	case "firefox":
		return BrowserFirefox
	case "brave", "brave-browser":
		return BrowserBrave
	case "vivaldi":
		return BrowserVivaldi
	}
	return ""
}

// defaultDir is %LOCALAPPDATA%\biowarden\connections, mirroring hostlog's
// fallback to the user cache dir so the package still works outside Windows.
func defaultDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = cache
	}
	return filepath.Join(base, "biowarden", stateDirName), nil
}

// RecordConnection persists browser's last connection time. An empty or
// unsupported id is kept as "unknown" so a run still leaves a trace.
func RecordConnection(browser string, at time.Time) error {
	dir, err := defaultDir()
	if err != nil {
		return err
	}
	return RecordConnectionIn(dir, browser, at)
}

// RecordConnectionIn is RecordConnection against an explicit directory, which
// is what tests use to keep off the real profile.
func RecordConnectionIn(dir, browser string, at time.Time) error {
	if at.IsZero() {
		at = time.Now()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := recordName(browser)
	// One file per browser plus an atomic rename is the whole concurrency
	// story: two host processes writing different browsers never touch the
	// same file, and two writing the same browser each move a complete temp
	// file into place, so a reader sees one timestamp or the other, never a
	// torn one.
	f, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.WriteString(at.Format(time.RFC3339Nano))
	cerr := f.Close()
	if werr != nil {
		os.Remove(tmp)
		return werr
	}
	if cerr != nil {
		os.Remove(tmp)
		return cerr
	}
	if err := renameIntoPlace(tmp, filepath.Join(dir, name)); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// renameMu serializes renames inside one process; the OS has to serialize the
// rest. It only covers the rename, not the write, so two browsers never wait
// on each other longer than a syscall.
var renameMu sync.Mutex

// renameIntoPlace moves a finished temp file onto its final name. Windows
// refuses a concurrent replace of the same destination with a sharing or
// access error instead of waiting, so a writer retries briefly: two host
// processes for one browser are exactly the case this state has to survive.
func renameIntoPlace(tmp, dst string) error {
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Millisecond)
		}
		renameMu.Lock()
		err = os.Rename(tmp, dst)
		renameMu.Unlock()
		if err == nil {
			return nil
		}
	}
	return err
}

// LastConnections reads each browser's last connection time, keyed by browser
// id. A missing directory means the host never recorded anything, which is the
// empty state rather than an error.
func LastConnections() (map[string]time.Time, error) {
	dir, err := defaultDir()
	if err != nil {
		return nil, err
	}
	return LastConnectionsIn(dir)
}

// LastConnectionsIn is LastConnections against an explicit directory. A file
// that does not parse is skipped: one bad byte must not hide every other
// browser's record.
func LastConnectionsIn(dir string) (map[string]time.Time, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]time.Time{}, nil
		}
		return nil, err
	}
	out := make(map[string]time.Time, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasSuffix(name, ".tmp") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(raw)))
		if err != nil {
			continue
		}
		out[name] = at
	}
	return out, nil
}

// recordName is the file name for a browser id. Only [a-z0-9_-] survives, so a
// hostile or accidental executable name cannot point the write outside the
// state directory.
func recordName(browser string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(browser)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return BrowserUnknown
	}
	return b.String()
}
