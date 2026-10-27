package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
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

// shrinkSessionRetries makes the retry waits and the verified linger
// milliseconds-scale for one test, the way shrinkBackoffs does.
func shrinkSessionRetries(t *testing.T) {
	t.Helper()
	oldDelays, oldLinger := sessionRetryDelays, sessionVerifiedLinger
	sessionRetryDelays = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}
	sessionVerifiedLinger = 10 * time.Millisecond
	t.Cleanup(func() {
		sessionRetryDelays, sessionVerifiedLinger = oldDelays, oldLinger
	})
}

// Each transient class of a failed session check gets its own wording, and an
// unrecognized failure falls back to the generic unverified line.
func TestAccountStateNoticesSessionCheckClasses(t *testing.T) {
	appLanguage(t, "en")
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"network", &sessionUnverifiedError{class: errSessionNetwork, cause: errors.New("dial")}, locales.AccountNoticeSessionUnverifiedNetwork()},
		{"rate limited", &sessionUnverifiedError{class: errSessionRateLimited, cause: errors.New("429")}, locales.AccountNoticeSessionUnverifiedRateLimited()},
		{"server", &sessionUnverifiedError{class: errSessionServerError, cause: errors.New("502")}, locales.AccountNoticeSessionUnverifiedServer()},
		{"other", errSessionUnverified, locales.AccountNoticeSessionUnverified()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			acct := &stubAccount{restoreSet: true, restoreSess: loggedOutSession(), restoreErr: tc.err}
			s := newAccountStateWith(fakeKeys{}, acct)
			waitRestore(t, &s)
			if got := s.notice.text(); got != tc.want {
				t.Fatalf("notice = %q, want %q", got, tc.want)
			}
			if !s.sessionCheckFailed() {
				t.Fatal("sessionCheckFailed = false, want the recheck button offered")
			}
		})
	}
}

// The recheck button reruns the check: the notice turns to checking, a
// second restore goes out (with the one-shot latch held closed, so a reload
// meanwhile cannot stack a third), and a success lands on the verified line.
func TestAccountStateRecheckRecoversToVerified(t *testing.T) {
	shrinkSessionRetries(t)
	appLanguage(t, "en")
	acct := &stubAccount{restoreSet: true, restoreSess: loggedOutSession(),
		restoreErr: &sessionUnverifiedError{class: errSessionNetwork, cause: errors.New("dial")}}
	s := newAccountStateWith(fakeKeys{}, acct)
	waitRestore(t, &s)
	if acct.restoreCalls != 1 || !s.sessionCheckFailed() {
		t.Fatalf("setup: restoreCalls=%d notice=%q", acct.restoreCalls, s.notice.text())
	}

	// The recheck's answer is scripted before the call: stubAccount's
	// fields are not synchronized, so writing them while the recheck is
	// already on the wire would race.
	acct.restoreErr = nil
	acct.restoreSess = loggedInSession()

	s.recheck()
	if s.notice.text() != locales.AccountNoticeSessionChecking() {
		t.Fatalf("notice while rechecking = %q, want the checking line", s.notice.text())
	}
	if !s.verifying {
		t.Fatal("verifying = false, want the recheck on the wire")
	}
	if !s.verified {
		t.Fatal("verified = false, want the latch closed, or a reload during the recheck would stack a second check")
	}
	// The manual path dropped the scheduled silent retry.
	if !s.retryAt.IsZero() {
		t.Fatal("recheck left a scheduled retry behind")
	}
	waitRestore(t, &s)
	if s.notice.text() != locales.AccountNoticeSessionVerified() || !s.notice.ok {
		t.Fatalf("notice after recovery = %q, want the verified line", s.notice.text())
	}
	if s.session.state != loginLoggedIn {
		t.Fatalf("session = %+v, want the restored login", s.session)
	}
}

// A failed check schedules its own bounded retries: the first fires and
// rechecks, and a success there turns the notice verified with no retry left.
func TestAccountStateSchedulesSilentRetryAfterFailure(t *testing.T) {
	shrinkSessionRetries(t)
	appLanguage(t, "en")
	acct := &stubAccount{restoreSet: true, restoreSess: loggedOutSession(),
		restoreErr: &sessionUnverifiedError{class: errSessionRateLimited, cause: errors.New("429")}}
	s := newAccountStateWith(fakeKeys{}, acct)
	waitRestore(t, &s)
	// armRetry becomes a deadline on the next tick's frame.
	tester := ui.NewTester(func(c *ui.Context) { s.tick(c) }, 100, 100)
	tester.Frame()
	if s.retryCount != 1 {
		t.Fatalf("retryCount = %d, want the first retry armed", s.retryCount)
	}

	// The retry fires through tick's clock and rechecks silently: no checking
	// line replaces the failure while it runs, and the answer that lands is
	// another failure, so the same wording stays.
	tester.Advance(sessionRetryDelays[0])
	waitRestore(t, &s)
	if acct.restoreCalls != 2 {
		t.Fatalf("restoreCalls = %d, want the silent retry to have gone out", acct.restoreCalls)
	}
	if got := s.notice.text(); got != locales.AccountNoticeSessionUnverifiedRateLimited() {
		t.Fatalf("notice while retrying = %q, want the failure line kept", got)
	}

	// The second retry succeeds: the notice turns verified and the schedule
	// is gone.
	acct.restoreErr = nil
	acct.restoreSess = loggedInSession()
	tester.Frame() // arms the second retry
	tester.Advance(sessionRetryDelays[1])
	waitRestore(t, &s)
	if got := s.notice.text(); got != locales.AccountNoticeSessionVerified() {
		t.Fatalf("notice after the retry succeeded = %q, want verified", got)
	}
	if !s.retryAt.IsZero() || s.retryCount != 0 {
		t.Fatalf("retryAt=%v retryCount=%d, want the schedule cleared", s.retryAt, s.retryCount)
	}
}

