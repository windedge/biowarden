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
	"github.com/windedge/biowarden/internal/session"
	"github.com/windedge/biowarden/locales"
)

// seedStore puts a key straight into the in-memory store, without counting a
// Put, so a restore test can assert it wrote nothing.
func seedStore(userID, email string) *fakeStore {
	store := newFakeStore()
	store.accounts[userID] = secrets.Account{UserID: userID, Email: email}
	store.keys[userID] = []byte("stored-key")
	return store
}

func fixedRefresher(tok auth.TokenResponse, err error) session.Refresher {
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
	sessions := &session.MemoryStore{}
	a := newRealAccountWith(store, verifier, fixedFlow(flow), sessions, nil)

	sess, err := a.login("ada@example.com", "hunter2-password", defaultServerURL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.state != loginLoggedIn {
		t.Fatalf("state = %v, want logged in", sess.state)
	}
	if !sessions.Has {
		t.Fatal("login did not persist a session")
	}
	rec := sessions.Rec
	if rec.UserID != "user-1" || rec.Email != "ada@example.com" ||
		rec.ServerURL != defaultServerURL || rec.RefreshToken != "refresh-1" {
		t.Fatalf("persisted record = %+v", rec)
	}
	blob, err := session.Encode(rec)
	if err != nil {
		t.Fatalf("session.Encode: %v", err)
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
	sessions := &session.MemoryStore{SaveErr: errors.New("credential write failed")}
	a := newRealAccountWith(store, verifier, fixedFlow(flow), sessions, nil)

	sess, err := a.login("ada@example.com", "pw", defaultServerURL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.state != loginLoggedIn {
		t.Fatalf("state = %v, want logged in despite the failed session write", sess.state)
	}
	if !errors.Is(sess.warning, errSessionPersist) {
		t.Fatalf("warning = %v, want errSessionPersist", sess.warning)
	}
	if !strings.Contains(sess.warning.Error(), "credential write failed") {
		t.Fatalf("warning = %v, want the store failure as the detail", sess.warning)
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
	sessions := &session.MemoryStore{Has: true, Rec: session.Record{
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
	if sessions.Rec.RefreshToken != "r-2" {
		t.Fatalf("stored refresh token = %q, want the rotated r-2", sessions.Rec.RefreshToken)
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
	sessions := &session.MemoryStore{Has: true, Rec: session.Record{
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
	if sessions.Has {
		t.Fatal("a revoked session must be cleared")
	}
	if len(sessions.Clears) != 1 || sessions.Clears[0] != "user-1" {
		t.Fatalf("clears = %v, want user-1", sessions.Clears)
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
	sessions := &session.MemoryStore{Has: true, Rec: session.Record{
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
	if !sessions.Has {
		t.Fatal("an unreachable server must not clear the stored session")
	}
	if len(sessions.Clears) != 0 {
		t.Fatalf("clears = %v, want none", sessions.Clears)
	}
}

// A slow server that trips the startup client's short deadline arrives as the
// same network error, so the restore falls back to the conservative unverified
// state and keeps the stored session for the next launch.
func TestRestoreSessionTimeoutKeepsSession(t *testing.T) {
	store := seedStore("user-1", "ada@example.com")
	sessions := &session.MemoryStore{Has: true, Rec: session.Record{
		UserID: "user-1", Email: "ada@example.com", ServerURL: defaultServerURL, RefreshToken: "r-1",
	}}
	refresh := func(context.Context, string, string) (auth.TokenResponse, error) {
		// What auth.Client produces once http.Client.Timeout fires.
		return auth.TokenResponse{}, fmt.Errorf("%w: Client.Timeout exceeded while awaiting headers", auth.ErrNetwork)
	}
	a := newRealAccountWith(store, &fakeVerifier{result: hello.Verified}, fixedFlow(&scriptedAuth{}), sessions, refresh)

	sess, err := a.restoreSession()
	if !errors.Is(err, errSessionUnverified) {
		t.Fatalf("restoreSession error = %v, want errSessionUnverified", err)
	}
	if sess.state != loginLoggedOut {
		t.Fatalf("session = %+v, want the conservative signed-out state", sess)
	}
	if !sessions.Has {
		t.Fatal("a timed-out restore must not clear the stored session")
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
		fixedFlow(&scriptedAuth{}), &session.MemoryStore{}, refresh)

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
	sessions := &session.MemoryStore{}
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
	if sessions.Has {
		t.Fatal("logout left the persisted session behind")
	}
	if len(sessions.Clears) != 1 || sessions.Clears[0] != "user-1" {
		t.Fatalf("clears = %v, want user-1", sessions.Clears)
	}
}

// Deleting a stored key must drop the persisted login that belongs to it;
// otherwise a restart would restore the session and show the account as signed
// in with no key.
func TestForgetSessionClearsStoredSession(t *testing.T) {
	sessions := &session.MemoryStore{Has: true, Rec: session.Record{
		UserID: "user-1", Email: "ada@example.com", ServerURL: defaultServerURL, RefreshToken: "r-1",
	}}
	a := newRealAccountWith(newFakeStore(), &fakeVerifier{result: hello.Verified},
		fixedFlow(&scriptedAuth{}), sessions, nil)

	if err := a.forgetSession("user-1"); err != nil {
		t.Fatalf("forgetSession: %v", err)
	}
	if sessions.Has {
		t.Fatal("forgetSession left the persisted session behind")
	}
	if len(sessions.Clears) != 1 || sessions.Clears[0] != "user-1" {
		t.Fatalf("clears = %v, want user-1", sessions.Clears)
	}
}

// Deleting a stored key also resets the in-memory session when it belongs to
// the deleted account: otherwise the card keeps saying "signed in" for a key
// that no longer exists. It must not touch another account's session.
func TestForgetSessionResetsTheInMemorySession(t *testing.T) {
	store := seedStore("user-1", "ada@example.com")
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	flow.step.refreshToken = "r-1"
	sessions := &session.MemoryStore{}
	a := newRealAccountWith(store, &fakeVerifier{result: hello.Verified}, fixedFlow(flow), sessions, nil)
	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := a.forgetSession("user-2"); err != nil {
		t.Fatalf("forgetSession(other): %v", err)
	}
	if sess, _ := a.session(); sess.state != loginLoggedIn || sess.userID != "user-1" {
		t.Fatalf("forgetting another account's session reset the login: %+v", sess)
	}

	if err := a.forgetSession("user-1"); err != nil {
		t.Fatalf("forgetSession: %v", err)
	}
	sess, err := a.session()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if sess.state != loginLoggedOut || sess.userID != "" || sess.email != "" ||
		sess.serverURL != defaultServerURL {
		t.Fatalf("session after forgetSession = %+v, want signed out", sess)
	}
	// Abandoning an unrelated attempt must not revive the deleted account.
	if _, err := a.cancelLogin(); err != nil {
		t.Fatalf("cancelLogin: %v", err)
	}
	if sess, _ := a.session(); sess.state != loginLoggedOut || sess.userID != "" {
		t.Fatalf("cancelLogin revived the deleted session: %+v", sess)
	}
}

// Removing a key from the account page leaves the backend signed out, so the
// card and the backend agree once the removal lands.
func TestRemoveAccountResetsTheBackendSession(t *testing.T) {
	store := seedStore("user-1", "ada@example.com")
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("secret"))}
	flow.step.refreshToken = "r-1"
	sessions := &session.MemoryStore{}
	verifier := &fakeVerifier{result: hello.Verified}
	refresh := fixedRefresher(auth.TokenResponse{RefreshToken: "r-1"}, nil)
	acct := newRealAccountWith(store, verifier, fixedFlow(flow), sessions, refresh)
	if _, err := acct.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	s := newAccountStateWith(newRealKeysWith(store, verifier), acct)
	waitRestore(t, &s)
	if s.session.state != loginLoggedIn {
		t.Fatalf("session before removal = %+v, want signed in", s.session)
	}

	s.removeAccount(&accountRow{userID: "user-1"})
	drain(t, &s)

	sess, err := acct.session()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if sess.state != loginLoggedOut || sess.userID != "" {
		t.Fatalf("backend session after removal = %+v, want signed out", sess)
	}
	if s.session.state != loginLoggedOut {
		t.Fatalf("page session after removal = %+v, want signed out", s.session)
	}
	if sessions.Has {
		t.Fatal("removal left the persisted session behind")
	}
	if len(s.accounts) != 0 {
		t.Fatalf("accounts = %v, want the removed row gone", s.accounts)
	}
}

// Signing out (behind Hello, from the card) and deleting the key (from the
// advanced list) are two entry points to the same end state: signed out, no
// stored key, no persisted session, no stale row.
func TestLogoutAndRemoveAccountEndInTheSameState(t *testing.T) {
	for _, path := range []string{"logout", "removeAccount"} {
		t.Run(path, func(t *testing.T) {
			store := seedStore("user-1", "ada@example.com")
			flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("secret"))}
			flow.step.refreshToken = "r-1"
			sessions := &session.MemoryStore{}
			verifier := &fakeVerifier{result: hello.Verified}
			acct := newRealAccountWith(store, verifier, fixedFlow(flow), sessions,
				fixedRefresher(auth.TokenResponse{RefreshToken: "r-1"}, nil))
			if _, err := acct.login("ada@example.com", "pw", defaultServerURL); err != nil {
				t.Fatalf("login: %v", err)
			}
			s := newAccountStateWith(newRealKeysWith(store, verifier), acct)
			waitRestore(t, &s)
			if s.session.state != loginLoggedIn {
				t.Fatalf("session before %s = %+v, want signed in", path, s.session)
			}

			switch path {
			case "logout":
				s.logout()
			case "removeAccount":
				s.removeAccount(&accountRow{userID: "user-1"})
			}
			drain(t, &s)

			if sess, _ := acct.session(); sess.state != loginLoggedOut || sess.userID != "" {
				t.Fatalf("%s left the backend session as %+v", path, sess)
			}
			if s.session.state != loginLoggedOut {
				t.Fatalf("%s left the page session as %+v", path, s.session)
			}
			if sessions.Has {
				t.Fatalf("%s left the persisted session behind", path)
			}
			if rows, err := store.List(); err != nil || len(rows) != 0 {
				t.Fatalf("%s left the stored key behind: %v %v", path, rows, err)
			}
			if len(s.accounts) != 0 {
				t.Fatalf("%s left the stored row behind: %v", path, s.accounts)
			}
		})
	}
}

// Removing a stored key goes through the account backend's session cleanup, so
// the key and its persisted session cannot outlive each other.
func TestRemoveAccountClearsStoredSession(t *testing.T) {
	store := newFakeStore()
	if err := store.Put(secrets.Account{UserID: "user-1", Email: "ada@example.com"}, secrets.Key("secret")); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	acct := &stubAccount{}
	s := newAccountStateWith(newRealKeysWith(store, &fakeVerifier{result: hello.Verified}), acct)
	waitRestore(t, &s)

	s.removeAccount(&accountRow{userID: "user-1"})
	drain(t, &s)

	if store.removeCalls != 1 {
		t.Fatalf("Remove calls = %d, want 1", store.removeCalls)
	}
	if len(acct.forgotten) != 1 || acct.forgotten[0] != "user-1" {
		t.Fatalf("forgetSession calls = %v, want user-1", acct.forgotten)
	}
	if len(s.accounts) != 0 {
		t.Fatalf("accounts = %v, want the removed row gone", s.accounts)
	}
}

// When the key removal fails, the session must be left alone: nothing was
// deleted, so the account is still signed in.
func TestRemoveAccountKeepsSessionWhenKeyRemovalFails(t *testing.T) {
	store := newFakeStore()
	store.removeErr = errors.New("locked")
	if err := store.Put(secrets.Account{UserID: "user-1"}, secrets.Key("secret")); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	acct := &stubAccount{}
	s := newAccountStateWith(newRealKeysWith(store, &fakeVerifier{result: hello.Verified}), acct)
	waitRestore(t, &s)

	s.removeAccount(&accountRow{userID: "user-1"})
	drain(t, &s)

	if len(acct.forgotten) != 0 {
		t.Fatalf("forgetSession calls = %v, want none after a failed removal", acct.forgotten)
	}
	if len(s.accounts) != 1 {
		t.Fatalf("accounts = %v, want the row kept after a failed removal", s.accounts)
	}
}

// The account card's three states are chosen from the session and the stored
// key, with the wording the ticket asks for.
func TestAccountCardStates(t *testing.T) {
	appLanguage(t, "zh-Hans")
	row := &accountRow{userID: "user-1", email: "ada@example.com", created: "2026-01-12 09:24", serverURL: defaultServerURL}

	signedOut := accountCardViewFor(accountSession{state: loginLoggedOut, serverURL: defaultServerURL}, nil)
	if signedOut.state != cardSignedOut || signedOut.title != locales.AccountCardSignedOut() {
		t.Fatalf("signed-out view = %+v", signedOut)
	}
	if signedOut.loginLabel != locales.LoginTitle() {
		t.Fatalf("signed-out button = %q", signedOut.loginLabel)
	}
	if signedOut.removable {
		t.Fatal("a card without a stored key must not offer sign-out")
	}

	configured := accountCardViewFor(accountSession{state: loginLoggedOut, serverURL: defaultServerURL}, row)
	if configured.state != cardConfigured || configured.title != locales.AccountCardConfigured() {
		t.Fatalf("configured view = %+v", configured)
	}
	for _, want := range []string{"ada@example.com", defaultServerURL, "2026-01-12 09:24",
		locales.AccountCardConfiguredDetail("ada@example.com", defaultServerURL, "2026-01-12 09:24")} {
		if !strings.Contains(configured.detail, want) {
			t.Fatalf("configured detail = %q, want it to contain %q", configured.detail, want)
		}
	}
	if configured.loginLabel != "" {
		t.Fatalf("configured view must offer no sign-in button: label=%q", configured.loginLabel)
	}
	if !configured.removable {
		t.Fatal("a configured card with a stored key must offer sign-out")
	}

	signedIn := accountCardViewFor(loggedInSession(), row)
	if signedIn.state != cardSignedIn || signedIn.title != locales.AccountCardSignedIn() {
		t.Fatalf("signed-in view = %+v", signedIn)
	}
	for _, want := range []string{"ada@example.com", defaultServerURL, "2026-01-12 09:24"} {
		if !strings.Contains(signedIn.detail, want) {
			t.Fatalf("signed-in detail = %q, want it to contain %q", signedIn.detail, want)
		}
	}
	if signedIn.loginLabel != "" {
		t.Fatalf("signed-in view with a stored key must offer no sign-in button: label=%q", signedIn.loginLabel)
	}
	if !signedIn.removable {
		t.Fatal("a signed-in card with a stored key must offer sign-out")
	}

	noKey := accountCardViewFor(loggedInSession(), nil)
	if noKey.removable || noKey.detail != locales.AccountCardSignedInNoKey("ada@example.com", defaultServerURL) {
		t.Fatalf("signed-in without a key = %+v", noKey)
	}
	// A signed-in session whose key is gone must still offer a way out: the
	// login window re-derives the key.
	if noKey.loginLabel != locales.LoginTitle() || noKey.state != cardSignedIn {
		t.Fatalf("signed-in without a key offers no sign-in: %+v", noKey)
	}
}

// The first reload revalidates the persisted session; a later reload only
// reads the in-memory session, so a save or logout never dials the server.
func TestReloadRevalidatesOnlyOnce(t *testing.T) {
	acct := &stubAccount{sess: loggedInSession()}
	s := newAccountStateWith(fakeKeys{}, acct)
	waitRestore(t, &s)

	if acct.restoreCalls != 1 {
		t.Fatalf("first load: restore %d, want 1", acct.restoreCalls)
	}
	if s.session.state != loginLoggedIn || s.verifying {
		t.Fatalf("session = %+v verifying=%v, want the settled login", s.session, s.verifying)
	}

	// A later reload re-reads only the in-memory session: it must not dial
	// the server again.
	s.reload()
	if acct.restoreCalls != 1 {
		t.Fatalf("second load: restore %d, want still 1", acct.restoreCalls)
	}
	if s.verifying {
		t.Fatal("a later reload must not re-enter verifying")
	}
}

// A revoked session lands on the configured state with a notice that names it.
func TestAccountStateShowsConfiguredAfterRevokedSession(t *testing.T) {
	acct := &stubAccount{restoreSet: true, restoreSess: loggedOutSession(), restoreErr: errSessionRevoked}
	s := newAccountStateWith(fakeKeys{}, acct)
	waitRestore(t, &s)

	if s.session.state != loginLoggedOut {
		t.Fatalf("session = %+v, want signed out", s.session)
	}
	if !strings.Contains(s.notice.text(), locales.AccountNoticeSessionRevoked()) {
		t.Fatalf("notice = %q, want the revoked-session wording", s.notice.text())
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
	waitRestore(t, &s)

	if s.session.state != loginLoggedOut {
		t.Fatalf("session = %+v, want signed out", s.session)
	}
	if !strings.Contains(s.notice.text(), locales.AccountNoticeSessionUnverified()) {
		t.Fatalf("notice = %q, want the unverified wording", s.notice.text())
	}
}

// The first reload renders the stored keys before the restore answers: local
// data is read on the UI thread, the persisted login's re-check runs in the
// background and lands only on the session fields, never on the rows already
// on screen.
func TestReloadShowsLocalStateBeforeSlowRestore(t *testing.T) {
	store := seedStore("user-1", "ada@example.com")
	acct := &stubAccount{
		// The in-memory session is still the fresh install's; only the
		// persisted login restoreSession will produce a login.
		sess:       loggedOutSession(),
		restoreSet: true, restoreSess: loggedInSession(),
		restoreGate: make(chan struct{}),
	}
	s := newAccountStateWith(newRealKeysWith(store, &fakeVerifier{result: hello.Verified}), acct)

	// The synchronous first reload: the stored row is already on the card
	// while the restore is still held on the wire.
	if len(s.accounts) != 1 || s.accounts[0].userID != "user-1" {
		t.Fatalf("accounts = %+v, want the stored row without waiting", s.accounts)
	}
	if s.session.state != loginLoggedOut || !s.verifying {
		t.Fatalf("session = %+v verifying=%v, want logged-out while verifying", s.session, s.verifying)
	}

	// The restore settles on its own lane: the session lands and the local
	// rows stay put.
	close(acct.restoreGate)
	waitRestore(t, &s)
	if s.verifying {
		t.Fatal("the settled restore did not clear verifying")
	}
	if s.session.state != loginLoggedIn || s.session.userID != "user-1" {
		t.Fatalf("session = %+v, want the restored login", s.session)
	}
	if len(s.accounts) != 1 || s.accounts[0].userID != "user-1" {
		t.Fatalf("accounts = %+v, want the local rows untouched", s.accounts)
	}
}

// A reload that lands while the startup restore is still on the wire reuses
// the in-flight check instead of starting a second one.
func TestReloadWhileVerifyingDoesNotDoubleRestore(t *testing.T) {
	acct := &stubAccount{sess: loggedOutSession(), restoreGate: make(chan struct{})}
	s := newAccountStateWith(fakeKeys{}, acct)
	s.reload() // the startup restore is still on the wire
	close(acct.restoreGate)
	waitRestore(t, &s)

	if acct.restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want exactly one", acct.restoreCalls)
	}
	if s.verifying {
		t.Fatal("the restore did not settle")
	}
}
