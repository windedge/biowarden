package ui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/secrets"
)

// scriptedAuth is an accountAuth whose one password step is fixed by the test.
// The same instance is returned for every attempt, so a refresh sees the
// mutated step.
type scriptedAuth struct {
	startErr  error
	step      authStep
	stepErr   error
	starts    int
	passwords []string
}

func (f *scriptedAuth) start(context.Context, string) error {
	f.starts++
	return f.startErr
}

func (f *scriptedAuth) password(_ context.Context, password string) (authStep, error) {
	f.passwords = append(f.passwords, password)
	return f.step, f.stepErr
}

// fixedFlow hands the same scripted flow to every attempt.
func fixedFlow(f *scriptedAuth) accountAuthFactory {
	return func(string) (accountAuth, error) { return f, nil }
}

func doneStep(userID, email string, key []byte) authStep {
	return authStep{
		userKey: key,
		account: secrets.Account{UserID: userID, Email: email},
	}
}

func TestRealAccountLoginWritesKeyAfterHello(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	key := []byte{0x00, 0x01, 0xff, 0x10}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", key)}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	sess, err := a.login("  ada@example.com ", "pw", defaultServerURL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.state != loginLoggedIn {
		t.Fatalf("state = %v, want logged in", sess.state)
	}
	if sess.userID != "user-1" || sess.email != "ada@example.com" {
		t.Fatalf("session = %+v", sess)
	}
	if store.putCalls != 1 {
		t.Fatalf("Put calls = %d, want 1", store.putCalls)
	}
	if got := store.keys["user-1"]; !bytes.Equal(got, key) {
		t.Fatalf("stored key = %x, want %x", got, key)
	}
	if verifier.calls != 1 {
		t.Fatalf("Verify calls = %d, want 1", verifier.calls)
	}
}

// A refused Hello prompt must leave the store untouched: the consent comes
// before Put.
func TestRealAccountLoginHelloRefusedDoesNotWrite(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Canceled}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	sess, err := a.login("ada@example.com", "pw", defaultServerURL)
	if err == nil {
		t.Fatal("login with canceled consent = nil, want error")
	}
	if store.putCalls != 0 {
		t.Fatalf("Put calls = %d, want 0", store.putCalls)
	}
	if sess.state == loginLoggedIn {
		t.Fatal("session reported a login despite the refused consent")
	}
}

// A 2FA stop is not a success: it keeps the challenge for the prompt and
// writes no key.
func TestRealAccountLoginTwoFactorDoesNotWrite(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: authStep{
		needs2FA:  true,
		challenge: &twoFactorPrompt{providers: []string{"authenticator"}, email: "ada@example.com"},
	}}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	sess, err := a.login("ada@example.com", "pw", defaultServerURL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.state != loginNeeds2FA {
		t.Fatalf("state = %v, want needs 2FA", sess.state)
	}
	if sess.challenge == nil || len(sess.challenge.providers) != 1 || sess.challenge.providers[0] != "authenticator" {
		t.Fatalf("challenge = %+v", sess.challenge)
	}
	if sess.twoFAMethod != "authenticator" {
		t.Fatalf("twoFAMethod = %q", sess.twoFAMethod)
	}
	if store.putCalls != 0 || verifier.calls != 0 {
		t.Fatalf("Put calls = %d, Verify calls = %d, want 0/0", store.putCalls, verifier.calls)
	}
}

// The auth sentinels must reach the page as actionable wording, not raw
// error text. The adapter is what maps them, so the mapping is tested here
// directly.
func TestFriendlyAuthErrorMapsSentinels(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{auth.ErrUnauthorized, "邮箱或主密码不正确"},
		{auth.ErrForbidden, "服务器拒绝登录"},
		{auth.ErrCaptchaRequired, "人机验证"},
		{auth.ErrNetwork, "无法连接服务器"},
		{auth.ErrServer, "服务器返回错误"},
		{auth.ErrTwoFactorRequired, "两步验证"},
	}
	for _, tc := range cases {
		if got := friendlyAuthError(tc.err).Error(); !strings.Contains(got, tc.want) {
			t.Errorf("friendlyAuthError(%v) = %q, want it to contain %q", tc.err, got, tc.want)
		}
	}
	// An unknown error passes through unchanged.
	other := errors.New("login failed hard")
	if got := friendlyAuthError(other); got != other {
		t.Errorf("friendlyAuthError(unknown) = %v, want the same error", got)
	}
}

