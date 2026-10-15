package ui

import (
	"errors"
	"strings"
	"testing"
)

// loginFixture drives the login window's state machine without a window
// server: the two GUI seams count their calls, so a test can see the window
// open and close while the state stays the thing under test.
type loginFixture struct {
	login  *loginWindow
	page   *accountState
	acct   *stubAccount
	opened int
	closed int
	// resizes records the height of each resizeWindow call, so a test can see
	// the window follow the form's state without a window server.
	resizes []int
}

func newLoginFixture(t *testing.T, acct *stubAccount) *loginFixture {
	t.Helper()
	page := newAccountStateWith(fakeKeys{}, acct)
	drain(t, &page) // the initial reload the page starts with
	f := &loginFixture{page: &page, acct: acct}
	lw := newLoginWindow(&page)
	lw.openWindow = func() { f.opened++ }
	lw.closeWindow = func() { f.closed++ }
	lw.resizeWindow = func(height int) { f.resizes = append(f.resizes, height) }
	f.login = &lw
	return f
}

// drainLogin applies the pending backend result the way tick does, so a test
// can drive the window without a ui.Context.
func drainLogin(t *testing.T, s *loginWindow) {
	t.Helper()
	apply := <-s.results
	s.busy = false
	apply(s)
}

func loggedOutSession() accountSession {
	return accountSession{state: loginLoggedOut, serverURL: defaultServerURL}
}

func loggedInSession() accountSession {
	return accountSession{
		state:     loginLoggedIn,
		email:     "ada@example.com",
		serverURL: defaultServerURL,
		userID:    "user-1",
	}
}

// codeChallenge is a challenge offering one method that takes a code, which is
// all the window can finish.
func codeChallenge(provider int, name string) *twoFactorPrompt {
	return &twoFactorPrompt{methods: []twoFactorMethod{{
		provider:    provider,
		name:        name,
		interaction: interactionCode,
	}}}
}

// emailChallenge is the email method, whose code goes to an address.
func emailChallenge(email string) *twoFactorPrompt {
	p := codeChallenge(twoFactorEmail, "email")
	p.methods[0].email = email
	return p
}

// beginLogin fills the form and opens the window, the way the account page's
// button does.
func beginLogin(f *loginFixture, email, password string) {
	f.login.begin()
	f.login.email, f.login.password = email, password
}

// Opening the window is a UI state change: no call goes out until the form is
// submitted.
func TestLoginWindowBeginOpensOnlyTheWindow(t *testing.T) {
	f := newLoginFixture(t, &stubAccount{sess: loggedOutSession()})

	beginLogin(f, "ada@example.com", "secret")
	if !f.login.showing {
		t.Fatal("begin did not mark the window as showing")
	}
	if f.opened != 1 {
		t.Fatalf("openWindow calls = %d, want 1", f.opened)
	}
	if f.acct.loginCalls != 0 || f.acct.checkCalls != 0 {
		t.Fatalf("begin started a backend call: login=%d check=%d", f.acct.loginCalls, f.acct.checkCalls)
	}
}

// A server address the login path would refuse must be reported on the field
// that produced it, before any network call.
func TestLoginWindowBadServerAddressShowsOnTheField(t *testing.T) {
	acct := &stubAccount{
		sess:     loggedOutSession(),
		checkErr: errors.New(`服务器地址无效：auth: base URL "http://bad host" has no host`),
	}
	f := newLoginFixture(t, acct)

	beginLogin(f, "ada@example.com", "secret")
	f.login.serverURL = "http://bad host"
	f.login.submit()
	if !f.login.busy {
		t.Fatal("submit did not start a backend call")
	}
	drainLogin(t, f.login)

	if f.acct.loginCalls != 0 {
		t.Fatalf("login ran despite the bad address (%d calls)", f.acct.loginCalls)
	}
	if !strings.Contains(f.login.serverErr, "服务器地址无效") {
		t.Fatalf("serverErr = %q, want the validator's refusal", f.login.serverErr)
	}
	if !f.login.advancedOpen {
		t.Fatal("the advanced fold stayed closed around the refused field")
	}
	if f.login.busy {
		t.Fatal("the window is still busy after the refusal landed")
	}
	if !f.login.showing || f.closed != 0 {
		t.Fatalf("window closed on a bad address: showing=%v closed=%d", f.login.showing, f.closed)
	}
	// The password must not leak into the wording.
	if strings.Contains(f.login.notice, "secret") || strings.Contains(f.login.serverErr, "secret") {
		t.Fatalf("the failure wording carries the password: %q / %q", f.login.notice, f.login.serverErr)
	}
}

