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

// scriptedAuth is an accountAuth whose steps are fixed by the test.
type scriptedAuth struct {
	startErr error
	step     authStep
	stepErr  error
	starts   int

	// solveStep and solveErr answer solveTwoFactor, and sendErr answers the
	// email preparation. Both record what they were asked, because the backend
	// must pass the window's provider and token through unchanged.
	solveStep  authStep
	solveErr   error
	solveCalls int
	providers  []int
	tokens     []string
	sendErr    error
	sendCalls  int
}

func (f *scriptedAuth) start(context.Context, string) error {
	f.starts++
	return f.startErr
}

func (f *scriptedAuth) password(_ context.Context, _ string) (authStep, error) {
	return f.step, f.stepErr
}

func (f *scriptedAuth) solveTwoFactor(_ context.Context, provider int, token string) (authStep, error) {
	f.solveCalls++
	f.providers = append(f.providers, provider)
	f.tokens = append(f.tokens, token)
	return f.solveStep, f.solveErr
}

func (f *scriptedAuth) sendTwoFactorEmail(context.Context) error {
	f.sendCalls++
	return f.sendErr
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
		challenge: codeChallenge(twoFactorAuthenticator, "authenticator"),
	}}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	sess, err := a.login("ada@example.com", "pw", defaultServerURL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.state != loginNeeds2FA {
		t.Fatalf("state = %v, want needs 2FA", sess.state)
	}
	if sess.challenge == nil || len(sess.challenge.methods) != 1 || sess.challenge.methods[0].name != "authenticator" {
		t.Fatalf("challenge = %+v", sess.challenge)
	}
	if sess.challenge.methods[0].provider != twoFactorAuthenticator || !sess.challenge.methods[0].takesCode() {
		t.Fatalf("method = %+v, want the code method", sess.challenge.methods[0])
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

// submit2FA finishes what login parked: the flow resubmits the grant, and the
// key is written the same way as in a password login, behind the same consent.
func TestRealAccountSubmit2FAWritesKeyAfterHello(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	key := []byte{0x10, 0x20, 0x30}
	flow := &scriptedAuth{
		step:      authStep{needs2FA: true, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")},
		solveStep: doneStep("user-1", "ada@example.com", key),
	}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	sess, err := a.submit2FA(twoFactorAuthenticator, " 123456 ")
	if err != nil {
		t.Fatalf("submit2FA: %v", err)
	}
	if sess.state != loginLoggedIn || sess.userID != "user-1" {
		t.Fatalf("session = %+v, want the logged-in session", sess)
	}
	if flow.solveCalls != 1 || flow.providers[0] != twoFactorAuthenticator || flow.tokens[0] != "123456" {
		t.Fatalf("solve call = (%v, %q) x%d, want the trimmed token", flow.providers, flow.tokens, flow.solveCalls)
	}
	if store.putCalls != 1 || !bytes.Equal(store.keys["user-1"], key) {
		t.Fatalf("Put calls = %d key = %x, want the key stored once", store.putCalls, store.keys["user-1"])
	}
	if verifier.calls != 1 {
		t.Fatalf("Verify calls = %d, want the one write's consent", verifier.calls)
	}
	// The parked attempt is gone: a second submission has nothing to answer.
	if _, err := a.submit2FA(twoFactorAuthenticator, "123456"); !errors.Is(err, errNoTwoFactorChallenge) {
		t.Fatalf("second submit2FA = %v, want errNoTwoFactorChallenge", err)
	}
}

// Refusing the Hello prompt must leave the store untouched, and must keep the
// challenge parked so the human can answer it again.
func TestRealAccountSubmit2FAHelloRefusedDoesNotWrite(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Canceled}
	flow := &scriptedAuth{
		step:      authStep{needs2FA: true, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")},
		solveStep: doneStep("user-1", "ada@example.com", []byte("key")),
	}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	sess, err := a.submit2FA(twoFactorAuthenticator, "123456")
	if err == nil {
		t.Fatal("submit2FA with a canceled consent = nil, want an error")
	}
	if store.putCalls != 0 {
		t.Fatalf("Put calls = %d, want 0", store.putCalls)
	}
	if sess.state != loginNeeds2FA {
		t.Fatalf("session = %+v, want the challenge still parked", sess)
	}
	if _, err := a.submit2FA(twoFactorAuthenticator, "123456"); err == nil {
		t.Fatal("the parked challenge was dropped by the refused consent")
	}
}

// Duo and WebAuthn are refused before the flow is touched: biowarden has no
// browser path for them, and opening one would leave the answer unreachable.
func TestRealAccountSubmit2FAUnsupportedMethod(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	challenge := &twoFactorPrompt{methods: []twoFactorMethod{
		{provider: twoFactorDuo, name: "duo", interaction: interactionBrowser, browserURL: "https://duo.example/authorize"},
	}}
	flow := &scriptedAuth{step: authStep{needs2FA: true, challenge: challenge}}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := a.submit2FA(twoFactorDuo, "code|state"); !errors.Is(err, errTwoFactorUnsupported) {
		t.Fatalf("submit2FA(duo) = %v, want errTwoFactorUnsupported", err)
	}
	if flow.solveCalls != 0 {
		t.Fatalf("solve calls = %d, want 0: the browser path must not start", flow.solveCalls)
	}
	if store.putCalls != 0 || verifier.calls != 0 {
		t.Fatalf("Put calls = %d, Verify calls = %d, want 0/0", store.putCalls, verifier.calls)
	}
}

// A provider the server never offered, and a submission with nothing parked,
// must both be refused instead of guessing.
func TestRealAccountSubmit2FANeedsAnOfferedParkedChallenge(t *testing.T) {
	a := newRealAccount(newFakeStore(), &fakeVerifier{result: hello.Verified},
		fixedFlow(&scriptedAuth{step: authStep{needs2FA: true, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")}}))

	if _, err := a.submit2FA(twoFactorAuthenticator, "123456"); !errors.Is(err, errNoTwoFactorChallenge) {
		t.Fatalf("submit2FA with nothing parked = %v, want errNoTwoFactorChallenge", err)
	}
	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := a.submit2FA(twoFactorYubiKey, "otp"); err == nil {
		t.Fatal("submit2FA with an unoffered provider = nil, want an error")
	}
	if _, err := a.submit2FA(twoFactorAuthenticator, "   "); !errors.Is(err, errTwoFactorEmptyCode) {
		t.Fatalf("submit2FA with a blank code = %v, want errTwoFactorEmptyCode", err)
	}
	if _, err := a.submit2FA(twoFactorAuthenticator, "123456"); err != nil {
		t.Fatalf("the valid submission after the refusals: %v", err)
	}
}

// A rejected code comes back as another challenge, so the prompt stays up and
// a corrected code can finish the login.
func TestRealAccountSubmit2FARejectedCodeKeepsTheAttempt(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{
		step:      authStep{needs2FA: true, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")},
		solveStep: authStep{needs2FA: true, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")},
	}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	sess, err := a.submit2FA(twoFactorAuthenticator, "000000")
	if err != nil {
		t.Fatalf("a rejected code must not be an error: %v", err)
	}
	if sess.state != loginNeeds2FA {
		t.Fatalf("session = %+v, want needs 2FA", sess)
	}
	if store.putCalls != 0 || verifier.calls != 0 {
		t.Fatalf("Put calls = %d, Verify calls = %d, want 0/0", store.putCalls, verifier.calls)
	}

	flow.solveStep = doneStep("user-1", "ada@example.com", []byte("key"))
	sess, err = a.submit2FA(twoFactorAuthenticator, "123456")
	if err != nil {
		t.Fatalf("corrected submit2FA: %v", err)
	}
	if sess.state != loginLoggedIn || store.putCalls != 1 {
		t.Fatalf("session = %+v Put calls = %d, want the login after the corrected code", sess, store.putCalls)
	}
}

// The email code goes out once per challenge: the second request is refused
// before the flow sees it.
func TestRealAccountSendEmailCodeGoesOutOnce(t *testing.T) {
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: authStep{needs2FA: true, challenge: emailChallenge("ada@example.com")}}
	a := newRealAccount(newFakeStore(), verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := a.sendEmailCode(); err != nil {
		t.Fatalf("sendEmailCode: %v", err)
	}
	if flow.sendCalls != 1 {
		t.Fatalf("send calls = %d, want 1", flow.sendCalls)
	}
	if err := a.sendEmailCode(); err != nil {
		t.Fatalf("second sendEmailCode: %v", err)
	}
	if flow.sendCalls != 1 {
		t.Fatalf("send calls after the second press = %d, want 1", flow.sendCalls)
	}
	// The challenge is still the pending one, so the code that follows submits.
	if sess, _ := a.session(); sess.state != loginNeeds2FA {
		t.Fatalf("state = %v, want needs 2FA", sess.state)
	}
}

// The email method is the only one with a preparation request.
func TestRealAccountSendEmailCodeNeedsTheEmailMethod(t *testing.T) {
	flow := &scriptedAuth{step: authStep{needs2FA: true, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")}}
	a := newRealAccount(newFakeStore(), &fakeVerifier{result: hello.Verified}, fixedFlow(flow))

	if err := a.sendEmailCode(); !errors.Is(err, errNoTwoFactorChallenge) {
		t.Fatalf("sendEmailCode with nothing parked = %v, want errNoTwoFactorChallenge", err)
	}
	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := a.sendEmailCode(); err == nil {
		t.Fatal("sendEmailCode for a TOTP challenge = nil, want an error")
	}
	if flow.sendCalls != 0 {
		t.Fatalf("send calls = %d, want 0", flow.sendCalls)
	}
}

// checkServer is what keeps a bad address from becoming a network error: it
// normalizes the way the login path will, and refuses what that path refuses.
func TestRealAccountCheckServer(t *testing.T) {
	a := newRealAccount(newFakeStore(), &fakeVerifier{result: hello.Verified}, newAuthFlowFactory())

	got, err := a.checkServer("vault.example.com")
	if err != nil {
		t.Fatalf("checkServer: %v", err)
	}
	if got != "https://vault.example.com" {
		t.Fatalf("checkServer = %q, want the normalized address", got)
	}
	// An empty address means the official cloud, as everywhere else.
	got, err = a.checkServer("")
	if err != nil || got != defaultServerURL {
		t.Fatalf("checkServer(empty) = %q, %v, want the default server", got, err)
	}
	if _, err := a.checkServer("http://bad host"); err == nil || !strings.Contains(err.Error(), "服务器地址无效") {
		t.Fatalf("checkServer(bad) = %v, want the validator's refusal", err)
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

// Cancelling a first login must not leave the session parked in the 2FA
// state, or every later reload would revive a form that can never succeed,
// and must drop the flow the window could still submit to.
func TestRealAccountCancelLoginResetsTwoFactorSession(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: authStep{
		needs2FA:  true,
		challenge: codeChallenge(twoFactorAuthenticator, "authenticator"),
	}}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess, _ := a.session(); sess.state != loginNeeds2FA {
		t.Fatalf("state before cancel = %v, want needs 2FA", sess.state)
	}

	sess, err := a.cancelLogin()
	if err != nil {
		t.Fatalf("cancelLogin: %v", err)
	}
	if sess.state != loginLoggedOut || sess.challenge != nil {
		t.Fatalf("cancelled session = %+v, want signed out with no challenge", sess)
	}
	// session() backs every reload; it must stay off the 2FA state.
	again, err := a.session()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if again.state != loginLoggedOut || again.challenge != nil {
		t.Fatalf("session after cancel = %+v, want signed out", again)
	}
	// The parked flow is gone, so a stale window cannot submit into it.
	if _, err := a.submit2FA(twoFactorAuthenticator, "123456"); !errors.Is(err, errNoTwoFactorChallenge) {
		t.Fatalf("submit2FA after cancel = %v, want errNoTwoFactorChallenge", err)
	}
}

// A sign-in started while already signed in, then parked on a 2FA prompt and
// cancelled, must return to the signed-in session instead of leaving the page
// on the abandoned attempt.
func TestRealAccountCancelRestoresSignedInSession(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	a := newRealAccount(store, verifier, fixedFlow(flow))

	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("login: %v", err)
	}
	flow.step = authStep{needs2FA: true, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")}
	if _, err := a.login("ada@example.com", "pw", defaultServerURL); err != nil {
		t.Fatalf("second login: %v", err)
	}

	sess, err := a.cancelLogin()
	if err != nil {
		t.Fatalf("cancelLogin: %v", err)
	}
	if sess.state != loginLoggedIn || sess.userID != "user-1" || sess.email != "ada@example.com" {
		t.Fatalf("session = %+v, want the previous signed-in session", sess)
	}
	if store.removeCalls != 0 {
		t.Fatalf("Remove calls = %d, want 0: cancelling must not touch the store", store.removeCalls)
	}
}
