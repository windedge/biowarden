package session

import (
	"context"
	"errors"
	"time"

	"github.com/windedge/biowarden/internal/auth"
)

// Refresher verifies a persisted refresh token against the server and returns
// the fresh grant. It is the auth seam behind startup recovery, and the only
// place a stored record meets the network.
type Refresher func(ctx context.Context, serverURL, refreshToken string) (auth.TokenResponse, error)

// refreshTimeout bounds one refresh attempt in the startup restore. It stays
// well under the login path's 30s because the account page is on screen
// waiting, but must clear a self-hosted server behind a CDN or a proxy: the
// lightest request on such a host measures 1.6-1.9s, so a cold-start token
// POST can legitimately take several seconds and must not be declared dead.
const refreshTimeout = 12 * time.Second

// The backoff between refresh attempts, in milliseconds: short waits for
// transport jitter, a longer one when the server said 429 and asked for it.
// Package vars so a test can shrink them to nothing through OverrideRetryWaits.
var (
	retryBackoffsMs      = []int{200, 600}
	retryRateLimitWaitMs = 1000
)

// OverrideRetryWaits replaces the retry waits and returns a func restoring the
// production values. Tests use it so a retry policy can be asserted without
// sleeping through the real backoff.
func OverrideRetryWaits(backoffsMs []int, rateLimitWaitMs int) (restore func()) {
	oldBackoffs, oldLimited := retryBackoffsMs, retryRateLimitWaitMs
	retryBackoffsMs, retryRateLimitWaitMs = backoffsMs, rateLimitWaitMs
	return func() { retryBackoffsMs, retryRateLimitWaitMs = oldBackoffs, oldLimited }
}

// RefreshWait waits one backoff, or returns immediately when ctx is cancelled
// while waiting.
func RefreshWait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RefreshClass names the failure class a refresh ended in, for the log line a
// restore writes. It says which sentinel matched, never the error's own text.
func RefreshClass(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, auth.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, auth.ErrForbidden):
		return "forbidden"
	case errors.Is(err, auth.ErrRateLimited):
		return "rate-limited"
	case errors.Is(err, auth.ErrTwoFactorRequired):
		return "two-factor-required"
	case errors.Is(err, auth.ErrCaptchaRequired):
		return "captcha-required"
	case errors.Is(err, auth.ErrNetwork):
		return "network"
	case errors.Is(err, auth.ErrServer):
		return "server"
	default:
		return "other"
	}
}

// Retryable reports whether a failed refresh is worth trying again: transport,
// server and rate-limit trouble can clear on its own, credential and policy
// answers cannot. Two-factor and captcha are terminal here too - a refresh
// token cannot answer an interactive challenge.
func Retryable(err error) bool {
	switch {
	case errors.Is(err, auth.ErrNetwork), errors.Is(err, auth.ErrServer):
		return true
	case errors.Is(err, auth.ErrRateLimited):
		return true
	default:
		return false
	}
}

// RefreshWithRetry runs refresher with up to len(retryBackoffsMs)+1 attempts
// and per-class backoff: 200ms/600ms for transport and server failures, a
// longer 1s wait after a 429. It stops at the first success and never retries a
// credential or policy refusal, so a dead token still answers after one round
// trip.
func RefreshWithRetry(ctx context.Context, refresher Refresher, serverURL, refreshToken string) (auth.TokenResponse, error) {
	var (
		tok auth.TokenResponse
		err error
	)
	for attempt := 0; ; attempt++ {
		tok, err = refresher(ctx, serverURL, refreshToken)
		if err == nil || attempt >= len(retryBackoffsMs) || !Retryable(err) {
			return tok, err
		}
		wait := time.Duration(retryBackoffsMs[attempt]) * time.Millisecond
		if errors.Is(err, auth.ErrRateLimited) {
			wait = time.Duration(retryRateLimitWaitMs) * time.Millisecond
		}
		if waitErr := RefreshWait(ctx, wait); waitErr != nil {
			return auth.TokenResponse{}, waitErr
		}
	}
}

// NewRefresher builds the real refresher through internal/auth. The refresh
// grant returns no account key, so a restored session can prove the login but
// still needs the master password to re-derive the key. The client gets the
// short startup timeout instead of the login path's 30 seconds; transport
// failures are retried by RefreshWithRetry, not here, so the real path and the
// tests share one policy.
func NewRefresher(deviceID string) Refresher {
	return func(ctx context.Context, serverURL, refreshToken string) (auth.TokenResponse, error) {
		cfg, err := auth.NewServerConfig(serverURL)
		if err != nil {
			return auth.TokenResponse{}, err
		}
		client := auth.NewClient(cfg, deviceID).WithTimeout(refreshTimeout)
		return client.Refresh(ctx, refreshToken)
	}
}

// Revoked reports whether a refresh failure means the token is dead (the
// session must be cleared) rather than a transport problem (the session must be
// kept). The mapping is by auth's sentinels: 401/invalid_grant and 403 are
// revocations, everything else (network, 5xx, config) is unverified.
func Revoked(err error) bool {
	return errors.Is(err, auth.ErrUnauthorized) || errors.Is(err, auth.ErrForbidden)
}
