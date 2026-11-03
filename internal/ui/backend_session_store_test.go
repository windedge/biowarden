package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/hostlog"
	"github.com/windedge/biowarden/internal/session"
)

// shrinkBackoffs makes the retry waits instantaneous for one test.
func shrinkBackoffs(t *testing.T) {
	t.Helper()
	t.Cleanup(session.OverrideRetryWaits([]int{0, 0}, 0))
}

// failOnceRefresher fails its first call and succeeds afterwards, counting the
// calls so a restore test can assert the retry policy ran.
type failOnceRefresher struct {
	tok   auth.TokenResponse
	err   error
	calls int
}

func (f *failOnceRefresher) refresh(context.Context, string, string) (auth.TokenResponse, error) {
	f.calls++
	if f.calls == 1 {
		return auth.TokenResponse{}, f.err
	}
	return f.tok, nil
}

// The verify log line carries counts and classes, never tokens or emails.
func TestLogSessionVerifyWritesCountsAndClasses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.log")
	l, err := hostlog.New(path, hostlog.LevelInfo)
	if err != nil {
		t.Fatalf("hostlog.New: %v", err)
	}
	defer l.Close()
	a := newRealAccount(nil, nil, nil)
	a.log = l

	a.logSessionVerify(3, fmt.Errorf("%w: dial tcp", auth.ErrNetwork), 2500*time.Millisecond)
	a.logSessionVerify(1, nil, time.Second)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(got)
	for _, want := range []string{"attempts=3", "outcome=network", "attempts=1", "outcome=ok"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log = %q, want it to contain %q", text, want)
		}
	}
}
