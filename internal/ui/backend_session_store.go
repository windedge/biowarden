package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/danieljoos/wincred"

	"github.com/windedge/biowarden/internal/auth"
)

// errNoStoredSession reports that no login is persisted: the normal state on a
// fresh install, after a logout, and after a revoked session was cleared.
var errNoStoredSession = errors.New("ui: no stored session")

// errSessionRevoked reports a persisted login the server rejected. The stored
// session credential is cleared, but the key is left alone so unlocking keeps
// working.
var errSessionRevoked = errors.New("ui: login session revoked")

// errSessionUnverified reports a persisted login that could not be checked
// because the network or the server failed. The session credential is kept,
// and the page conservatively shows the configured state instead of claiming
// a login it could not confirm.
var errSessionUnverified = errors.New("ui: login session not verified yet")

// sessionRecord is the persisted login: the only thing biowarden keeps between
// runs besides the key. It carries the refresh token so a restart can verify
// the session without the master password; it never carries the password or
// the account key.
type sessionRecord struct {
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	ServerURL    string `json:"serverURL"`
	RefreshToken string `json:"refreshToken"`
}

// String redacts the refresh token so a record cannot reach a log through %v.
func (sessionRecord) String() string { return "sessionRecord(<redacted>)" }

// encodeSession is the exact bytes a store writes, so a test can assert the
// master password and the account key never appear in them.
func encodeSession(r sessionRecord) ([]byte, error) { return json.Marshal(r) }

func decodeSession(b []byte) (sessionRecord, error) {
	var r sessionRecord
	if err := json.Unmarshal(b, &r); err != nil {
		return sessionRecord{}, fmt.Errorf("ui: session corrupt: %w", err)
	}
	return r, nil
}

// sessionStore is the persisted-login seam. Save, Load and Clear never see the
// master password or the user key, only the refresh token and the identity.
type sessionStore interface {
	save(sessionRecord) error
	load() (sessionRecord, error) // errNoStoredSession when nothing is stored
	clear(userID string) error
}

// nopSessionStore is the injected default for callers that do not persist a
// session (the tests that predate the session ticket). It stores nothing.
type nopSessionStore struct{}

func (nopSessionStore) save(sessionRecord) error     { return nil }
func (nopSessionStore) clear(string) error           { return nil }
func (nopSessionStore) load() (sessionRecord, error) { return sessionRecord{}, errNoStoredSession }

// wincredSessionStore keeps the persisted login in the Windows Credential
// Manager under the Session subtree (Biowarden/Session/<userId>), separate
// from the account keys. DPAPI encrypts it for the current Windows user, the
// same protection the key entries get.
type wincredSessionStore struct {
	prefix string // the session prefix, e.g. secrets.SessionPrefix
}

// newWincredSessionStore is the real store over prefix.
func newWincredSessionStore(prefix string) *wincredSessionStore {
	return &wincredSessionStore{prefix: prefix}
}

func (s *wincredSessionStore) target(userID string) string { return s.prefix + "/" + userID }

func (s *wincredSessionStore) save(rec sessionRecord) error {
	if rec.UserID == "" {
		return errors.New("ui: session has no user id")
	}
	blob, err := encodeSession(rec)
	if err != nil {
		return err
	}
	cred := wincred.NewGenericCredential(s.target(rec.UserID))
	cred.UserName = rec.UserID
	cred.CredentialBlob = blob
	if err := cred.Write(); err != nil {
		return err
	}
	// One active login: drop any older session so a stale account cannot be
	// restored after a different account signed in.
	creds, err := wincred.FilteredList(s.prefix + "/*")
	if err != nil {
		return err
	}
	for _, old := range creds {
		if old.TargetName == s.target(rec.UserID) {
			continue
		}
		full, err := wincred.GetGenericCredential(old.TargetName)
		if err != nil {
			continue
		}
		_ = full.Delete()
	}
	return nil
}

func (s *wincredSessionStore) load() (sessionRecord, error) {
	creds, err := wincred.FilteredList(s.prefix + "/*")
	if err != nil {
		return sessionRecord{}, err
	}
	if len(creds) == 0 {
		return sessionRecord{}, errNoStoredSession
	}
	newest := creds[0]
	for _, c := range creds[1:] {
		if c.LastWritten.After(newest.LastWritten) {
			newest = c
		}
	}
	full, err := wincred.GetGenericCredential(newest.TargetName)
	if err != nil {
		return sessionRecord{}, err
	}
	return decodeSession(full.CredentialBlob)
}

