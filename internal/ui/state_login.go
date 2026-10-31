package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// loginWindow is the login window's working data: the form, the pending
// two-factor challenge, and any backend call in flight. The window is a modal
// child of the main window, so the account page behind it cannot be used until
// the attempt settles. Backend calls run off the UI thread (Windows Hello
// blocks on a human and a real login hits the network) and come back through
// results as closures that only the UI thread applies, like accountState.
//
// The master password and the 2FA code live here for the length of one
// attempt: they are cleared as soon as the outcome lands, and never reach a
// log, an error message, or the disk.
type loginWindow struct {
	// page is the account page the outcome lands on; its backend runs the
	// calls, so the login window and the account card share one auth session
	// and one credential store.
	page *accountState

	// showing tells whether the window is up. begin sets it; a successful
	// login and a cancel clear it.
	showing bool

	email    string
	password string
	// serverURL is the address the form shows; the default is the official
	// cloud, and checkServer normalizes whatever the human typed.
	serverURL string
	// serverErr is the checkServer refusal, shown under the address field so
	// a typo is visible where it was made; errorText says it at draw time.
	serverErr    error
	advancedOpen bool

	// challenge is the pending second factor, nil while the credentials form
	// shows.
	challenge *twoFactorPrompt
	// methodIndex picks among challenge.codeMethods().
	methodIndex int
	code        string
	// emailSent records that the code email went out, so the button does not
	// ask for a second one.
	emailSent bool

	busy   bool
	notice loginNotice

	results asyncRunner[func(*loginWindow)]

	// openWindow, closeWindow and resizeWindow are the GUI seams. They are nil
	// in tests, which drive the same state machine without a window server.
	openWindow   func()
	closeWindow  func()
	resizeWindow func(height int)
	// appliedHeight is the height the window is currently sized to, so a frame
	// that did not change the form does not resize the window again.
	appliedHeight int
}

// newLoginWindow wires the login window onto the account page.
func newLoginWindow(page *accountState) loginWindow {
	return loginWindow{
		page:      page,
		serverURL: defaultServerURL,
		results:   startAsync[func(*loginWindow)](4),
	}
}

// backend is the account seam the login window drives.
func (s *loginWindow) backend() accountBackend { return s.page.account }

// tick applies a finished backend call and keeps frames coming while one runs,
// the way accountState.tick does for the page's own calls. The main window
// calls it too, so a cancel that lands while the login window closes still
// reaches the account page.
func (s *loginWindow) tick(c *ui.Context) {
	s.results.tick(func(apply func(*loginWindow)) {
		s.busy = false
		apply(s)
	})
	if s.busy {
		c.After(60 * time.Millisecond)
	}
}

// async runs work on its own goroutine and hands the UI a closure that applies
// the outcome. One call at a time keeps a second Hello prompt from stacking on
// the first.
func (s *loginWindow) async(work func() func(*loginWindow)) {
	if s.busy {
		return
	}
	s.busy = true
	s.notice = loginNotice{}
	s.serverErr = nil
	s.results.run(work)
}

// begin opens the login window from the account page. It is a UI state change
// only: no call goes out until the form is submitted. A session parked on a
// two-factor challenge reopens on the code prompt, because the backend still
// holds that attempt.
func (s *loginWindow) begin() {
	s.showing = true
	s.busy = false
	s.notice = loginNotice{}
	s.serverErr = nil
	s.password = ""
	s.code = ""
	s.emailSent = false
	// Prefill the address so a re-login to a self-hosted server does not start
	// from the official cloud and fail on a typo the human has to retype.
	s.serverURL = s.page.effectiveServerURL()
	if sess := s.page.session; sess.state == loginNeeds2FA && sess.challenge != nil {
		s.challenge = sess.challenge
		s.methodIndex = 0
	} else {
		s.challenge = nil
	}
	if s.email == "" {
		s.email = s.page.session.email
	}
	if s.email == "" {
		// A configured-but-signed-out page has no session email; the stored
		// account's address is the best prefill for a re-login.
		if p := s.page.primary(); p != nil {
			s.email = p.email
		}
	}
	if s.openWindow != nil {
		s.openWindow()
	}
	// The window is created at the credentials height; apply the state's own
	// target in case Advanced was left open from the last attempt.
	s.appliedHeight = loginWindowHeight
	s.syncHeight()
}

// targetHeight is the outer height the form needs in its current state. The
// credentials step is the opening or the Advanced height; a two-factor prompt
// is the single-method height plus the rows its challenge added. On top of the
// stage go the rows its text adds: a visible status line (a retry hint, the
// email confirmation) and, at the credentials step, the refusal under the
// server address field. Keeping these in the target is what stops the form
// from falling back to the scroll view while a notice is up.
func (s *loginWindow) targetHeight() int {
	var h int
	if s.challenge == nil {
		h = loginWindowHeight
		if s.advancedOpen {
			h = loginWindowAdvancedHeight
			// The refusal only shows under the address field, which only the
			// Advanced fold holds.
			if s.serverErr != nil {
				h += loginWindowServerErrHeight
			}
		}
	} else {
		h = loginWindowTwoFactorHeight
		if len(s.challenge.codeMethods()) > 1 {
			h += loginWindowSegmentHeight
		}
		if m, ok := s.selectedMethod(); ok && m.provider == twoFactorEmail {
			h += loginWindowEmailHeight
		}
		if len(s.challenge.unsupported()) > 0 {
			h += loginWindowUnsupportedHeight
		}
	}
	return h + s.noticeHeight()
}

