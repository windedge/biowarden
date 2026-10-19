package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The typed backend sentinels must become wording in the language the app
// shows, and nothing else: an error the UI has no wording for says its own
// message, which the backend never writes in a user-facing language.
func TestLoginErrorTextMapsSentinels(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		en, zh string
	}{
		{"bad credentials", errWrongCredentials, "email or master password is incorrect", "邮箱或主密码不正确"},
		{"forbidden", errLoginForbidden, "refused the login", "服务器拒绝登录"},
		{"captcha", errCaptchaRequired, "CAPTCHA", "人机验证"},
		{"network", errNetworkUnreachable, "Cannot reach the server", "无法连接服务器"},
		{"server", errServerFailure, "returned an error", "服务器返回错误"},
		{"no method", errNoTwoFactorMethod, "offered no usable method", "没有返回可用的验证方式"},
		{"unsupported", errTwoFactorUnsupported, "not supported by biowarden", "暂不支持"},
		{"no challenge", errNoTwoFactorChallenge, "no login waiting for a code", "没有等待验证码的登录"},
		{"empty code", errTwoFactorEmptyCode, "Enter the two-factor code", "请输入两步验证码"},
		{"email method missing", errEmailMethodMissing, "did not offer the email code method", "没有提供邮箱验证码方式"},
		{"server address", newLoginError(errServerAddress, "no host"), "Invalid server address: no host", "服务器地址无效：no host"},
		{"flow start", newLoginError(errLoginFlowStart, "loginNeedPassword"), "started in an unexpected state", "起始状态异常"},
		{"flow state", newLoginError(errLoginFlowState, "loginNeedPassword"), "did not finish", "登录未完成"},
		{"save key", newLoginError(errSaveKey, "vault full"), "Saving the key failed: vault full", "保存密钥失败：vault full"},
		{"provider", newLoginError(errProviderNotOffered, 7), "provider 7", "provider 7"},
		{"persist session", newLoginError(errSessionPersist, "write failed"), "could not be persisted", "无法持久化"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appLanguage(t, "en")
			if got := errorText(tc.err); !strings.Contains(got, tc.en) {
				t.Errorf("en errorText(%v) = %q, want it to contain %q", tc.err, got, tc.en)
			}
			appLanguage(t, "zh-Hans")
			if got := errorText(tc.err); !strings.Contains(got, tc.zh) {
				t.Errorf("zh errorText(%v) = %q, want it to contain %q", tc.err, got, tc.zh)
			}
		})
	}
	if got := errorText(nil); got != "" {
		t.Errorf("errorText(nil) = %q, want empty", got)
	}
	other := errors.New("login failed hard")
	if got := errorText(other); got != "login failed hard" {
		t.Errorf("errorText(unknown) = %q, want the error's own message", got)
	}
}

// The status line keeps its kind and arguments, so it is re-said in the new
// language when the app's language changes; the wording is produced at draw
// time, not when the state was set.
func TestLoginNoticeFollowsTheLanguage(t *testing.T) {
	notices := []loginNotice{
		{kind: loginNoticeBadServer},
		{kind: loginNoticeTwoFactor},
		{kind: loginNoticeSigningIn},
		{kind: loginNoticeCodeRejected},
		{kind: loginNoticeVerifying},
		{kind: loginNoticeEmailSent},
		{kind: loginNoticeEmailSentTo, args: []string{"ada@example.com"}},
		{kind: loginNoticeBusy},
	}

	appLanguage(t, "en")
	english := make([]string, len(notices))
	for i, n := range notices {
		english[i] = n.text()
		if english[i] == "" {
			t.Fatalf("notice %d says nothing in English", i)
		}
	}
	appLanguage(t, "zh-Hans")
	for i, n := range notices {
		if got := n.text(); got == "" || got == english[i] {
			t.Errorf("notice %d = %q, want Chinese, not the English %q", i, got, english[i])
		}
	}

	// A failure's wording follows too, because the error is mapped at draw time.
	failure := loginNotice{kind: loginNoticeFailure, failure: errWrongCredentials}
	appLanguage(t, "en")
	if got := failure.text(); !strings.Contains(got, "incorrect") {
		t.Errorf("en failure notice = %q, want the English wording", got)
	}
	appLanguage(t, "zh-Hans")
	if got := failure.text(); got != "邮箱或主密码不正确" {
		t.Errorf("zh failure notice = %q, want the Chinese wording", got)
	}

	// An empty notice says nothing in any language.
	appLanguage(t, "zh-Hans")
	if got := (loginNotice{}).text(); got != "" {
		t.Errorf("empty notice = %q, want empty", got)
	}
}