// A failure from the flow must surface to the page and leave the store
// untouched.
func TestRealAccountLoginAuthErrorSurfaces(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{stepErr: errors.New("login failed hard")}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	_, err := a.login("ada@example.com", "pw", defaultServerURL)
	if err == nil || !strings.Contains(err.Error(), "login failed hard") {
		t.Fatalf("error = %v, want the flow error", err)
	}
	if store.putCalls != 0 {
		t.Fatalf("Put calls = %d, want 0", store.putCalls)
	}
}

// A store failure after consent must surface, not be swallowed, and must not
// leave the session claiming a successful login.
func TestRealAccountLoginStoreErrorSurfaces(t *testing.T) {
	store := newFakeStore()
	store.putErr = errors.New("vault full")
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	sess, err := a.login("ada@example.com", "pw", defaultServerURL)
	if err == nil || !strings.Contains(err.Error(), "保存密钥失败") {
		t.Fatalf("error = %v, want the wrapped store failure", err)
	}
	if sess.state == loginLoggedIn {
		t.Fatal("session reported a login despite the failed write")
	}
}

func TestRealAccountLogoutDeletesKeyAndIsIdempotent(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := a.logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if store.removeCalls != 1 {
		t.Fatalf("Remove calls = %d, want 1", store.removeCalls)
	}
	if _, ok := store.keys["user-1"]; ok {
		t.Fatal("key survived logout")
	}
	sess, _ := a.session()
	if sess.state != loginLoggedOut {
		t.Fatalf("state = %v, want logged out", sess.state)
	}
	if sess.serverURL != defaultServerURL {
		t.Fatalf("serverURL = %q, want the default", sess.serverURL)
	}
	// A second logout has nothing to delete and must not prompt again.
	before := verifier.calls
	if err := a.logout(); err != nil {
		t.Fatalf("second logout: %v", err)
	}
	if store.removeCalls != 1 || verifier.calls != before {
		t.Fatalf("second logout ran Remove(%d) or Verify(%d)", store.removeCalls, verifier.calls-before)
	}
}

// Refusing the Hello prompt must keep the account signed in and the key in
// place, so a locked device cannot be used to erase keys silently.
func TestRealAccountLogoutRefusedKeepsSession(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	verifier.result = hello.Locked
	if err := a.logout(); err == nil {
		t.Fatal("logout with locked consent = nil, want error")
	}
	if store.removeCalls != 0 {
		t.Fatalf("Remove calls = %d, want 0", store.removeCalls)
	}
	sess, _ := a.session()
	if sess.state != loginLoggedIn {
		t.Fatalf("state = %v, want still logged in", sess.state)
	}
}

// refreshKey re-derives the key from the in-memory credentials and rewrites
// the stored copy after a fresh consent.
func TestRealAccountRefreshRewritesKey(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("old-key"))}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	flow.step = doneStep("user-1", "ada@example.com", []byte("new-key"))
	if err := a.refreshKey(); err != nil {
		t.Fatalf("refreshKey: %v", err)
	}
	if got := store.keys["user-1"]; !bytes.Equal(got, []byte("new-key")) {
		t.Fatalf("stored key = %q, want new-key", got)
	}
	if store.putCalls != 2 {
		t.Fatalf("Put calls = %d, want 2", store.putCalls)
	}
	if verifier.calls != 2 {
		t.Fatalf("Verify calls = %d, want 2 (one per write)", verifier.calls)
	}
	if len(flow.passwords) != 2 {
		t.Fatalf("password steps = %d, want 2", len(flow.passwords))
	}
}

