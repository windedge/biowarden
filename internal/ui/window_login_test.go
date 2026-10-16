package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// renderedContentHeight measures the rendered login form at the login window's
// width: the lowest text box plus the column's 16 DIP bottom padding. The state
// machine cannot measure at runtime (it has no window), so this pins the height
// constants to what the renderer actually lays out.
func renderedContentHeight(t *testing.T, s *loginWindow) float32 {
	t.Helper()
	tester := ui.NewTester(s.loginView, loginWindowWidth, 1600)
	tester.Frame()
	var bottom float32
	for _, text := range tester.Texts() {
		if r, ok := tester.Find(text); ok && r.Y+r.H > bottom {
			bottom = r.Y + r.H
		}
	}
	return bottom + 16
}

// The two-factor prompt with its retry hint must not shrink the frame room the
// same prompt has without one: that loss is what put the screenshot's scroll
// bar on the window. Counting the notice in the target restores it.
func TestLoginWindowTwoFactorNoticeFitsMeasuredContent(t *testing.T) {
	acct := &stubAccount{sess: accountSession{
		state:     loginNeeds2FA,
		email:     "ada@example.com",
		serverURL: defaultServerURL,
		challenge: codeChallenge(twoFactorAuthenticator, "authenticator"),
	}}
	f := newLoginFixture(t, acct)
	beginLogin(f, "ada@example.com", "secret")
	f.login.submit()
	drainLogin(t, f.login)

	// The 2FA stop left the "server asked for two-factor" hint up.
	if f.login.notice == "" {
		t.Fatal("the 2FA stop left no notice to measure")
	}
	noticeContent := renderedContentHeight(t, f.login)
	noticeTarget := f.login.targetHeight()

	f.login.notice = ""
	plainContent := renderedContentHeight(t, f.login)
	plainTarget := f.login.targetHeight()

	if noticeContent > float32(noticeTarget) {
		t.Fatalf("2FA content %.1f exceeds its target %d", noticeContent, noticeTarget)
	}
	plainSlack := float32(plainTarget) - plainContent
	noticeSlack := float32(noticeTarget) - noticeContent
	if noticeSlack < plainSlack-1 {
		t.Fatalf("notice slack %.1f < plain slack %.1f: the window would scroll", noticeSlack, plainSlack)
	}
	if noticeTarget != plainTarget+loginWindowNoticeHeight {
		t.Fatalf("notice target %d, want the plain target %d plus one notice row", noticeTarget, plainTarget)
	}
}