// The height target and the drawing share loginNotice.text, so the height
// follows the wording the current language draws: a longer English notice
// wraps to another row where the Chinese one fits on one.
func TestLoginNoticeHeightFollowsTheLanguage(t *testing.T) {
	appLanguage(t, "zh-Hans")
	f := newLoginFixture(t, &stubAccount{sess: loggedOutSession()})
	beginLogin(f, "ada@example.com", "secret")
	f.login.notice = loginNotice{kind: loginNoticeTwoFactor}
	zhText, zhHeight := f.login.notice.text(), f.login.targetHeight()

	appLanguage(t, "en")
	enText, enHeight := f.login.notice.text(), f.login.targetHeight()

	if zhText == enText {
		t.Fatal("the notice says the same thing in both languages")
	}
	if zhHeight != loginWindowHeight+loginWindowNoticeHeight {
		t.Errorf("zh target = %d, want the opening height plus one notice row", zhHeight)
	}
	// The English wording is longer and wraps to a second line, so the target
	// grows with it: the height comes from the drawn text, not the state.
	if want := loginWindowHeight + loginWindowNoticeHeight + loginWindowNoticeLineHeight; enHeight != want {
		t.Errorf("en target = %d, want the opening height plus two notice rows (%d)", enHeight, want)
	}
}

// Each two-factor method is named in the language the app shows, and the list
// separator is the one that language uses.
func TestLoginMethodLabelsFollowTheLanguage(t *testing.T) {
	methods := []twoFactorMethod{
		{provider: twoFactorAuthenticator, name: "authenticator"},
		{provider: twoFactorEmail, name: "email"},
		{provider: twoFactorYubiKey, name: "yubikey"},
		{provider: twoFactorDuo, name: "duo"},
		{provider: twoFactorOrgDuo, name: "org duo"},
		{provider: twoFactorWebAuthn, name: "webauthn"},
		{provider: twoFactorRecoveryCode, name: "recovery"},
	}

	appLanguage(t, "en")
	if got, want := methodTitles(methods), "Authenticator app (TOTP), Email code, YubiKey, Duo, Duo, Security key (WebAuthn), Recovery code"; got != want {
		t.Errorf("en methodTitles = %q, want %q", got, want)
	}
	if got := methodFieldLabel(twoFactorMethod{provider: twoFactorEmail}); got != "Email code" {
		t.Errorf("en email field label = %q", got)
	}
	if got := methodHint(twoFactorMethod{provider: twoFactorYubiKey}); !strings.Contains(got, "44-character") {
		t.Errorf("en YubiKey hint = %q", got)
	}
	if got := methodHint(twoFactorMethod{provider: twoFactorAuthenticator}); !strings.Contains(got, "6-digit") {
		t.Errorf("en default hint = %q", got)
	}

	appLanguage(t, "zh-Hans")
	if got, want := methodTitles(methods), "验证器应用（TOTP）、邮箱验证码、YubiKey、Duo、Duo、安全密钥（WebAuthn）、恢复码"; got != want {
		t.Errorf("zh methodTitles = %q, want %q", got, want)
	}
	if got := methodFieldLabel(twoFactorMethod{provider: twoFactorEmail}); got != "邮箱验证码" {
		t.Errorf("zh email field label = %q", got)
	}
	if got := methodHint(twoFactorMethod{provider: twoFactorYubiKey}); got != "触摸 YubiKey 后出现的 44 位密码" {
		t.Errorf("zh YubiKey hint = %q", got)
	}
	if got := methodTitle(twoFactorMethod{provider: 99, name: "custom"}); got != "custom" {
		t.Errorf("an unknown provider = %q, want its protocol name", got)
	}
}