// The retries stop after the last one in the schedule; no fourth attempt.
func TestAccountStateStopsRetryingAfterTheLastDelay(t *testing.T) {
	shrinkSessionRetries(t)
	appLanguage(t, "en")
	acct := &stubAccount{restoreSet: true, restoreSess: loggedOutSession(),
		restoreErr: &sessionUnverifiedError{class: errSessionNetwork, cause: errors.New("dial")}}
	s := newAccountStateWith(fakeKeys{}, acct)
	waitRestore(t, &s)
	// armRetry becomes a deadline on the next tick's frame; each fired
	// retry's failure arms the next one until the budget is spent, at which
	// point no deadline is left and the loop must stop instead of waiting
	// for an answer that will never come.
	tester := ui.NewTester(func(c *ui.Context) { s.tick(c) }, 100, 100)
	tester.Frame()
	for rounds := 0; !s.retryAt.IsZero(); rounds++ {
		if rounds > len(sessionRetryDelays) {
			t.Fatal("the retries never stopped")
		}
		tester.Advance(20 * time.Millisecond)
		waitRestore(t, &s)
		tester.Frame()
	}
	if acct.restoreCalls != 1+len(sessionRetryDelays) {
		t.Fatalf("restoreCalls = %d, want exactly %d (the first check plus the bounded retries)",
			acct.restoreCalls, 1+len(sessionRetryDelays))
	}
	if !s.retryAt.IsZero() {
		t.Fatal("a fourth retry was scheduled")
	}
}

// Logout clears the schedule: no retry may dial the server afterwards.
func TestAccountStateLogoutCancelsScheduledRetry(t *testing.T) {
	shrinkSessionRetries(t)
	acct := &stubAccount{restoreSet: true, restoreSess: loggedOutSession(),
		restoreErr: &sessionUnverifiedError{class: errSessionNetwork, cause: errors.New("dial")}}
	s := newAccountStateWith(fakeKeys{}, acct)
	waitRestore(t, &s)
	tester := ui.NewTester(func(c *ui.Context) { s.tick(c) }, 100, 100)
	tester.Frame() // arms the first retry
	if s.retryAt.IsZero() {
		t.Fatal("setup: no retry was scheduled")
	}

	s.logout()
	drain(t, &s)
	if !s.retryAt.IsZero() || s.retryCount != 0 {
		t.Fatalf("retryAt=%v retryCount=%d, want the schedule cleared by logout", s.retryAt, s.retryCount)
	}
	calls := acct.restoreCalls
	tester.Frame()
	tester.Advance(50 * time.Millisecond)
	if acct.restoreCalls != calls {
		t.Fatalf("restoreCalls = %d after logout, want it to stay %d", acct.restoreCalls, calls)
	}
}

// A check still on the wire when the logout lands must not come back to
// resurrect it: neither its failure notice nor a retry schedule.
func TestAccountStateLogoutDropsARestoreStillOnTheWire(t *testing.T) {
	shrinkSessionRetries(t)
	appLanguage(t, "en")
	gate := make(chan struct{})
	acct := &stubAccount{restoreGate: gate, restoreSet: true,
		restoreSess: loggedOutSession(),
		restoreErr: &sessionUnverifiedError{class: errSessionNetwork, cause: errors.New("dial")}}
	s := newAccountStateWith(fakeKeys{}, acct)

	// The startup check is frozen on the wire when the logout lands.
	s.logout()
	drain(t, &s)
	if got := s.notice.text(); got != locales.AccountNoticeLoggedOut() {
		t.Fatalf("notice = %q, want the logged-out line", got)
	}

	close(gate) // the frozen check answers with an unverified failure
	waitRestore(t, &s)
	if got := s.notice.text(); got != locales.AccountNoticeLoggedOut() {
		t.Fatalf("late answer resurrected the notice: %q, want the logged-out line kept", got)
	}
	if s.verifying {
		t.Fatal("verifying = true, want the late answer to have only closed it")
	}
	if !s.retryAt.IsZero() || s.armRetry || s.retryCount != 0 {
		t.Fatalf("late answer re-armed the retry schedule: retryAt=%v armRetry=%v retryCount=%d",
			s.retryAt, s.armRetry, s.retryCount)
	}
}

// The verified line fades on its own timer and takes nothing else with it.
func TestAccountStateVerifiedNoticeFades(t *testing.T) {
	shrinkSessionRetries(t)
	appLanguage(t, "en")
	acct := &stubAccount{restoreSet: true, restoreSess: loggedInSession()}
	s := newAccountStateWith(fakeKeys{}, acct)
	waitRestore(t, &s)
	if s.notice.text() != locales.AccountNoticeSessionVerified() {
		t.Fatalf("notice = %q, want the verified line", s.notice.text())
	}
	tester := ui.NewTester(func(c *ui.Context) { s.tick(c) }, 100, 100)
	tester.Frame()
	tester.Advance(sessionVerifiedLinger + 5*time.Millisecond)
	if s.notice.text() != "" {
		t.Fatalf("notice = %q after the linger, want none", s.notice.text())
	}
}
