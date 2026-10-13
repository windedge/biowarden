package ui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"

	"github.com/windedge/biowarden/internal/autostart"
	"github.com/windedge/biowarden/internal/hello"
)

// realSettings is the production settingsBackend: the login item and the
// host's log file. It also embeds a Hello Verifier, which the Settings page
// asks for directly (the settingsBackend interface does not carry it).
type realSettings struct {
	// hello.Verifier is embedded so a test can inject a fake; production uses
	// hello.New, whose Available check never runs on the UI thread.
	hello.Verifier

	openAtLogin    func() bool
	setOpenAtLogin func(bool) error
	// runOnMain runs the login-item write inside the app's main loop, where
	// mygo.App.SetOpenAtLogin belongs. It is injected so tests avoid mygo.
	runOnMain func(func())
	logFile   string
}

// settingsBackendOnce keeps one realSettings per process. The Hello verifier
// owns an OS thread and a hidden window, so building it once per page would
// leave a second one behind; the GUI has a single window, so one is right.
var (
	settingsBackendOnce sync.Once
	settingsBackendReal *realSettings
)

func realSettingsInstance() *realSettings {
	settingsBackendOnce.Do(func() { settingsBackendReal = newRealSettings() })
	return settingsBackendReal
}

func newRealSettings() *realSettings {
	return &realSettings{
		Verifier:       hello.New(),
		openAtLogin:    autostart.OpenAtLogin,
		setOpenAtLogin: autostart.SetOpenAtLogin,
		runOnMain:      mygo.RunOnMain,
		logFile:        hostLogPath(),
	}
}

func (s *realSettings) autostart() (bool, error) {
	return s.openAtLogin(), nil
}

// setAutostart marshals onto the main loop: autostart writes through
// mygo.App, which must run there. RunOnMain runs inline when the caller is
// already on the main thread, so the page's click path pays nothing extra.
func (s *realSettings) setAutostart(open bool) error {
	var err error
	s.runOnMain(func() { err = s.setOpenAtLogin(open) })
	return err
}

func (s *realSettings) logPath() string { return s.logFile }

// logTail reads the last n bytes of the log, trimmed to a whole first line.
// A missing file reads as empty: the host may never have run.
func (s *realSettings) logTail(n int64) (string, error) {
	if n <= 0 {
		return "", nil
	}
	f, err := os.Open(s.logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	start := info.Size() - n
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	if start > 0 {
		// The window may start mid-line. It is already whole when the byte
		// just before it ends a line; otherwise drop to after the next newline.
		prev := make([]byte, 1)
		if _, err := f.ReadAt(prev, start-1); err != nil {
			return "", err
		}
		if prev[0] != '\n' {
			if i := bytes.IndexByte(data, '\n'); i >= 0 {
				data = data[i+1:]
			} else {
				data = nil
			}
		}
	}
	return string(data), nil
}

// hostLogPath matches the path the host writes. logPath has no error return,
// so a missing LOCALAPPDATA falls back to the temp directory instead of
// showing an empty field.
func hostLogPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "biowarden", "host.log")
}
