package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/hostlog"
)

// scriptedRefresher answers each call with the next scripted result. The
// results list repeats its last entry once exhausted; the tokens list maps
// 1:1 to calls, an entry past its end answering with an empty grant.
type scriptedRefresher struct {
	calls   int
	tokens  []auth.TokenResponse
	results []error
}

func (f *scriptedRefresher) refresh(context.Context, string, string) (auth.TokenResponse, error) {
	i := f.calls
	f.calls++
	if i >= len(f.results) {
		i = len(f.results) - 1
	}
	var tok auth.TokenResponse
	if i < len(f.tokens) {
		tok = f.tokens[i]
	}
	return tok, f.results[i]
}

// shrinkBackoffs makes the retry waits instantaneous for one test.
func shrinkBackoffs(t *testing.T) {
	t.Helper()
	oldBackoffs, oldLimited := refreshRetryBackoffsMs, refreshRateLimitWaitMs
	refreshRetryBackoffsMs, refreshRateLimitWaitMs = []int{0, 0}, 0
	t.Cleanup(func() { refreshRetryBackoffsMs, refreshRateLimitWaitMs = oldBackoffs, oldLimited })
}

// A transient transport failure clears on the retry: the restore succeeds and
// only the failed attempt cost time.
func TestRefreshWithRetryRetriesNetworkError(t *testing.T) {
	shrinkBackoffs(t)
	f := &scriptedRefresher{
		tokens:  []auth.TokenResponse{{}, {RefreshToken: "r-2"}},
		results: []error{fmt.Errorf("%w: dial tcp", auth.ErrNetwork), nil},
	}

	tok, err := refreshWithRetry(context.Background(), f.refresh, defaultServerURL, "r-1")
	if err != nil {
		t.Fatalf("refreshWithRetry: %v", err)
	}
	if tok.RefreshToken != "r-2" {
		t.Fatalf("token = %q, want the first scripted success", tok.RefreshToken)
	}
	if f.calls != 2 {
		t.Fatalf("calls = %d, want 2: one failure then the retry", f.calls)
	}
}

// A persistent transport failure uses every attempt, then surfaces the last
// error.
func TestRefreshWithRetryExhaustsAttempts(t *testing.T) {
	shrinkBackoffs(t)
	f := &scriptedRefresher{
		results: []error{
			fmt.Errorf("%w: dial tcp", auth.ErrNetwork),
			fmt.Errorf("%w: timeout", auth.ErrNetwork),
			fmt.Errorf("%w: connection reset", auth.ErrNetwork),
		},
	}

	_, err := refreshWithRetry(context.Background(), f.refresh, defaultServerURL, "r-1")
	if !errors.Is(err, auth.ErrNetwork) {
		t.Fatalf("error = %v, want the network sentinel", err)
	}
	if !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("error = %v, want the last attempt's error", err)
	}
	if f.calls != len(refreshRetryBackoffsMs)+1 {
		t.Fatalf("calls = %d, want %d attempts", f.calls, len(refreshRetryBackoffsMs)+1)
	}
}

// Credential and policy answers are final: a dead token or a forbidden
// account must not be retried, or the wait doubles the answer for nothing.
func TestRefreshWithRetryDoesNotRetryCredentialErrors(t *testing.T) {
	shrinkBackoffs(t)
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"unauthorized", fmt.Errorf("%w: invalid_grant", auth.ErrUnauthorized)},
		{"forbidden", auth.ErrForbidden},
		{"two-factor", auth.ErrTwoFactorRequired},
		{"captcha", auth.ErrCaptchaRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &scriptedRefresher{results: []error{tc.err, tc.err, tc.err}}
			_, err := refreshWithRetry(context.Background(), f.refresh, defaultServerURL, "r-1")
			if !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want the original sentinel", err)
			}
			if f.calls != 1 {
				t.Fatalf("calls = %d, want 1: %s must not be retried", f.calls, tc.name)
			}
		})
	}
}

// A 429 is retried like a transport failure, but the wait is the longer
// rate-limit one rather than the short backoff.
func TestRefreshWithRetryWaitsLongerWhenRateLimited(t *testing.T) {
	oldBackoffs, oldLimited := refreshRetryBackoffsMs, refreshRateLimitWaitMs
	t.Cleanup(func() { refreshRetryBackoffsMs, refreshRateLimitWaitMs = oldBackoffs, oldLimited })
	// A wait no test can sleep through, and no other wait should be used.
	refreshRetryBackoffsMs, refreshRateLimitWaitMs = []int{1, 1}, 15000

	f := &scriptedRefresher{
		tokens:  []auth.TokenResponse{{RefreshToken: "r-2"}, {RefreshToken: "r-3"}},
		results: []error{auth.ErrRateLimited, nil},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	tok, err := refreshWithRetry(ctx, f.refresh, defaultServerURL, "r-1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want the ctx deadline to fire during the long wait", err)
	}
	if tok.RefreshToken != "" {
		t.Fatalf("token = %q, want none", tok.RefreshToken)
	}
	if f.calls != 1 {
		t.Fatalf("calls = %d, want 1: the wait must come before the retry", f.calls)
	}
}

// A cancelled context stops the wait instead of running the next attempt.
func TestRefreshWithRetryStopsWhenContextCancelled(t *testing.T) {
	oldBackoffs, oldLimited := refreshRetryBackoffsMs, refreshRateLimitWaitMs
	t.Cleanup(func() { refreshRetryBackoffsMs, refreshRateLimitWaitMs = oldBackoffs, oldLimited })
	refreshRetryBackoffsMs, refreshRateLimitWaitMs = []int{10000, 10000}, 0

	f := &scriptedRefresher{results: []error{fmt.Errorf("%w: dial tcp", auth.ErrNetwork)}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Cancel once the first attempt is on the wire.
		for f.calls == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	start := time.Now()
	_, err := refreshWithRetry(ctx, f.refresh, defaultServerURL, "r-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancelled wait took %s, want it to return immediately", elapsed)
	}
	if f.calls != 1 {
		t.Fatalf("calls = %d, want 1: no attempt after the cancel", f.calls)
	}
}

// The failure-class names the sentinel that matched, not the error's text.
func TestRefreshRetryClass(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, "ok"},
		{fmt.Errorf("%w: invalid_grant", auth.ErrUnauthorized), "unauthorized"},
		{auth.ErrForbidden, "forbidden"},
		{auth.ErrRateLimited, "rate-limited"},
		{fmt.Errorf("%w: captcha", auth.ErrCaptchaRequired), "captcha-required"},
		{auth.ErrTwoFactorRequired, "two-factor-required"},
		{fmt.Errorf("%w: dial tcp", auth.ErrNetwork), "network"},
		{fmt.Errorf("%w: 500", auth.ErrServer), "server"},
		{errors.New("boom"), "other"},
	}
	for _, tc := range cases {
		if got := refreshRetryClass(tc.err); got != tc.want {
			t.Errorf("refreshRetryClass(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// The real refresher's client timeout must clear a cold self-hosted token
// POST, not kill it at the first hiccup.
func TestSessionRefreshTimeout(t *testing.T) {
	if sessionRefreshTimeout != 12*time.Second {
		t.Fatalf("sessionRefreshTimeout = %s, want 12s", sessionRefreshTimeout)
	}
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
