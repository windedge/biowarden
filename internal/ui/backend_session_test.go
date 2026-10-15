package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/secrets"
)

// fakeSessionStore is an in-memory sessionStore, so these tests never touch
// the real Credential Manager or the network.
type fakeSessionStore struct {
	rec      sessionRecord
	has      bool
	saveErr  error
	clearErr error
	loadErr  error
	saves    int
	clears   []string
}

func (s *fakeSessionStore) save(rec sessionRecord) error {
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.rec, s.has = rec, true
	return nil
}

func (s *fakeSessionStore) load() (sessionRecord, error) {
	if s.loadErr != nil {
		return sessionRecord{}, s.loadErr
	}
	if !s.has {
		return sessionRecord{}, errNoStoredSession
	}
	return s.rec, nil
}

func (s *fakeSessionStore) clear(userID string) error {
	s.clears = append(s.clears, userID)
	if s.clearErr != nil {
		return s.clearErr
	}
	if s.has && s.rec.UserID == userID {
		s.has = false
	}
	return nil
}

// seedStore puts a key straight into the in-memory store, without counting a
// Put, so a restore test can assert it wrote nothing.
func seedStore(userID, email string) *fakeStore {
	store := newFakeStore()
	store.accounts[userID] = secrets.Account{UserID: userID, Email: email}
	store.keys[userID] = []byte("stored-key")
	return store
}

func fixedRefresher(tok auth.TokenResponse, err error) sessionRefresher {
	return func(context.Context, string, string) (auth.TokenResponse, error) { return tok, err }
}