// What goes to the backend is the normalized address, and the form keeps it.
func TestLoginWindowServerAddressIsNormalized(t *testing.T) {
	acct := &stubAccount{sess: loggedInSession(), checkSess: "https://vault.example.com", cancelSess: loggedInSession()}
	f := newLoginFixture(t, acct)

	beginLogin(f, "ada@example.com", "secret")
	f.login.serverURL = "vault.example.com"
	f.login.submit()
	drainLogin(t, f.login)

	if f.acct.lastServer != "vault.example.com" {
		t.Fatalf("checkServer got %q, want the raw address", f.acct.lastServer)
	}
	if f.login.serverURL != "https://vault.example.com" {
		t.Fatalf("serverURL = %q, want the normalized address", f.login.serverURL)
	}
}

// A successful login lands the session on the account page, keeps the
// confirmation line across the reload, and closes the window.
func TestLoginWindowLoginSuccessLandsOnAccountPageAndCloses(t *testing.T) {
	acct := &stubAccount{sess: loggedInSession(), cancelSess: loggedInSession()}
	f := newLoginFixture(t, acct)

	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	if f.page.session.state != loginLoggedIn || f.page.session.userID != "user-1" {
		t.Fatalf("session = %+v, want the logged-in session", f.page.session)
	}
	if f.page.notice != "已登录 ada@example.com" || !f.page.noticeOK {
		t.Fatalf("notice = %q ok=%v, want the success line", f.page.notice, f.page.noticeOK)
	}
	if f.login.password != "" {
		t.Fatal("the master password stayed in the form after a successful login")
	}
	if f.login.showing || f.closed != 1 {
		t.Fatalf("window did not close: showing=%v closed=%d", f.login.showing, f.closed)
	}
	drain(t, f.page) // the reload the page started; it must not wipe the line
	if f.page.notice != "已登录 ada@example.com" {
		t.Fatalf("notice after reload = %q, want the success line", f.page.notice)
	}
}

// A failed login keeps the form up with the backend's wording, and a corrected
// retry still succeeds: the attempt is not dead after one refusal.
func TestLoginWindowLoginFailureKeepsTheFormForRetry(t *testing.T) {
	acct := &stubAccount{sess: loggedOutSession(), err: errors.New("邮箱或主密码不正确")}
	f := newLoginFixture(t, acct)

	beginLogin(f, "ada@example.com", "wrong")
	f.login.submit()
	drainLogin(t, f.login)

	if f.login.notice != "邮箱或主密码不正确" || f.login.noticeOK {
		t.Fatalf("notice = %q ok=%v, want the failure", f.login.notice, f.login.noticeOK)
	}
	if !f.login.showing || f.closed != 0 {
		t.Fatalf("window closed on a failure: showing=%v closed=%d", f.login.showing, f.closed)
	}
	if f.login.busy {
		t.Fatal("the window is still busy after the failure landed")
	}

	acct.err = nil
	acct.sess = loggedInSession()
	acct.cancelSess = loggedInSession()
	f.login.password = "secret"
	f.login.submit()
	drainLogin(t, f.login)

	if f.page.session.state != loginLoggedIn || f.closed != 1 {
		t.Fatalf("retry did not land the login: sess=%+v closed=%d", f.page.session, f.closed)
	}
}

