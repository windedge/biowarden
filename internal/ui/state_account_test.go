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

// stubAccount is an accountBackend with fixed answers, so the account page and
// the login window can be driven without a network or a Hello prompt. Every
// call is counted, because the tests care about which calls ran, not only
// about their results.
type stubAccount struct {
	sess accountSession
	err  error

	loginCalls   int
	lastEmail    string
	lastPassword string

	checkSess  string
	checkErr   error
	checkCalls int
	lastServer string

	cancelSess  accountSession
	cancelErr   error
	cancelCalls int

	submitSess   accountSession
	submitErr    error
	submitCalls  int
	lastProvider int
	lastCode     string

	sendErr   error
	sendCalls int
}

func (a *stubAccount) session() (accountSession, error) { return a.sess, nil }

func (a *stubAccount) checkServer(serverURL string) (string, error) {
	a.checkCalls++
	a.lastServer = serverURL
	if a.checkErr != nil {
		return "", a.checkErr
	}
	if a.checkSess != "" {
		return a.checkSess, nil
	}
	return serverURLOf(serverURL), nil
}

func (a *stubAccount) login(email, password, _ string) (accountSession, error) {
	a.loginCalls++
	a.lastEmail, a.lastPassword = email, password
	return a.sess, a.err
}

func (a *stubAccount) cancelLogin() (accountSession, error) {
	a.cancelCalls++
	if a.cancelErr != nil {
		return accountSession{}, a.cancelErr
	}
	return a.cancelSess, nil
}

func (a *stubAccount) submit2FA(provider int, code string) (accountSession, error) {
	a.submitCalls++
	a.lastProvider, a.lastCode = provider, code
	return a.submitSess, a.submitErr
}

func (a *stubAccount) sendEmailCode() error {
	a.sendCalls++
	return a.sendErr
}

func (a *stubAccount) logout() error     { return a.err }
func (a *stubAccount) refreshKey() error { return a.err }
func (a *stubAccount) relogin() error    { return a.err }
