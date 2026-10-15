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