// A 2FA stop renders the challenge the server sent, and keeps the window up.
func TestLoginWindowTwoFactorStopRendersTheChallenge(t *testing.T) {
	challenge := codeChallenge(twoFactorAuthenticator, "authenticator")
	acct := &stubAccount{sess: accountSession{
		state:     loginNeeds2FA,
		email:     "ada@example.com",
		serverURL: defaultServerURL,
		challenge: challenge,
	}}
	f := newLoginFixture(t, acct)

	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	if f.login.challenge == nil || len(f.login.challenge.codeMethods()) != 1 {
		t.Fatalf("challenge = %+v, want the offered code method", f.login.challenge)
	}
	if m, ok := f.login.selectedMethod(); !ok || m.provider != twoFactorAuthenticator {
		t.Fatalf("selectedMethod = %+v ok=%v, want the TOTP method", m, ok)
	}
	if !strings.Contains(f.login.notice, "两步验证") {
		t.Fatalf("notice = %q, want the 2FA hint", f.login.notice)
	}
	if !f.login.showing || f.closed != 0 {
		t.Fatalf("window closed at the 2FA stop: showing=%v closed=%d", f.login.showing, f.closed)
	}
}

// A challenge with no code method has nothing to submit: the window shows the
// unsupported wording and Cancel is the only way out. This is the decided
// landing path for Duo and WebAuthn, not an oversight.
func TestLoginWindowUnsupportedMethodOffersOnlyCancel(t *testing.T) {
	challenge := &twoFactorPrompt{methods: []twoFactorMethod{
		{provider: twoFactorDuo, name: "duo", interaction: interactionBrowser, browserURL: "https://duo.example/authorize"},
		{provider: twoFactorWebAuthn, name: "webauthn", interaction: interactionBrowser},
	}}
	acct := &stubAccount{
		sess:       accountSession{state: loginNeeds2FA, email: "ada@example.com", serverURL: defaultServerURL, challenge: challenge},
		cancelSess: loggedOutSession(),
	}
	f := newLoginFixture(t, acct)

	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	if len(f.login.challenge.codeMethods()) != 0 {
		t.Fatalf("codeMethods = %+v, want none", f.login.challenge.codeMethods())
	}
	if got := len(f.login.challenge.unsupported()); got != 2 {
		t.Fatalf("unsupported methods = %d, want 2", got)
	}
	f.login.submit2FA()
	if f.acct.submitCalls != 0 {
		t.Fatalf("submit2FA reached the backend with no code method (%d calls)", f.acct.submitCalls)
	}
	// The wording the unsupported panel renders is the backend's, so the
	// message the path reports and the message the human reads do not drift.
	if errTwoFactorUnsupported.Error() != "此方式 biowarden 暂不支持，请在账号页高级选项里用提取脚本录入密钥" {
		t.Fatalf("errTwoFactorUnsupported = %q", errTwoFactorUnsupported)
	}
}

