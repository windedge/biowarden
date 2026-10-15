package ui

import (
	"errors"
	"testing"
)

// TestNavPageRoundTrip keeps the sidebar's string IDs and the app's page enum
// in step: a typo in either map would send a click to the wrong page.
func TestNavPageRoundTrip(t *testing.T) {
	for _, p := range []navPage{pageAccount, pageBrowsers, pageSettings, pageLog} {
		if got := navPageOf(p.navID()); got != p {
			t.Fatalf("navPageOf(%q) = %v, want %v", p.navID(), got, p)
		}
	}
	// An unknown ID falls back to the landing page rather than a blank screen.
	if got := navPageOf("nope"); got != pageAccount {
		t.Fatalf("navPageOf(unknown) = %v, want pageAccount", got)
	}
}

// drain applies the pending async result the way tick does, so a test can
// drive the state machine without a ui.Context.
func drain(t *testing.T, s *accountState) {
	t.Helper()
	// Block: the async call always posts exactly one result.
	apply := <-s.results
	s.busy = false
	apply(s)
}

func TestAccountStateStartsLoggedOutWithStoredKeys(t *testing.T) {
	s := newAccountStateWith(fakeKeys{}, fakeAccount{})
	drain(t, &s)

	if s.session.state != loginLoggedOut {
		t.Fatalf("session state = %v, want logged out", s.session.state)
	}
	if s.session.serverURL != defaultServerURL {
		t.Fatalf("server = %q, want %q", s.session.serverURL, defaultServerURL)
	}
	if len(s.accounts) == 0 {
		t.Fatalf("stored keys were not loaded")
	}
	if s.primary() == nil {
		t.Fatalf("primary() = nil with stored keys")
	}
}

// TestAccountStateBeginLoginIsUIOnly checks the button the task says must not
// log in: it opens the form and starts no backend call.
func TestAccountStateBeginLoginIsUIOnly(t *testing.T) {
	s := newAccountStateWith(fakeKeys{}, fakeAccount{})
	drain(t, &s)

	s.beginLogin()
	if !s.loginOpen {
		t.Fatalf("beginLogin did not open the login form")
	}
	if s.busy {
		t.Fatalf("beginLogin started a backend call")
	}
}

func TestAccountStateLoginReportsNotImplemented(t *testing.T) {
	s := newAccountStateWith(fakeKeys{}, fakeAccount{})
	drain(t, &s)

	s.email, s.password = "ada@example.com", "secret"
	s.login()
	if !s.busy {
		t.Fatalf("login did not start a backend call")
	}
	drain(t, &s)
	if s.notice != errAccountBackend.Error() {
		t.Fatalf("notice = %q, want the not-implemented error", s.notice)
	}
	if s.noticeOK {
		t.Fatalf("noticeOK = true for a not-implemented backend")
	}
}

// stubAccount is an accountBackend with a fixed session and error, so the
// page state machine can be driven without a network or a Hello prompt.
type stubAccount struct {
	sess        accountSession
	err         error
	loginCalls  int
	cancelSess  accountSession
	cancelErr   error
	cancelCalls int
}

func (a *stubAccount) session() (accountSession, error) { return a.sess, nil }
func (a *stubAccount) login(string, string, string) (accountSession, error) {
	a.loginCalls++
	return a.sess, a.err
}
func (a *stubAccount) cancelLogin() (accountSession, error) {
	a.cancelCalls++
	if a.cancelErr != nil {
		return accountSession{}, a.cancelErr
	}
	return a.cancelSess, nil
}
func (a *stubAccount) submit2FA(string) (accountSession, error) { return a.sess, a.err }
func (a *stubAccount) logout() error                            { return a.err }
func (a *stubAccount) refreshKey() error                        { return a.err }
func (a *stubAccount) relogin() error                           { return a.err }

