package hostlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windedge/biowarden/internal/paths"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]Level{
		"":        LevelInfo,
		"info":    LevelInfo,
		"INFO":    LevelInfo,
		"debug":   LevelDebug,
		" DEBUG ": LevelDebug,
		"trace":   LevelInfo,
	}
	for input, want := range cases {
		if got := ParseLevel(input); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestRedactMasksSecretFields(t *testing.T) {
	blob := strings.Repeat("A", 44)
	cases := []string{
		`{"userKeyB64":"` + blob + `"}`,
		`{"keyB64":"AAAA"}`,
		`{"user_key":"` + blob + `"}`,
		`{"sharedSecret":"QUJD"}`,
		`{"encryptedString":"2.iv|data|mac"}`,
		`{"iv":"short","mac":"short"}`,
	}
	for _, in := range cases {
		got := Redact(in)
		if !strings.Contains(got, "<redacted>") {
			t.Errorf("Redact(%q) = %q, want a redaction marker", in, got)
		}
		if strings.Contains(got, "AAAA") || strings.Contains(got, blob) {
			t.Errorf("Redact(%q) leaked a value: %q", in, got)
		}
	}
}

// A bare base64 key has no field name; the length fallback still has to hide it.
func TestRedactMasksBareBlob(t *testing.T) {
	blob := strings.Repeat("Zm9v", 12) // 48 base64 characters
	got := Redact("transport key: " + blob)
	if strings.Contains(got, blob) {
		t.Fatalf("Redact leaked a bare base64 blob: %q", got)
	}
}

func TestRedactKeepsPlainText(t *testing.T) {
	in := "legacy command=unlockWithBiometricsForUser userId=6f1c9a04-1111-2222-3333-444455556666 messageId=7"
	if got := Redact(in); got != in {
		t.Fatalf("Redact changed plain text:\n got %q\nwant %q", got, in)
	}
}

func TestLoggerWritesRedactedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.log")
	logger, err := New(path, LevelInfo)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	secret := strings.Repeat("A", 44)
	logger.Info(`reply {"userKeyB64":"` + secret + `"}`)
	logger.Debug("debug line must be dropped at info level")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(content)
	if strings.Contains(text, secret) {
		t.Fatalf("log contains the secret:\n%s", text)
	}
	if !strings.Contains(text, "<redacted>") {
		t.Fatalf("log has no redaction marker:\n%s", text)
	}
	if strings.Contains(text, "debug line") {
		t.Fatalf("info-level logger wrote a debug line:\n%s", text)
	}
}

func TestLoggerRotatesAtMaxSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.log")
	logger, err := newWithMaxSize(path, LevelInfo, 200)
	if err != nil {
		t.Fatalf("newWithMaxSize: %v", err)
	}
	// Hyphens keep the padding out of the base64 fallback in Redact, so the
	// line length reaching the file is predictable.
	line := strings.Repeat("-", 120)
	for i := 0; i < 3; i++ {
		logger.Info(line)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(path + rotatedSuffix); err != nil {
		t.Fatalf("rotated file %s missing: %v", path+rotatedSuffix, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current log: %v", err)
	}
	if info.Size() >= 200 {
		t.Fatalf("current log is %d bytes, want it rotated below maxSize", info.Size())
	}
}

// The host and the GUI must not share a file: each has its own path, and the
// paths differ, so one process's open handle never blocks the other's rotation.
func TestGUIPathIsDistinct(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	hostPath, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	guiPath, err := GUIPath()
	if err != nil {
		t.Fatalf("GUIPath: %v", err)
	}
	if hostPath == guiPath {
		t.Fatalf("DefaultPath and GUIPath are the same file: %s", hostPath)
	}
	if filepath.Base(hostPath) != paths.HostLogName || filepath.Base(guiPath) != paths.GUILogName {
		t.Fatalf("paths = %s, %s; want host.log and gui.log", hostPath, guiPath)
	}
	if filepath.Dir(hostPath) != filepath.Dir(guiPath) {
		t.Fatalf("host and gui logs live in different directories: %s, %s", hostPath, guiPath)
	}
}

// Two loggers on one path is what the debug loggers do inside the host
// process. The rename cannot succeed while the other handle is open, so the
// copy fallback has to keep the file bounded instead of appending forever.
func TestSharedPathRotationStaysBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.log")
	const maxSize = 400
	a, err := newWithMaxSize(path, LevelInfo, maxSize)
	if err != nil {
		t.Fatalf("newWithMaxSize: %v", err)
	}
	b, err := newWithMaxSize(path, LevelInfo, maxSize)
	if err != nil {
		t.Fatalf("newWithMaxSize: %v", err)
	}
	defer func() { _ = a.Close(); _ = b.Close() }()

	line := strings.Repeat("-", 100)
	for i := 0; i < 40; i++ {
		if i%2 == 0 {
			a.Info(line)
		} else {
			b.Info(line)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current log: %v", err)
	}
	// The fallback truncates at the max size, so one more line is the worst case.
	if info.Size() > maxSize+int64(len(line))+64 {
		t.Fatalf("current log grew to %d bytes with maxSize %d: rotation never took effect",
			info.Size(), maxSize)
	}
	rotated, err := os.Stat(path + rotatedSuffix)
	if err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
	if rotated.Size() == 0 {
		t.Fatal("rotated file is empty")
	}
}

// With the two processes on their own files, alternating writes rotate each
// file independently: neither logger's handle can block the other's rename.
func TestHostAndGUILoggersRotateIndependently(t *testing.T) {
	dir := t.TempDir()
	hostPath := filepath.Join(dir, paths.HostLogName)
	guiPath := filepath.Join(dir, paths.GUILogName)
	const maxSize = 400
	host, err := newWithMaxSize(hostPath, LevelInfo, maxSize)
	if err != nil {
		t.Fatalf("host logger: %v", err)
	}
	gui, err := newWithMaxSize(guiPath, LevelInfo, maxSize)
	if err != nil {
		t.Fatalf("gui logger: %v", err)
	}
	defer func() { _ = host.Close(); _ = gui.Close() }()

	hostLine := "host " + strings.Repeat("-", 90)
	guiLine := "gui " + strings.Repeat("-", 90)
	for i := 0; i < 20; i++ {
		host.Info(hostLine)
		gui.Info(guiLine)
	}

	for _, path := range []string{hostPath, guiPath} {
		if _, err := os.Stat(path + rotatedSuffix); err != nil {
			t.Fatalf("%s did not rotate: %v", filepath.Base(path), err)
		}
	}
	hostText, err := os.ReadFile(hostPath + rotatedSuffix)
	if err != nil {
		t.Fatalf("read rotated host log: %v", err)
	}
	if strings.Contains(string(hostText), "gui ") {
		t.Fatalf("the GUI wrote into the host's log:\n%s", hostText)
	}
}

func TestNopLoggerIsSafe(t *testing.T) {
	logger := Nop()
	logger.Info("ignored")
	logger.Debug("ignored")
	logger.Error("ignored")
	if err := logger.Close(); err != nil {
		t.Fatalf("Nop Close: %v", err)
	}
}
