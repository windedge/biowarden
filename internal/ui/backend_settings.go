package ui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/windedge/biowarden/internal/autostart"
	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/hostlog"
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
	// guiLogFile is the GUI process's own log; the page joins it after the
	// host's, because the two processes write separate files.
	guiLogFile string
}

// newRealSettings takes the process's shared verifier; realSettingsInstance
// and its sync.Once are gone because newAppState builds the single instance
// itself and hands it to every page that needs it.
func newRealSettings(verifier hello.Verifier) *realSettings {
	return &realSettings{
		Verifier:       verifier,
		openAtLogin:    autostart.OpenAtLogin,
		setOpenAtLogin: autostart.SetOpenAtLogin,
		runOnMain:      mygo.RunOnMain,
		logFile:        hostLogPath(),
		guiLogFile:     guiLogPath(),
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

// logTail reads the last n bytes of the host's log, trimmed to a whole first
// line, then the same window of the GUI's log under a line naming that file.
// A missing file reads as empty: the process that writes it may never have run.
func (s *realSettings) logTail(n int64) (string, error) {
	hostTail, err := tailFile(s.logFile, n)
	if err != nil {
		return "", err
	}
	guiTail, err := tailFile(s.guiLogFile, n)
	if err != nil {
		return "", err
	}
	if guiTail == "" {
		return hostTail, nil
	}
	return hostTail + "=== " + filepath.Base(s.guiLogFile) + " ===\n" + guiTail, nil
}

// tailFile reads the last n bytes of path, trimmed to a whole first line. A
// missing file reads as empty.
func tailFile(path string, n int64) (string, error) {
	if n <= 0 || path == "" {
		return "", nil
	}
	f, err := os.Open(path)
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

// hostLogPath and guiLogPath match the paths the two processes write through
// hostlog. logPath has no error return, so a missing LOCALAPPDATA falls back to
// the temp directory instead of showing an empty field.
func hostLogPath() string { return resolveLogPath(hostlog.DefaultPath, "host.log") }

func guiLogPath() string { return resolveLogPath(hostlog.GUIPath, "gui.log") }

func resolveLogPath(resolve func() (string, error), name string) string {
	if path, err := resolve(); err == nil {
		return path
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "biowarden", name)
}