// A successful login must land the session on the page, keep its confirmation
// line on screen across the reload it triggers, and never leave the login form
// open.
func TestAccountStateLoginSuccessShowsSession(t *testing.T) {
	acct := &stubAccount{sess: accountSession{
		state:     loginLoggedIn,
		email:     "ada@example.com",
		serverURL: defaultServerURL,
		userID:    "user-1",
	}}
	s := newAccountStateWith(fakeKeys{}, acct)
	drain(t, &s)

	s.loginOpen = true
	s.email, s.password = "ada@example.com", "secret"
	s.login()
	drain(t, &s) // the login result, which starts a store reload
	if s.session.state != loginLoggedIn || s.session.userID != "user-1" {
		t.Fatalf("session = %+v, want the logged-in session", s.session)
	}
	if s.notice != "已登录 ada@example.com" || !s.noticeOK {
		t.Fatalf("notice = %q ok=%v, want the success line", s.notice, s.noticeOK)
	}
	if s.loginOpen {
		t.Fatal("login form stayed open after a successful login")
	}
	drain(t, &s) // the reload; it must not wipe the success line
	if s.notice != "已登录 ada@example.com" {
		t.Fatalf("notice after reload = %q, want the success line", s.notice)
	}
}

// Cancelling must close the form, drop the pending 2FA session the backend
// returned away from, and leave a visible confirmation.
func TestAccountStateCancelLoginClearsPendingTwoFactor(t *testing.T) {
	acct := &stubAccount{
		sess:       accountSession{state: loginLoggedOut, serverURL: defaultServerURL},
		cancelSess: accountSession{state: loginLoggedOut, serverURL: defaultServerURL},
	}
	s := newAccountStateWith(fakeKeys{}, acct)
	drain(t, &s)

	s.session = accountSession{state: loginNeeds2FA, serverURL: defaultServerURL, challenge: &twoFactorPrompt{}}
	s.loginOpen = true
	s.email, s.password, s.twoFACode = "ada@example.com", "secret", "123456"
	s.cancelLogin()
	if !s.busy {
		t.Fatal("cancelLogin did not start a backend call")
	}
	drain(t, &s)

	if acct.cancelCalls != 1 {
		t.Fatalf("cancelLogin calls = %d, want 1", acct.cancelCalls)
	}
	if s.session.state == loginNeeds2FA {
		t.Fatalf("session = %+v, want the pending 2FA state gone", s.session)
	}
	if s.loginOpen || s.email != "" || s.password != "" || s.twoFACode != "" {
		t.Fatalf("form not cleared: open=%v email=%q password=%q code=%q", s.loginOpen, s.email, s.password, s.twoFACode)
	}
	if s.notice != "已取消登录" || !s.noticeOK {
		t.Fatalf("notice = %q ok=%v, want the cancellation line", s.notice, s.noticeOK)
	}
}

// A cancel that cannot reach the backend must tell the human instead of
// silently closing the form.
func TestAccountStateCancelLoginSurfacesFailure(t *testing.T) {
	acct := &stubAccount{cancelErr: errors.New("backend down")}
	s := newAccountStateWith(fakeKeys{}, acct)
	drain(t, &s)

	s.loginOpen = true
	s.cancelLogin()
	drain(t, &s)

	if s.notice != "backend down" || s.noticeOK {
		t.Fatalf("notice = %q ok=%v, want the failure visible", s.notice, s.noticeOK)
	}
}

// A cancelled relogin comes back to the signed-in session, not to the 2FA
// prompt the backend had parked itself in.
func TestAccountStateCancelLoginRestoresSignedInSession(t *testing.T) {
	signedIn := accountSession{state: loginLoggedIn, email: "ada@example.com", serverURL: defaultServerURL, userID: "user-1"}
	acct := &stubAccount{sess: signedIn, cancelSess: signedIn}
	s := newAccountStateWith(fakeKeys{}, acct)
	drain(t, &s)

	s.session = accountSession{state: loginNeeds2FA, serverURL: defaultServerURL}
	s.cancelLogin()
	drain(t, &s)

	if s.session.state != loginLoggedIn || s.session.userID != "user-1" {
		t.Fatalf("session = %+v, want the previous signed-in session", s.session)
	}
}