// noticeHeight is the rows the visible status line adds to the form, nil text
// adding none. The state machine has no Context to measure the line with, so
// it counts display cells (see noticeLines) on the same text the drawing says.
func (s *loginWindow) noticeHeight() int {
	text := s.notice.text()
	if text == "" {
		return 0
	}
	return loginWindowNoticeHeight + (noticeLines(text)-1)*loginWindowNoticeLineHeight
}

// noticeLines is how many lines text wraps to at the notice's width, which is
// the window minus the column's 16 DIP padding on both sides. The renderer's
// 12 DIP font fits 32 wide glyphs (CJK) or 56 half-width characters on that
// line: proportional Latin text is wider than a monospace cell, so the estimate
// uses 56 rather than the 64 a pure cell count would allow. The estimate counts
// a wide rune as two cells and any other as one, and the Scroll view still
// catches a text this estimate undercounts.
func noticeLines(text string) int {
	const cellsPerLine = 56
	cells := 0
	for _, r := range text {
		if isWideRune(r) {
			cells += 2
		} else {
			cells++
		}
	}
	if cells == 0 {
		return 0
	}
	return (cells + cellsPerLine - 1) / cellsPerLine
}

// isWideRune reports whether r draws two cells at the notice font: the CJK and
// Hangul blocks and the fullwidth forms, which are the scripts the catalog's
// notices use.
func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE6F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x20000 && r <= 0x3FFFD:
		return true
	}
	return false
}

// syncHeight resizes the window to the height its current state needs. The
// view calls it every frame, so the comparison against appliedHeight is what
// keeps a steady form from calling into the window server on each one.
func (s *loginWindow) syncHeight() {
	target := s.targetHeight()
	if target == s.appliedHeight {
		return
	}
	s.appliedHeight = target
	if s.resizeWindow != nil {
		s.resizeWindow(target)
	}
}

// submit starts the password login. The server address is checked first, so a
// typo is reported on the field instead of coming back as a network error.
func (s *loginWindow) submit() {
	backend := s.backend()
	email := strings.TrimSpace(s.email)
	password := s.password
	serverURL := strings.TrimSpace(s.serverURL)
	s.async(func() func(*loginWindow) {
		normalized, err := backend.checkServer(serverURL)
		if err != nil {
			return func(st *loginWindow) { st.failServer(err) }
		}
		sess, err := backend.login(email, password, normalized)
		return func(st *loginWindow) { st.applyLogin(normalized, sess, err) }
	})
}

// submitOnEnter starts the password login when Enter was pressed in one of the
// credential fields and the form is ready. The guard mirrors the primary
// button's, so Enter cannot bypass a disabled Login (an empty form, or a call
// already in flight) and Cancel is untouched.
func (s *loginWindow) submitOnEnter(entered bool) bool {
	if !entered || s.busy || strings.TrimSpace(s.email) == "" || s.password == "" {
		return false
	}
	s.submit()
	return true
}

// failServer shows a rejected address on the field that produced it, opened so
// it is on screen.
func (s *loginWindow) failServer(err error) {
	s.advancedOpen = true
	s.serverErr = err
	s.notice = loginNotice{kind: loginNoticeBadServer}
	s.syncHeight()
}

// applyLogin lands a password-step outcome.
func (s *loginWindow) applyLogin(serverURL string, sess accountSession, err error) {
	if err != nil {
		s.fail(err)
		return
	}
	s.serverURL = serverURL
	s.password = ""
	switch sess.state {
	case loginLoggedIn:
		s.finishLogin(sess)
	case loginNeeds2FA:
		s.challenge = sess.challenge
		s.methodIndex = 0
		s.emailSent = false
		s.notice = loginNotice{kind: loginNoticeTwoFactor}
		s.syncHeight()
	default:
		s.notice = loginNotice{kind: loginNoticeSigningIn}
	}
}

// fail keeps the form up for a correction; the backend already dropped the
// attempt it could not finish. The message is the backend's wording: a failure
// must not carry the password or the code it was called with.
func (s *loginWindow) fail(err error) {
	s.notice = loginNotice{kind: loginNoticeFailure, failure: err}
	s.syncHeight()
}

// submit2FA answers the challenge with the code in the form. The chosen method
// decides which provider the submission echoes; the backend refuses the ones
// with no biowarden path, so no browser opens from here.
func (s *loginWindow) submit2FA() {
	m, ok := s.selectedMethod()
	if !ok {
		return
	}
	backend := s.backend()
	code := strings.TrimSpace(s.code)
	s.async(func() func(*loginWindow) {
		sess, err := backend.submit2FA(m.provider, code)
		return func(st *loginWindow) { st.applyTwoFactor(sess, err) }
	})
}

