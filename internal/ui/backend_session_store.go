package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/danieljoos/wincred"

	"github.com/windedge/biowarden/internal/auth"
)

// errNoStoredSession reports that no login is persisted: the normal state on a
// fresh install, after a logout, and after a revoked session was cleared.
var errNoStoredSession = errors.New("ui: no stored session")

// errSessionRevoked reports a persisted login the server rejected. The stored
// session credential is cleared, but the key is left alone so unlocking keeps
// working.
var errSessionRevoked = errors.New("ui: 登录会话已失效")

// errSessionUnverified reports a persisted login that could not be checked
// because the network or the server failed. The session credential is kept,
// and the page conservatively shows the configured state instead of claiming
// a login it could not confirm.
var errSessionUnverified = errors.New("ui: 登录会话暂未验证")

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

// newSessionRefresher builds the real refresher through internal/auth. The
// refresh grant returns no account key, so a restored session can prove the
// login but still needs the master password to re-derive the key.
func newSessionRefresher() sessionRefresher {
	return func(ctx context.Context, serverURL, refreshToken string) (auth.TokenResponse, error) {
		cfg, err := auth.NewServerConfig(serverURL)
		if err != nil {
			return auth.TokenResponse{}, err
		}
		return auth.NewClient(cfg, deviceID).Refresh(ctx, refreshToken)
	}
}

// sessionRevoked reports whether a refresh failure means the token is dead
// (the session must be cleared) rather than a transport problem (the session
// must be kept). The mapping is by auth's sentinels: 401/invalid_grant and
// 403 are revocations, everything else (network, 5xx, config) is unverified.
func sessionRevoked(err error) bool {
	return errors.Is(err, auth.ErrUnauthorized) || errors.Is(err, auth.ErrForbidden)
}
