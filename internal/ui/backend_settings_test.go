package ui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLogTailBoundaries(t *testing.T) {
	log := func(content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "host.log")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	empty := log("")

	tests := []struct {
		name string
		path string
		n    int64
		want string
	}{
		{name: "missing file", path: filepath.Join(t.TempDir(), "absent.log"), n: 1024, want: ""},
		{name: "empty file", path: empty, n: 1024, want: ""},
		{name: "smaller than n", path: log("a\nb\n"), n: 1024, want: "a\nb\n"},
		{name: "non-positive n", path: log("a\nb\n"), n: 0, want: ""},
		{name: "n larger than size keeps whole file", path: log("one\ntwo\n"), n: 99, want: "one\ntwo\n"},
		{name: "window starts mid-line drops partial first line",
			path: log("1111111111\n2222222222\n3333333333\n"), n: 12, want: "3333333333\n"},
		{name: "window starts on a line boundary keeps first line",
			path: log("aaa\nbbb\n"), n: 4, want: "bbb\n"},
		{name: "no newline in window returns nothing",
			path: log("0123456789ABCDEFGHIJ"), n: 5, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &realSettings{logFile: tt.path}
			got, err := s.logTail(tt.n)
			if err != nil {
				t.Fatalf("logTail(%d) error: %v", tt.n, err)
			}
			if got != tt.want {
				t.Fatalf("logTail(%d) = %q; want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestRealSettingsAutostart(t *testing.T) {
	boom := errors.New("autostart write failed")
	var calls []bool
	ranOnMain := false
	s := &realSettings{
		openAtLogin:    func() bool { return true },
		setOpenAtLogin: func(open bool) error { calls = append(calls, open); return boom },
		runOnMain:      func(fn func()) { ranOnMain = true; fn() },
	}

	if on, err := s.autostart(); err != nil || !on {
		t.Fatalf("autostart() = %v, %v; want true, nil", on, err)
	}
	if err := s.setAutostart(false); !errors.Is(err, boom) {
		t.Fatalf("setAutostart(false) error = %v; want %v", err, boom)
	}
	if !ranOnMain {
		t.Fatal("setAutostart did not run through the main-thread scheduler")
	}
	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("setOpenAtLogin calls = %v; want [false]", calls)
	}
}

func TestRealSettingsAutostartNilError(t *testing.T) {
	called := false
	s := &realSettings{
		setOpenAtLogin: func(bool) error { called = true; return nil },
		runOnMain:      func(fn func()) { fn() },
	}
	if err := s.setAutostart(true); err != nil {
		t.Fatalf("setAutostart(true) error = %v; want nil", err)
	}
	if !called {
		t.Fatal("setOpenAtLogin was not called")
	}
}