func TestLoginWindowSubmitTwoFactorSuccess(t *testing.T) {
	acct := &stubAccount{
		sess:       accountSession{state: loginNeeds2FA, email: "ada@example.com", serverURL: defaultServerURL, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")},
		submitSess: loggedInSession(),
		cancelSess: loggedInSession(),
	}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	f.login.code = " 123456 "
	f.login.submit2FA()
	drainLogin(t, f.login)

	if f.acct.submitCalls != 1 || f.acct.lastProvider != twoFactorAuthenticator || f.acct.lastCode != "123456" {
		t.Fatalf("submit2FA call = (%d, %s) after %d calls", f.acct.lastProvider, f.acct.lastCode, f.acct.submitCalls)
	}
	if f.page.session.state != loginLoggedIn || !f.page.noticeOK {
		t.Fatalf("session = %+v, want the logged-in session", f.page.session)
	}
	if f.login.code != "" || f.login.challenge != nil {
		t.Fatalf("the code or the challenge outlived the login: code=%q challenge=%+v", f.login.code, f.login.challenge)
	}
	if f.login.showing || f.closed != 1 {
		t.Fatalf("window did not close: showing=%v closed=%d", f.login.showing, f.closed)
	}
}

// A rejected code keeps the prompt up with a corrected-code hint; the server
// answers a rejection as the needs-2FA state, not as an error.
func TestLoginWindowRejectedCodeKeepsThePrompt(t *testing.T) {
	challenge := codeChallenge(twoFactorAuthenticator, "authenticator")
	acct := &stubAccount{
		sess:       accountSession{state: loginNeeds2FA, email: "ada@example.com", serverURL: defaultServerURL, challenge: challenge},
		submitSess: accountSession{state: loginNeeds2FA, email: "ada@example.com", serverURL: defaultServerURL, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")},
	}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	f.login.code = "000000"
	f.login.submit2FA()
	drainLogin(t, f.login)

	if !strings.Contains(f.login.notice, "验证码未被接受") {
		t.Fatalf("notice = %q, want the retry hint", f.login.notice)
	}
	if f.login.code != "" {
		t.Fatal("the rejected code stayed in the field")
	}
	if f.login.challenge == nil || !f.login.showing || f.closed != 0 {
		t.Fatalf("the prompt did not stay up: challenge=%+v showing=%v closed=%d", f.login.challenge, f.login.showing, f.closed)
	}
}

// A refused Windows Hello prompt is a failure like any other: the message
// shows, the prompt stays up, and a retry can still finish.
func TestLoginWindowHelloRefusedKeepsThePromptForRetry(t *testing.T) {
	acct := &stubAccount{
		sess:       accountSession{state: loginNeeds2FA, email: "ada@example.com", serverURL: defaultServerURL, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")},
		submitErr:  errors.New("已取消 Windows Hello 验证"),
		cancelSess: loggedOutSession(),
	}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	f.login.code = "123456"
	f.login.submit2FA()
	drainLogin(t, f.login)

	if f.login.notice != "已取消 Windows Hello 验证" {
		t.Fatalf("notice = %q, want the consent refusal", f.login.notice)
	}
	if !f.login.showing || f.closed != 0 {
		t.Fatalf("window closed on a refused consent: showing=%v closed=%d", f.login.showing, f.closed)
	}

	acct.submitErr = nil
	acct.submitSess = loggedInSession()
	f.login.code = "654321"
	f.login.submit2FA()
	drainLogin(t, f.login)

	if f.page.session.state != loginLoggedIn || f.closed != 1 {
		t.Fatalf("retry did not land the login: sess=%+v closed=%d", f.page.session, f.closed)
	}
}

// The email method's code goes out once per challenge.
func TestLoginWindowSendEmailCodeGoesOutOnce(t *testing.T) {
	acct := &stubAccount{
		sess:       accountSession{state: loginNeeds2FA, email: "ada@example.com", serverURL: defaultServerURL, challenge: emailChallenge("ada@example.com")},
		cancelSess: loggedOutSession(),
	}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	f.login.sendEmailCode()
	drainLogin(t, f.login)
	if f.acct.sendCalls != 1 {
		t.Fatalf("sendEmailCode calls = %d, want 1", f.acct.sendCalls)
	}
	if !f.login.emailSent || !strings.Contains(f.login.notice, "验证码已发送到 ada@example.com") {
		t.Fatalf("emailSent=%v notice=%q, want a visible confirmation", f.login.emailSent, f.login.notice)
	}

	f.login.sendEmailCode()
	if f.acct.sendCalls != 1 {
		t.Fatalf("a second press reached the backend: %d calls", f.acct.sendCalls)
	}
}

// Cancelling lands the session the backend returned on the account page and
// closes the window, keeping the visible confirmation.
func TestLoginWindowCancelRestoresTheSessionAndCloses(t *testing.T) {
	acct := &stubAccount{
		sess:       accountSession{state: loginNeeds2FA, serverURL: defaultServerURL},
		cancelSess: loggedInSession(),
	}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")
	f.login.code = "123456"

	f.login.cancel()
	if !f.login.busy {
		t.Fatal("cancel did not start a backend call")
	}
	drainLogin(t, f.login)

	if f.acct.cancelCalls != 1 {
		t.Fatalf("cancelLogin calls = %d, want 1", f.acct.cancelCalls)
	}
	if f.page.session.state != loginLoggedIn || f.page.session.userID != "user-1" {
		t.Fatalf("session = %+v, want the restored signed-in session", f.page.session)
	}
	if f.page.notice != "已取消登录" || !f.page.noticeOK {
		t.Fatalf("notice = %q ok=%v, want the cancellation line", f.page.notice, f.page.noticeOK)
	}
	if f.login.password != "" || f.login.code != "" || f.login.challenge != nil {
		t.Fatalf("secrets outlived the cancel: password=%q code=%q", f.login.password, f.login.code)
	}
	if f.login.showing || f.closed != 1 {
		t.Fatalf("window did not close: showing=%v closed=%d", f.login.showing, f.closed)
	}
}

// A cancel that cannot reach the backend keeps the window up instead of
// pretending the attempt is over.
func TestLoginWindowCancelFailureKeepsTheWindow(t *testing.T) {
	acct := &stubAccount{sess: loggedOutSession(), cancelErr: errors.New("backend down")}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")

	f.login.cancel()
	drainLogin(t, f.login)

	if f.login.notice != "backend down" || f.login.noticeOK {
		t.Fatalf("notice = %q ok=%v, want the failure visible", f.login.notice, f.login.noticeOK)
	}
	if !f.login.showing || f.closed != 0 {
		t.Fatalf("window closed despite the failed cancel: showing=%v closed=%d", f.login.showing, f.closed)
	}
}

// Closing the window by hand means Cancel. While a call is in flight the
// window stays and says so; when it settles, the cancel closes it.
func TestLoginWindowCloseRequestMeansCancel(t *testing.T) {
	acct := &stubAccount{sess: loggedOutSession(), cancelSess: loggedInSession()}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")

	f.login.submit()
	f.login.requestClose()
	if f.acct.cancelCalls != 0 {
		t.Fatalf("a close during a call started a cancel (%d calls)", f.acct.cancelCalls)
	}
	if !strings.Contains(f.login.notice, "登录进行中") {
		t.Fatalf("notice = %q, want the busy hint", f.login.notice)
	}
	drainLogin(t, f.login)
	if !f.login.showing || f.closed != 0 {
		t.Fatalf("the window closed while the login was still in flight: showing=%v closed=%d", f.login.showing, f.closed)
	}

	f.login.requestClose()
	drainLogin(t, f.login)
	if f.acct.cancelCalls != 1 || f.login.showing || f.closed != 1 {
		t.Fatalf("close did not cancel: cancel=%d showing=%v closed=%d", f.acct.cancelCalls, f.login.showing, f.closed)
	}
}

// A session parked on a 2FA challenge reopens on the prompt instead of asking
// for the password again: the backend still holds that attempt.
func TestLoginWindowBeginReopensAParkedChallenge(t *testing.T) {
	challenge := codeChallenge(twoFactorAuthenticator, "authenticator")
	acct := &stubAccount{sess: accountSession{
		state:     loginNeeds2FA,
		email:     "ada@example.com",
		serverURL: defaultServerURL,
		challenge: challenge,
	}}
	f := newLoginFixture(t, acct)

	f.login.begin()
	if f.login.challenge == nil || len(f.login.challenge.codeMethods()) != 1 {
		t.Fatalf("challenge = %+v, want the parked one", f.login.challenge)
	}
	if f.login.email != "ada@example.com" {
		t.Fatalf("email = %q, want the parked address", f.login.email)
	}
	if f.opened != 1 {
		t.Fatalf("openWindow calls = %d, want 1", f.opened)
	}
}

// The address the form opens on prefers the live session, then the stored
// account's metadata, then the official cloud. An entry from before the
// metadata carried an address counts as none, so it cannot blank the form.
func TestLoginWindowPrefillsServerURL(t *testing.T) {
	cases := []struct {
		name          string
		sessionServer string
		accountServer string
		want          string
	}{
		{"session wins", "https://session.example", "https://account.example", "https://session.example"},
		{"account metadata when the session has none", "", "https://account.example", "https://account.example"},
		{"default when neither has one", "", "", defaultServerURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLoginFixture(t, &stubAccount{sess: accountSession{state: loginLoggedOut, serverURL: tc.sessionServer}})
			if tc.accountServer == "" {
				f.page.accounts = nil
			} else {
				f.page.accounts = []*accountRow{{userID: "user-1", email: "ada@example.com", serverURL: tc.accountServer}}
			}

			f.login.begin()
			if f.login.serverURL != tc.want {
				t.Fatalf("serverURL = %q, want %q", f.login.serverURL, tc.want)
			}
		})
	}
}

// Enter in a credential field runs the primary button's action, and lands the
// login the same way a click does. Cancel is not part of that path.
func TestLoginWindowEnterSubmitsTheCredentials(t *testing.T) {
	acct := &stubAccount{sess: loggedInSession(), cancelSess: loggedInSession()}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")

	if !f.login.submitOnEnter(true) {
		t.Fatal("Enter in a filled form did not start the login")
	}
	if !f.login.busy {
		t.Fatal("Enter did not mark the window busy")
	}
	drainLogin(t, f.login)

	if acct.loginCalls != 1 {
		t.Fatalf("login calls = %d, want 1", acct.loginCalls)
	}
	if acct.cancelCalls != 0 {
		t.Fatalf("Enter reached cancel (%d calls)", acct.cancelCalls)
	}
	if f.page.session.state != loginLoggedIn || f.closed != 1 {
		t.Fatalf("Enter did not land the login: sess=%+v closed=%d", f.page.session, f.closed)
	}
}

// An empty form and a call in flight both refuse Enter, so it cannot submit
// what the disabled Login button would not.
func TestLoginWindowEnterRespectsTheLoginButtonGuard(t *testing.T) {
	acct := &stubAccount{sess: loggedInSession(), cancelSess: loggedInSession()}
	f := newLoginFixture(t, acct)

	f.login.begin()
	f.login.email = "ada@example.com"
	if f.login.submitOnEnter(true) {
		t.Fatal("Enter submitted a form with no password")
	}
	if acct.loginCalls != 0 {
		t.Fatalf("login calls = %d, want 0 with an empty password", acct.loginCalls)
	}

	f.login.password = "secret"
	f.login.submit()
	if f.login.submitOnEnter(true) {
		t.Fatal("Enter submitted a second time while one call was in flight")
	}
	drainLogin(t, f.login)
	if acct.loginCalls != 1 {
		t.Fatalf("login calls = %d, want 1: busy must not double-submit", acct.loginCalls)
	}
}

// Enter at the 2FA step submits the code, and refuses an empty one or a
// challenge with nothing biowarden can answer.
func TestLoginWindowEnterSubmitsTheTwoFactorCode(t *testing.T) {
	acct := &stubAccount{
		sess:       accountSession{state: loginNeeds2FA, email: "ada@example.com", serverURL: defaultServerURL, challenge: codeChallenge(twoFactorAuthenticator, "authenticator")},
		submitSess: loggedInSession(),
		cancelSess: loggedInSession(),
	}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	if f.login.submitTwoFactorOnEnter(true) {
		t.Fatal("Enter submitted an empty code")
	}
	if acct.submitCalls != 0 {
		t.Fatalf("submit calls = %d, want 0 with an empty code", acct.submitCalls)
	}

	f.login.code = " 123456 "
	if !f.login.submitTwoFactorOnEnter(true) {
		t.Fatal("Enter with a code did not submit it")
	}
	drainLogin(t, f.login)
	if acct.submitCalls != 1 || acct.lastCode != "123456" {
		t.Fatalf("submit calls = %d code = %q, want the trimmed code", acct.submitCalls, acct.lastCode)
	}
	if acct.cancelCalls != 0 {
		t.Fatalf("Enter at the 2FA step reached cancel (%d calls)", acct.cancelCalls)
	}
	if f.page.session.state != loginLoggedIn || f.closed != 1 {
		t.Fatalf("Enter did not land the 2FA login: sess=%+v closed=%d", f.page.session, f.closed)
	}
}

// A challenge with no code method never lets Enter reach the backend.
func TestLoginWindowEnterDoesNothingForUnsupportedMethods(t *testing.T) {
	challenge := &twoFactorPrompt{methods: []twoFactorMethod{
		{provider: twoFactorDuo, name: "duo", interaction: interactionBrowser, browserURL: "https://duo.example/authorize"},
	}}
	acct := &stubAccount{
		sess:       accountSession{state: loginNeeds2FA, email: "ada@example.com", serverURL: defaultServerURL, challenge: challenge},
		cancelSess: loggedOutSession(),
	}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	f.login.code = "whatever"
	if f.login.submitTwoFactorOnEnter(true) {
		t.Fatal("Enter claimed a submission on an unsupported panel")
	}
	if acct.submitCalls != 0 {
		t.Fatalf("submit calls = %d, want 0 for Duo/WebAuthn", acct.submitCalls)
	}
}

// The window follows the form: opening Advanced grows it, closing it shrinks it
// back, and a steady frame does not touch the window server again.
func TestLoginWindowResizesForTheAdvancedSection(t *testing.T) {
	f := newLoginFixture(t, &stubAccount{sess: loggedOutSession()})
	beginLogin(f, "ada@example.com", "secret")

	if len(f.resizes) != 0 {
		t.Fatalf("opening with Advanced closed resized: %v", f.resizes)
	}

	f.login.advancedOpen = true
	f.login.syncHeight()
	if len(f.resizes) != 1 || f.resizes[0] != loginWindowAdvancedHeight {
		t.Fatalf("expanded resizes = %v, want [%d]", f.resizes, loginWindowAdvancedHeight)
	}

	// The view calls syncHeight every frame; a later one must be a no-op.
	f.login.syncHeight()
	if len(f.resizes) != 1 {
		t.Fatalf("a steady frame resized again: %v", f.resizes)
	}

	f.login.advancedOpen = false
	f.login.syncHeight()
	if len(f.resizes) != 2 || f.resizes[1] != loginWindowHeight {
		t.Fatalf("collapsed resizes = %v, want the opening height back", f.resizes)
	}
}

// A refused address opens Advanced, so the resize rides along with it.
func TestLoginWindowResizesWhenABadAddressOpensAdvanced(t *testing.T) {
	acct := &stubAccount{
		sess:     loggedOutSession(),
		checkErr: errors.New("服务器地址无效"),
	}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")
	f.login.serverURL = "http://bad host"
	f.login.submit()
	drainLogin(t, f.login)

	if len(f.resizes) != 1 || f.resizes[0] != loginWindowAdvancedHeight {
		t.Fatalf("resizes after the refused address = %v, want [%d]", f.resizes, loginWindowAdvancedHeight)
	}
}

// Each two-factor stage has its own target: a single code method is the
// smallest, and the rows the challenge adds raise it.
func TestLoginWindowResizesForTwoFactorStages(t *testing.T) {
	cases := []struct {
		name    string
		methods []twoFactorMethod
		want    int
	}{
		{
			name:    "single method",
			methods: []twoFactorMethod{{provider: twoFactorAuthenticator, name: "authenticator", interaction: interactionCode}},
			want:    loginWindowTwoFactorHeight,
		},
		{
			name:    "email method adds the send row",
			methods: []twoFactorMethod{{provider: twoFactorEmail, name: "email", interaction: interactionCode}},
			want:    loginWindowTwoFactorHeight + loginWindowEmailHeight,
		},
		{
			name: "two code methods add the segmented row",
			methods: []twoFactorMethod{
				{provider: twoFactorAuthenticator, name: "authenticator", interaction: interactionCode},
				{provider: twoFactorEmail, name: "email", interaction: interactionCode},
			},
			want: loginWindowTwoFactorHeight + loginWindowSegmentHeight,
		},
		{
			name: "an unsupported method adds its line",
			methods: []twoFactorMethod{
				{provider: twoFactorAuthenticator, name: "authenticator", interaction: interactionCode},
				{provider: twoFactorDuo, name: "duo", interaction: interactionBrowser},
			},
			want: loginWindowTwoFactorHeight + loginWindowUnsupportedHeight,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acct := &stubAccount{sess: accountSession{
				state:     loginNeeds2FA,
				email:     "ada@example.com",
				serverURL: defaultServerURL,
				challenge: &twoFactorPrompt{methods: tc.methods},
			}}
			f := newLoginFixture(t, acct)
			beginLogin(f, "ada@example.com", "secret")
			f.login.submit()
			drainLogin(t, f.login)

			if len(f.resizes) != 1 || f.resizes[0] != tc.want {
				t.Fatalf("resizes = %v, want [%d]", f.resizes, tc.want)
			}
		})
	}
}
