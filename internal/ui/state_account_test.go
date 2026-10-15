package ui

import "testing"

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
	sess       accountSession
	err        error
	loginCalls int
}

func (a *stubAccount) session() (accountSession, error) { return a.sess, nil }
func (a *stubAccount) login(string, string, string) (accountSession, error) {
	a.loginCalls++
	return a.sess, a.err
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
