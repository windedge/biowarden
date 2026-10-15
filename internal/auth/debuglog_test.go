package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windedge/biowarden/internal/hostlog"
)

// setDebugLogger redirects the package diagnostic logger into a test log and
// restores the previous value afterwards.
func setDebugLogger(t *testing.T, l *hostlog.Logger) {
	t.Helper()
	debugLogMu.Lock()
	prev := debugLog
	debugLog = l
	debugLogMu.Unlock()
	t.Cleanup(func() {
		debugLogMu.Lock()
		debugLog = prev
		debugLogMu.Unlock()
	})
}

// The shape of the real 2FA body is logged as field names, types and provider
// numbers only. None of the values (email, two-factor token, error text) may
// appear.
func TestTwoFactorShapeNamesFieldsWithoutValues(t *testing.T) {
	body := []byte(`{
		"error":"invalid_grant",
		"error_description":"Two factor required.",
		"TwoFactorProviders":["0"],
		"TwoFactorProviders2":{"0":null},
		"Email":"ada@example.com",
		"TwoFactorToken":"two-factor-secret-value"
	}`)
	got := twoFactorShape(body)
	for _, want := range []string{
		"keys=[",
		"TwoFactorProviders",
		"TwoFactorProviders2",
		"types=[",
		"TwoFactorProviders=array<string>",
		"TwoFactorProviders2=object",
		"providers=[0]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("twoFactorShape() = %q, want it to contain %q", got, want)
		}
	}
	for _, leak := range []string{"ada@example.com", "two-factor-secret-value", "invalid_grant"} {
		if strings.Contains(got, leak) {
			t.Errorf("twoFactorShape() leaked a value %q: %q", leak, got)
		}
	}
}

// The diagnostic is written only when BIOWARDEN_LOG=debug, and the written
// line still carries no values.
func TestLogTwoFactorShapeWritesOnlyInDebug(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.log")
	debugLogger, err := hostlog.New(path, hostlog.LevelDebug)
	if err != nil {
		t.Fatalf("hostlog.New: %v", err)
	}
	setDebugLogger(t, debugLogger)

	body := []byte(`{"TwoFactorProviders":["0"],"TwoFactorProviders2":{"0":null},"Email":"ada@example.com","TwoFactorToken":"two-factor-secret-value"}`)

	t.Setenv("BIOWARDEN_LOG", "")
	logTwoFactorShape(body)

	t.Setenv("BIOWARDEN_LOG", "debug")
	logTwoFactorShape(body)

	if err := debugLogger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(content)
	if lines := strings.Count(text, "\n"); lines != 1 {
		t.Fatalf("log line count = %d, want 1 (info level must write nothing):\n%s", lines, text)
	}
	for _, want := range []string{"two-factor response", "keys=[", "providers=[0]"} {
		if !strings.Contains(text, want) {
			t.Errorf("debug line missing %q:\n%s", want, text)
		}
	}
	for _, leak := range []string{"ada@example.com", "two-factor-secret-value", "invalid_grant"} {
		if strings.Contains(text, leak) {
			t.Errorf("log leaked %q:\n%s", leak, text)
		}
	}
}
