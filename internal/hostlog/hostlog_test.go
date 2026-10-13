package hostlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestNopLoggerIsSafe(t *testing.T) {
	logger := Nop()
	logger.Info("ignored")
	logger.Debug("ignored")
	logger.Error("ignored")
	if err := logger.Close(); err != nil {
		t.Fatalf("Nop Close: %v", err)
	}
}