func (s *wincredSessionStore) clear(userID string) error {
	if userID == "" {
		return nil
	}
	cred, err := wincred.GetGenericCredential(s.target(userID))
	if errors.Is(err, wincred.ErrElementNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return cred.Delete()
}

// sessionRefresher verifies a persisted refresh token against the server and
// returns the fresh grant. It is the auth seam behind startup recovery, and
// the only place the session store's contents meet the network.
type sessionRefresher func(ctx context.Context, serverURL, refreshToken string) (auth.TokenResponse, error)

// sessionRefreshTimeout bounds one refresh attempt in the startup restore. It
// stays well under the login path's 30s because the account page is on screen
// waiting, but must clear a self-hosted server behind a CDN or a proxy: the
// lightest request on such a host measures 1.6-1.9s, so a cold-start token
// POST can legitimately take several seconds and must not be declared dead.
const sessionRefreshTimeout = 12 * time.Second

// The backoff between refresh attempts, in milliseconds. Package vars so the
// tests shrink them to nothing; the values match the ticket: short waits for
// transport jitter, a longer one when the server said 429 and asked for it.
var (
	refreshRetryBackoffsMs = []int{200, 600}
	refreshRateLimitWaitMs = 1000
)

// refreshRetryWait waits one backoff, or returns immediately when ctx is
// cancelled while waiting.
func refreshRetryWait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// refreshRetryClass names the failure class a refresh ended in, for the log
// line restoreSession writes. It says which sentinel matched, never the
// error's own text.
func refreshRetryClass(err error) string {
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

// refreshRetryable reports whether a failed refresh is worth trying again:
// transport and server trouble can clear on its own, credential and policy
// answers cannot. Two-factor and captcha are terminal here too - a refresh
// token cannot answer an interactive challenge.
func refreshRetryable(err error) bool {
	switch {
	case errors.Is(err, auth.ErrNetwork), errors.Is(err, auth.ErrServer):
		return true
	case errors.Is(err, auth.ErrRateLimited):
		return true
	default:
		return false
	}
}

// refreshWithRetry runs refresher with up to len(refreshRetryBackoffsMs)+1
// attempts and per-class backoff: 200ms/600ms for transport and server
// failures, a longer 1s wait after a 429. It stops at the first success and
// never retries a credential or policy refusal, so a dead token still answers
// after one round trip.
func refreshWithRetry(ctx context.Context, refresher sessionRefresher, serverURL, refreshToken string) (auth.TokenResponse, error) {
	var (
		tok auth.TokenResponse
		err error
	)
	for attempt := 0; ; attempt++ {
		tok, err = refresher(ctx, serverURL, refreshToken)
		if err == nil || attempt >= len(refreshRetryBackoffsMs) || !refreshRetryable(err) {
			return tok, err
		}
		wait := time.Duration(refreshRetryBackoffsMs[attempt]) * time.Millisecond
		if errors.Is(err, auth.ErrRateLimited) {
			wait = time.Duration(refreshRateLimitWaitMs) * time.Millisecond
		}
		if waitErr := refreshRetryWait(ctx, wait); waitErr != nil {
			return auth.TokenResponse{}, waitErr
		}
	}
}

// newSessionRefresher builds the real refresher through internal/auth. The
// refresh grant returns no account key, so a restored session can prove the
// login but still needs the master password to re-derive the key. The client
// gets the short startup timeout instead of the login path's 30 seconds;
// transport failures are retried by refreshWithRetry, not here, so the real
// path and the tests share one policy.
func newSessionRefresher() sessionRefresher {
	return func(ctx context.Context, serverURL, refreshToken string) (auth.TokenResponse, error) {
		cfg, err := auth.NewServerConfig(serverURL)
		if err != nil {
			return auth.TokenResponse{}, err
		}
		client := auth.NewClient(cfg, deviceID).WithTimeout(sessionRefreshTimeout)
		return client.Refresh(ctx, refreshToken)
	}
}

// sessionRevoked reports whether a refresh failure means the token is dead
// (the session must be cleared) rather than a transport problem (the session
// must be kept). The mapping is by auth's sentinels: 401/invalid_grant and
// 403 are revocations, everything else (network, 5xx, config) is unverified.
func sessionRevoked(err error) bool {
	return errors.Is(err, auth.ErrUnauthorized) || errors.Is(err, auth.ErrForbidden)
}