// submitTwoFactorOnEnter answers the challenge when Enter was pressed in the
// code field and a code is present. The unsupported-method panel has no code
// field, so Enter there does nothing at all.
func (s *loginWindow) submitTwoFactorOnEnter(entered bool) bool {
	if !entered || s.busy || strings.TrimSpace(s.code) == "" {
		return false
	}
	if _, ok := s.selectedMethod(); !ok {
		return false
	}
	s.submit2FA()
	return true
}

// selectedMethod is the code method the form is offering, of the challenge's
// code methods in server order.
func (s *loginWindow) selectedMethod() (twoFactorMethod, bool) {
	methods := s.challenge.codeMethods()
	if len(methods) == 0 {
		return twoFactorMethod{}, false
	}
	if s.methodIndex < 0 || s.methodIndex >= len(methods) {
		s.methodIndex = 0
	}
	return methods[s.methodIndex], true
}

// applyTwoFactor lands a 2FA submission. A rejected code and a fresh server
// challenge both arrive as the needs-2FA state with a challenge, so the prompt
// stays up for a corrected code; an error keeps it up too, with the backend's
// wording.
func (s *loginWindow) applyTwoFactor(sess accountSession, err error) {
	if err != nil {
		s.code = ""
		s.fail(err)
		return
	}
	switch sess.state {
	case loginLoggedIn:
		s.finishLogin(sess)
	case loginNeeds2FA:
		s.code = ""
		if sess.challenge != nil {
			// The server may have switched methods; follow its new challenge.
			s.challenge = sess.challenge
			s.methodIndex = 0
			s.emailSent = false
			s.syncHeight()
		}
		s.notice = loginNotice{kind: loginNoticeCodeRejected}
	default:
		s.notice = loginNotice{kind: loginNoticeVerifying}
	}
}

// sendEmailCode asks the server to mail the code for the pending email
// challenge. It goes out once per challenge: this state refuses a repeat, and
// the backend and the flow refuse one too.
func (s *loginWindow) sendEmailCode() {
	if s.emailSent {
		return
	}
	backend := s.backend()
	s.async(func() func(*loginWindow) {
		err := backend.sendEmailCode()
		return func(st *loginWindow) { st.applyEmailSent(err) }
	})
}

func (s *loginWindow) applyEmailSent(err error) {
	if err != nil {
		s.fail(err)
		return
	}
	s.emailSent = true
	s.notice = loginNotice{kind: loginNoticeEmailSent, ok: true}
	if m, ok := s.selectedMethod(); ok && m.email != "" {
		s.notice = loginNotice{kind: loginNoticeEmailSentTo, args: []string{m.email}, ok: true}
	}
	s.syncHeight()
}

// finishLogin hands the session to the account page, which reloads the stored
// keys, and closes the window. The backend wrote the key behind a Hello
// consent already; the page only mirrors the result.
func (s *loginWindow) finishLogin(sess accountSession) {
	s.page.session = sess
	s.page.reload()
	if sess.warning != nil {
		// The login itself succeeded; only keeping the session failed.
		s.page.notice = failureNotice(sess.warning)
	} else {
		s.page.notice = pageNotice{kind: noticeAccountLoggedIn, args: []string{sess.email}, ok: true}
	}
	s.clearSecrets()
	s.showing = false
	s.close()
}

// cancel abandons the attempt: the backend drops the parked flow, the
// challenge and the in-memory password, and the window closes.
func (s *loginWindow) cancel() {
	backend := s.backend()
	s.async(func() func(*loginWindow) {
		sess, err := backend.cancelLogin()
		return func(st *loginWindow) { st.applyCancel(sess, err) }
	})
}

func (s *loginWindow) applyCancel(sess accountSession, err error) {
	if err != nil {
		s.fail(err)
		return
	}
	s.page.session = sess
	s.page.notice = pageNotice{kind: noticeAccountCancelled, ok: true}
	s.clearSecrets()
	s.showing = false
	s.close()
}

// requestClose handles the human closing the window: it means the same as
// Cancel. The window never closes right away; the state machine destroys it
// once the backend confirmed the attempt is abandoned, so a cancel that fails
// leaves the window up with its message.
func (s *loginWindow) requestClose() {
	if s.busy {
		s.notice = loginNotice{kind: loginNoticeBusy}
		return
	}
	s.cancel()
}

// clearSecrets drops the values that must not outlive one attempt.
func (s *loginWindow) clearSecrets() {
	s.password, s.code = "", ""
	s.emailSent = false
	s.challenge = nil
	s.methodIndex = 0
	s.busy = false
}

// close asks the GUI to destroy the window. It is a no-op in tests.
func (s *loginWindow) close() {
	if s.closeWindow != nil {
		s.closeWindow()
	}
}
