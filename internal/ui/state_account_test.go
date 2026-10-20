package ui

import (
	"testing"
	"time"
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

// drain applies one pending page result the way tick does, so a test can
// drive an async backend call without a ui.Context. It blocks: async always
// posts exactly one result. The startup restore lands on its own lane and is
// drained by waitRestore instead.
func drain(t *testing.T, s *accountState) {
	t.Helper()
	apply := <-s.results
	s.busy = false
	apply(s)
}

// waitRestore applies the startup restore's result the way tick does while
// verifying, blocking until it lands so a test can assert the settled state.
func waitRestore(t *testing.T, s *accountState) {
	t.Helper()
	select {
	case apply := <-s.restoreResult:
		apply(s)
	case <-time.After(2 * time.Second):
		t.Fatal("the startup restore did not settle")
	}
}

func TestAccountStateStartsLoggedOutWithStoredKeys(t *testing.T) {
	s := newAccountStateWith(fakeKeys{}, fakeAccount{})
	waitRestore(t, &s)

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

	// restoreSet lets a test drive the first-load restore independently of the
	// plain session() answer; without it restoreSession mirrors sess.
	restoreSet  bool
	restoreSess accountSession
	restoreErr  error
	// restoreGate, when set, holds restoreSession until it is closed, so a
	// test can freeze the startup verification on the wire.
	restoreGate chan struct{}

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

	// forgotten records the ids removeAccount asked to drop sessions for.
	forgotten []string
	forgetErr error

	sessionCalls int
	restoreCalls int
}

func (a *stubAccount) session() (accountSession, error) {
	a.sessionCalls++
	return a.sess, nil
}

func (a *stubAccount) restoreSession() (accountSession, error) {
	a.restoreCalls++
	if a.restoreGate != nil {
		<-a.restoreGate
	}
	if a.restoreSet {
		return a.restoreSess, a.restoreErr
	}
	return a.sess, nil
}

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
	// Mirror the backend: a cancel lands the session it returns, so a later
	// reload's session() read agrees with what the page shows.
	a.sess = a.cancelSess
	return a.cancelSess, nil
}

func (a *stubAccount) submit2FA(provider int, code string) (accountSession, error) {
	a.submitCalls++
	a.lastProvider, a.lastCode = provider, code
	if a.submitErr != nil {
		return a.submitSess, a.submitErr
	}
	// Mirror the backend: a successful answer replaces the parked session, so
	// the page reload that follows does not resurrect the 2FA prompt.
	a.sess = a.submitSess
	return a.submitSess, nil
}

func (a *stubAccount) sendEmailCode() error {
	a.sendCalls++
	return a.sendErr
}

func (a *stubAccount) logout() error {
	if a.err != nil {
		return a.err
	}
	a.sess = loggedOutSession()
	return nil
}

func (a *stubAccount) forgetSession(userID string) error {
	a.forgotten = append(a.forgotten, userID)
	if a.forgetErr != nil {
		return a.forgetErr
	}
	// Mirror the backend: dropping a key's session also resets the in-memory
	// session when it is that account's.
	if a.sess.userID == userID {
		a.sess = loggedOutSession()
	}
	return nil
}