// A finished login persists the refresh token and the identity, and encodes
// neither the master password nor the account key.
func TestLoginPersistsSessionWithoutPasswordOrKey(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	key := []byte("SUPER-SECRET-USER-KEY")
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", key)}
	flow.step.refreshToken = "refresh-1"
	sessions := &fakeSessionStore{}
	a := newRealAccountWith(store, verifier, fixedFlow(flow), sessions, nil)

	sess, err := a.login("ada@example.com", "hunter2-password", defaultServerURL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.state != loginLoggedIn {
		t.Fatalf("state = %v, want logged in", sess.state)
	}
	if !sessions.has {
		t.Fatal("login did not persist a session")
	}
	rec := sessions.rec
	if rec.UserID != "user-1" || rec.Email != "ada@example.com" ||
		rec.ServerURL != defaultServerURL || rec.RefreshToken != "refresh-1" {
		t.Fatalf("persisted record = %+v", rec)
	}
	blob, err := encodeSession(rec)
	if err != nil {
		t.Fatalf("encodeSession: %v", err)
	}
	if bytes.Contains(blob, []byte("hunter2-password")) {
		t.Fatalf("session blob leaks the master password: %s", blob)
	}
	if bytes.Contains(blob, key) {
		t.Fatalf("session blob leaks the user key: %s", blob)
	}
}

// A session that cannot be written must not fail the login: the key is already
// stored and unlocking still works, so the page only warns.
func TestLoginSessionSaveFailureDoesNotBlockLogin(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	flow.step.refreshToken = "refresh-1"
	sessions := &fakeSessionStore{saveErr: errors.New("credential write failed")}
	a := newRealAccountWith(store, verifier, fixedFlow(flow), sessions, nil)

	sess, err := a.login("ada@example.com", "pw", defaultServerURL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.state != loginLoggedIn {
		t.Fatalf("state = %v, want logged in despite the failed session write", sess.state)
	}
	if !strings.Contains(sess.warning, "无法持久化") {
		t.Fatalf("warning = %q, want the persistence warning", sess.warning)
	}
	if store.putCalls != 1 {
		t.Fatalf("Put calls = %d, want the key stored once", store.putCalls)
	}
}

// stepFromResult must carry the grant's refresh token, or a restart has
// nothing to verify the session with.
func TestStepFromResultCarriesRefreshToken(t *testing.T) {
	res := auth.LoginResult{
		State:   auth.LoginDone,
		Token:   auth.TokenResponse{RefreshToken: "r-1"},
		Account: secrets.Account{UserID: "user-1"},
	}
	step, err := stepFromResult(res)
	if err != nil {
		t.Fatalf("stepFromResult: %v", err)
	}
	if step.refreshToken != "r-1" {
		t.Fatalf("refreshToken = %q, want r-1", step.refreshToken)
	}
}

// A valid persisted login is restored without a Hello prompt or a key write,
// and a rotated refresh token replaces the stored one.
func TestRestoreSessionValid(t *testing.T) {
	store := seedStore("user-1", "ada@example.com")
	verifier := &fakeVerifier{result: hello.Verified}
	sessions := &fakeSessionStore{has: true, rec: sessionRecord{
		UserID: "user-1", Email: "ada@example.com", ServerURL: defaultServerURL, RefreshToken: "r-1",
	}}
	var gotServer, gotToken string
	refresh := func(_ context.Context, server, token string) (auth.TokenResponse, error) {
		gotServer, gotToken = server, token
		return auth.TokenResponse{RefreshToken: "r-2"}, nil
	}
	a := newRealAccountWith(store, verifier, fixedFlow(&scriptedAuth{}), sessions, refresh)

	sess, err := a.restoreSession()
	if err != nil {
		t.Fatalf("restoreSession: %v", err)
	}
	if sess.state != loginLoggedIn || sess.email != "ada@example.com" || sess.userID != "user-1" {
		t.Fatalf("session = %+v, want the restored login", sess)
	}
	if gotServer != defaultServerURL || gotToken != "r-1" {
		t.Fatalf("refresh called with (%q, %q), want the stored values", gotServer, gotToken)
	}
	if sessions.rec.RefreshToken != "r-2" {
		t.Fatalf("stored refresh token = %q, want the rotated r-2", sessions.rec.RefreshToken)
	}
	if verifier.calls != 0 || store.putCalls != 0 || store.removeCalls != 0 {
		t.Fatalf("restore touched Hello(%d) or the key store (put %d, remove %d)",
			verifier.calls, store.putCalls, store.removeCalls)
	}
	again, _ := a.session()
	if again.state != loginLoggedIn {
		t.Fatalf("session() = %+v, want the restored login to stick", again)
	}
}

// A rejected refresh token clears the stored session but never touches the
// stored key: unlocking must keep working.
func TestRestoreSessionRevokedClearsSessionKeepsKey(t *testing.T) {
	store := seedStore("user-1", "ada@example.com")
	sessions := &fakeSessionStore{has: true, rec: sessionRecord{
		UserID: "user-1", Email: "ada@example.com", ServerURL: defaultServerURL, RefreshToken: "r-1",
	}}
	refresh := fixedRefresher(auth.TokenResponse{}, fmt.Errorf("%w: invalid_grant", auth.ErrUnauthorized))
	a := newRealAccountWith(store, &fakeVerifier{result: hello.Verified}, fixedFlow(&scriptedAuth{}), sessions, refresh)

	sess, err := a.restoreSession()
	if !errors.Is(err, errSessionRevoked) {
		t.Fatalf("restoreSession error = %v, want errSessionRevoked", err)
	}
	if sess.state != loginLoggedOut {
		t.Fatalf("session = %+v, want signed out", sess)
	}
	if sessions.has {
		t.Fatal("a revoked session must be cleared")
	}
	if len(sessions.clears) != 1 || sessions.clears[0] != "user-1" {
		t.Fatalf("clears = %v, want user-1", sessions.clears)
	}
	if store.removeCalls != 0 {
		t.Fatalf("Remove calls = %d, want 0: the key must survive a revoked session", store.removeCalls)
	}
	if _, ok := store.keys["user-1"]; !ok {
		t.Fatal("the stored key was deleted by session recovery")
	}
}

// A network or server failure keeps the stored session and reports the
// conservative unverified state, so the page never claims a login it could
// not confirm.
func TestRestoreSessionNetworkErrorKeepsSession(t *testing.T) {
	store := seedStore("user-1", "ada@example.com")
	sessions := &fakeSessionStore{has: true, rec: sessionRecord{
		UserID: "user-1", Email: "ada@example.com", ServerURL: defaultServerURL, RefreshToken: "r-1",
	}}
	refresh := fixedRefresher(auth.TokenResponse{}, fmt.Errorf("%w: dial tcp", auth.ErrNetwork))
	a := newRealAccountWith(store, &fakeVerifier{result: hello.Verified}, fixedFlow(&scriptedAuth{}), sessions, refresh)

	sess, err := a.restoreSession()
	if !errors.Is(err, errSessionUnverified) {
		t.Fatalf("restoreSession error = %v, want errSessionUnverified", err)
	}
	if sess.state != loginLoggedOut {
		t.Fatalf("session = %+v, want the conservative signed-out state", sess)
	}
	if !sessions.has {
		t.Fatal("an unreachable server must not clear the stored session")
	}
	if len(sessions.clears) != 0 {
		t.Fatalf("clears = %v, want none", sessions.clears)
	}
}

// With nothing stored, a restore is a no-op that reports the signed-out state
// and never calls the network.
func TestRestoreSessionWithoutStoredSession(t *testing.T) {
	called := false
	refresh := func(context.Context, string, string) (auth.TokenResponse, error) {
		called = true
		return auth.TokenResponse{}, nil
	}
	a := newRealAccountWith(newFakeStore(), &fakeVerifier{result: hello.Verified},
		fixedFlow(&scriptedAuth{}), &fakeSessionStore{}, refresh)

	sess, err := a.restoreSession()
	if err != nil {
		t.Fatalf("restoreSession: %v", err)
	}
	if sess.state != loginLoggedOut {
		t.Fatalf("session = %+v, want signed out", sess)
	}
	if called {
		t.Fatal("restore dialed the server with no stored session")
	}
}

// Logout clears the persisted session together with the key it belongs to.
func TestLogoutClearsStoredSession(t *testing.T) {
	store := newFakeStore()
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	flow.step.refreshToken = "r-1"
	sessions := &fakeSessionStore{}
	a := newRealAccountWith(store, &fakeVerifier{result: hello.Verified}, fixedFlow(flow), sessions, nil)

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := a.logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if store.removeCalls != 1 {
		t.Fatalf("Remove calls = %d, want 1", store.removeCalls)
	}
	if sessions.has {
		t.Fatal("logout left the persisted session behind")
	}
	if len(sessions.clears) != 1 || sessions.clears[0] != "user-1" {
		t.Fatalf("clears = %v, want user-1", sessions.clears)
	}
}

// The account card's three states are chosen from the session and the stored
// key, with the wording the ticket asks for.
func TestAccountCardStates(t *testing.T) {
	row := &accountRow{userID: "user-1", email: "ada@example.com", created: "2026-01-12 09:24"}

	signedOut := accountCardViewFor(accountSession{state: loginLoggedOut, serverURL: defaultServerURL}, nil)
	if signedOut.state != cardSignedOut || signedOut.title != "尚未登录" {
		t.Fatalf("signed-out view = %+v", signedOut)
	}
	if signedOut.loginLabel != "登录 Bitwarden 账号" {
		t.Fatalf("signed-out button = %q", signedOut.loginLabel)
	}

	configured := accountCardViewFor(accountSession{state: loginLoggedOut, serverURL: defaultServerURL}, row)
	if configured.state != cardConfigured || configured.title != "已配置" {
		t.Fatalf("configured view = %+v", configured)
	}
	for _, want := range []string{"ada@example.com", "2026-01-12 09:24", "解锁可用"} {
		if !strings.Contains(configured.detail, want) {
			t.Fatalf("configured detail = %q, want it to contain %q", configured.detail, want)
		}
	}
	if configured.loginLabel != "重新登录（用于刷新密钥）" {
		t.Fatalf("configured button = %q", configured.loginLabel)
	}

	signedIn := accountCardViewFor(loggedInSession(), row)
	if signedIn.state != cardSignedIn || signedIn.title != "已登录" {
		t.Fatalf("signed-in view = %+v", signedIn)
	}
	for _, want := range []string{"ada@example.com", defaultServerURL, "2026-01-12 09:24"} {
		if !strings.Contains(signedIn.detail, want) {
			t.Fatalf("signed-in detail = %q, want it to contain %q", signedIn.detail, want)
		}
	}
	if !signedIn.logout {
		t.Fatal("a signed-in card must offer logout")
	}
}

// The first reload revalidates the persisted session; a later reload only
// reads the in-memory session, so a save or logout never dials the server.
func TestReloadRevalidatesOnlyOnce(t *testing.T) {
	acct := &stubAccount{sess: loggedInSession()}
	s := newAccountStateWith(fakeKeys{}, acct)
	drain(t, &s)

	if acct.restoreCalls != 1 || acct.sessionCalls != 0 {
		t.Fatalf("first load: restore %d, session %d, want 1/0", acct.restoreCalls, acct.sessionCalls)
	}
	s.reload()
	drain(t, &s)
	if acct.restoreCalls != 1 || acct.sessionCalls != 1 {
		t.Fatalf("second load: restore %d, session %d, want 1/1", acct.restoreCalls, acct.sessionCalls)
	}
}

// A revoked session lands on the configured state with a notice that names it.
func TestAccountStateShowsConfiguredAfterRevokedSession(t *testing.T) {
	acct := &stubAccount{restoreSet: true, restoreSess: loggedOutSession(), restoreErr: errSessionRevoked}
	s := newAccountStateWith(fakeKeys{}, acct)
	drain(t, &s)

	if s.session.state != loginLoggedOut {
		t.Fatalf("session = %+v, want signed out", s.session)
	}
	if !strings.Contains(s.notice, "登录会话已失效") {
		t.Fatalf("notice = %q, want the revoked-session wording", s.notice)
	}
	if v := accountCardViewFor(s.session, s.primary()); v.state != cardConfigured {
		t.Fatalf("card state = %v, want configured (the key is still stored)", v.state)
	}
}

// An unreachable server keeps the session and shows the conservative
// unverified notice instead of claiming a login.
func TestAccountStateShowsUnverifiedNotice(t *testing.T) {
	acct := &stubAccount{restoreSet: true, restoreSess: loggedOutSession(), restoreErr: errSessionUnverified}
	s := newAccountStateWith(fakeKeys{}, acct)
	drain(t, &s)

	if s.session.state != loginLoggedOut {
		t.Fatalf("session = %+v, want signed out", s.session)
	}
	if !strings.Contains(s.notice, "无法验证登录会话") {
		t.Fatalf("notice = %q, want the unverified wording", s.notice)
	}
}