// The login window's own labels are drawn from the catalog, so the same form
// renders in English and in Chinese.
func TestLoginWindowRendersInBothLanguages(t *testing.T) {
	cases := []struct{ name, tag, title, submit, cancel string }{
		{"english", "en", "Sign in to Bitwarden", "Sign in", "Cancel"},
		{"chinese", "zh-Hans", "登录 Bitwarden 账号", "登录", "取消"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appLanguage(t, tc.tag)
			f := newLoginFixture(t, &stubAccount{sess: loggedOutSession()})
			beginLogin(f, "ada@example.com", "secret")

			tester := ui.NewTester(f.login.loginView, loginWindowWidth, 1600)
			tester.Frame()
			for _, want := range []string{tc.title, tc.submit, tc.cancel} {
				if _, ok := tester.Find(want); !ok {
					t.Errorf("the credentials form does not show %q: %v", want, tester.Texts())
				}
			}
		})
	}
}

// The two-factor prompt and the unsupported panel render in both languages
// too, including the field's label and the wording that points at the account
// page's extraction script.
func TestLoginTwoFactorRendersInBothLanguages(t *testing.T) {
	cases := []struct {
		name, tag, field, send string
	}{
		{name: "english", tag: "en", field: "Email code", send: "Send code"},
		{name: "chinese", tag: "zh-Hans", field: "邮箱验证码", send: "发送验证码"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appLanguage(t, tc.tag)
			acct := &stubAccount{sess: accountSession{
				state:     loginNeeds2FA,
				email:     "ada@example.com",
				serverURL: defaultServerURL,
				challenge: emailChallenge("ada@example.com"),
			}}
			f := newLoginFixture(t, acct)
			beginLogin(f, "ada@example.com", "secret")
			f.login.submit()
			drainLogin(t, f.login)

			tester := ui.NewTester(f.login.loginView, loginWindowWidth, 1600)
			tester.Frame()
			for _, want := range []string{tc.field, tc.send} {
				if _, ok := tester.Find(want); !ok {
					t.Errorf("the email prompt does not show %q: %v", want, tester.Texts())
				}
			}
		})
	}
}

// The unsupported panel names the methods it cannot answer and the extraction
// script, in the app's language.
func TestLoginUnsupportedPanelRendersInBothLanguages(t *testing.T) {
	cases := []struct {
		name, tag, want string
	}{
		{"english", "en", "not supported by biowarden"},
		{"chinese", "zh-Hans", "暂不支持"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appLanguage(t, tc.tag)
			challenge := &twoFactorPrompt{methods: []twoFactorMethod{
				{provider: twoFactorDuo, name: "duo", interaction: interactionBrowser},
			}}
			acct := &stubAccount{sess: accountSession{
				state: loginNeeds2FA, email: "ada@example.com", serverURL: defaultServerURL, challenge: challenge,
			}}
			f := newLoginFixture(t, acct)
			beginLogin(f, "ada@example.com", "secret")
			f.login.submit()
			drainLogin(t, f.login)

			tester := ui.NewTester(f.login.loginView, loginWindowWidth, 1600)
			tester.Frame()
			found := false
			for _, text := range tester.Texts() {
				if strings.Contains(text, tc.want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("the unsupported panel does not say %q: %v", tc.want, tester.Texts())
			}
		})
	}
}

// The login window's language listener is dropped with the window, and
// unsubscribing twice is a no-op: the window is rebuilt for every attempt.
func TestUnsubscribeLoginLocaleIsIdempotent(t *testing.T) {
	app := &appState{}
	calls := 0
	app.loginLocaleOff = func() { calls++ }

	app.unsubscribeLoginLocale()
	app.unsubscribeLoginLocale()

	if calls != 1 {
		t.Fatalf("off calls = %d, want 1", calls)
	}
	if app.loginLocaleOff != nil {
		t.Fatal("the off function stayed set after unsubscribing")
	}
}