// Without a cached login (the app was restarted) refreshing cannot re-derive
// the key, and says so instead of silently doing nothing.
func TestRealAccountRefreshWithoutCredentials(t *testing.T) {
	a := newRealAccount(newFakeStore(), &fakeVerifier{result: hello.Verified}, fixedFlow(&scriptedAuth{}))
	if err := a.refreshKey(); !errors.Is(err, errNeedCredentials) {
		t.Fatalf("refreshKey error = %v, want errNeedCredentials", err)
	}
	if err := a.relogin(); !errors.Is(err, errNeedCredentials) {
		t.Fatalf("relogin error = %v, want errNeedCredentials", err)
	}
}

// A refresh that stops for 2FA must not report success: the stored key was not
// rewritten.
func TestRealAccountRefreshTwoFactorIsNotSuccess(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("old-key"))}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	flow.step = authStep{needs2FA: true, challenge: &twoFactorPrompt{providers: []string{"authenticator"}}}
	if err := a.refreshKey(); !errors.Is(err, errTwoFactorUnsupported) {
		t.Fatalf("refreshKey error = %v, want errTwoFactorUnsupported", err)
	}
	if got := store.keys["user-1"]; !bytes.Equal(got, []byte("old-key")) {
		t.Fatalf("stored key = %q, want the unchanged old-key", got)
	}
}

// submit2FA belongs to the login window ticket; it must report that plainly.
func TestRealAccountSubmit2FAIsNotImplemented(t *testing.T) {
	a := newRealAccount(newFakeStore(), &fakeVerifier{result: hello.Verified}, fixedFlow(&scriptedAuth{}))
	if _, err := a.submit2FA("123456"); !errors.Is(err, errTwoFactorUnsupported) {
		t.Fatalf("submit2FA error = %v, want errTwoFactorUnsupported", err)
	}
}

func TestRealAccountRejectsInvalidServerURL(t *testing.T) {
	store := newFakeStore()
	a := newRealAccount(store, &fakeVerifier{result: hello.Verified}, newAuthFlowFactory())
	if _, err := a.login("ada@example.com", "pw", "http://bad host"); err == nil {
		t.Fatal("login with an invalid server URL = nil, want error")
	}
	if store.putCalls != 0 {
		t.Fatalf("Put calls = %d, want 0", store.putCalls)
	}
}

// The stored-account list is the source of truth for the multi-account view:
// every key in the store must show up, with its metadata.
func TestRealKeysListsAllStoredAccounts(t *testing.T) {
	store := newFakeStore()
	store.Put(secrets.Account{UserID: "user-1", Email: "ada@example.com"}, secrets.Key("k1"))
	store.Put(secrets.Account{UserID: "user-2", Email: "grace@example.com"}, secrets.Key("k2"))
	k := newRealKeysWith(store, &fakeVerifier{available: true})

	rows, err := k.accounts()
	if err != nil {
		t.Fatalf("accounts: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	seen := map[string]string{}
	for _, r := range rows {
		seen[r.userID] = r.email
	}
	if seen["user-1"] != "ada@example.com" || seen["user-2"] != "grace@example.com" {
		t.Fatalf("rows = %+v", rows)
	}
}

// The account card can switch which stored account the key card describes;
// the sign-in session is not touched, because there are no parallel sessions.
func TestAccountStatePrimaryFollowsSelection(t *testing.T) {
	s := newAccountStateWith(fakeKeys{}, fakeAccount{})
	drain(t, &s)

	if got := s.primary(); got == nil || got.email != "ada@example.com" {
		t.Fatalf("primary = %+v, want the first stored account", got)
	}
	s.selectAccount("b27ae6f80c1d4a53b6e9d7c240f8a1b5")
	if got := s.primary(); got == nil || got.email != "grace@example.com" {
		t.Fatalf("primary after select = %+v, want grace", got)
	}
	// A stale selection is dropped when the next reload no longer has it.
	s.applyReload([]accountRow{{userID: "other", email: "other@example.com"}}, nil, true, nil, accountSession{state: loginLoggedOut, serverURL: defaultServerURL}, nil)
	if s.selectedUserID != "" {
		t.Fatalf("selectedUserID = %q, want cleared", s.selectedUserID)
	}
}
