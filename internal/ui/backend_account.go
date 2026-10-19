package ui

import "github.com/windedge/biowarden/locales"

// The account seam is where the Account page and the login window meet
// Bitwarden's account and login machinery. The pages talk only to
// accountBackend and never import internal/auth; backend_account_real.go is
// the one adapter that drives the real auth.LoginFlow plus the hello and
// secrets seams, and it does so through the small interfaces its constructor
// takes, so tests inject fakes and never touch the network, the credential
// store or a Hello prompt.

// defaultServerURL is the official Bitwarden server, shown until an account
// (or a later settings ticket) says otherwise.
const defaultServerURL = "https://vault.bitwarden.com"

// loginState is how far a login has come. It mirrors the states the Account
// page draws; the transitions live in the backend.
type loginState int

const (
	loginLoggedOut loginState = iota
	loginLoggingIn
	loginNeeds2FA
	loginLoggedIn
)

// label is the page's wording for the state, said in the language the app
// shows.
func (s loginState) label() string {
	switch s {
	case loginLoggingIn:
		return locales.LoginStateLoggingIn()
	case loginNeeds2FA:
		return locales.LoginStateNeeds2FA()
	case loginLoggedIn:
		return locales.LoginStateLoggedIn()
	default:
		return locales.LoginStateLoggedOut()
	}
}

// Second-factor provider numbers, as they appear in the Bitwarden protocol.
// They mirror auth.TwoFactorProvider, which the pages may not import, so the
// numbers a page branches on live here. The mapping is fixed by the server's
// TwoFactorProviders payload.
const (
	twoFactorAuthenticator = 0
	twoFactorEmail         = 1
	twoFactorDuo           = 2
	twoFactorYubiKey       = 3
	twoFactorOrgDuo        = 6
	twoFactorWebAuthn      = 7
	twoFactorRecoveryCode  = 8
)

// Interactions a challenge's methods need, mirroring
// auth.ChallengeInteraction. Only interactionCode is one this UI can finish.
const (
	interactionCode    = "code"
	interactionBrowser = "browser"
)

// twoFactorMethod is one offered second factor in local terms: the provider
// number a submission must echo back, its interaction, and the payload the
// prompt needs to render it.
type twoFactorMethod struct {
	provider    int
	name        string // "authenticator", "email", "yubikey", ...
	interaction string
	email       string // where the email code goes (provider 1)
	browserURL  string // the page a browser method would open
}

// takesCode reports whether the method is answered with a short code the
// human types, the only interaction the login window can finish.
func (m twoFactorMethod) takesCode() bool { return m.interaction == interactionCode }

// twoFactorPrompt is the challenge a login that stopped for a second factor
// hands to the prompt: the offered methods in the server's order. It mirrors
// auth.TwoFactorChallenge in local terms so the page types stay free of
// internal/auth.
type twoFactorPrompt struct {
	methods []twoFactorMethod
}

// codeMethods lists the offered methods that take a short code, in server
// order. Rendering these is what the login window's 2FA form is for.
func (p *twoFactorPrompt) codeMethods() []twoFactorMethod {
	if p == nil {
		return nil
	}
	var out []twoFactorMethod
	for _, m := range p.methods {
		if m.takesCode() {
			out = append(out, m)
		}
	}
	return out
}

// unsupported lists the offered methods biowarden has no path for, so the
// prompt can name them instead of silently hiding them.
func (p *twoFactorPrompt) unsupported() []twoFactorMethod {
	if p == nil {
		return nil
	}
	var out []twoFactorMethod
	for _, m := range p.methods {
		if !m.takesCode() {
			out = append(out, m)
		}
	}
	return out
}

// first names the first offered method, for the page's one-line state.
func (p *twoFactorPrompt) first() string {
	if p == nil || len(p.methods) == 0 {
		return ""
	}
	return p.methods[0].name
}

// method finds one offered method by provider number.
func (p *twoFactorPrompt) method(provider int) (twoFactorMethod, bool) {
	if p == nil {
		return twoFactorMethod{}, false
	}
	for _, m := range p.methods {
		if m.provider == provider {
			return m, true
		}
	}
	return twoFactorMethod{}, false
}

// accountSession is what the Account page shows: the login state and the
// identity behind it. The key itself never travels through here.
type accountSession struct {
	state     loginState
	email     string
	serverURL string
	// userID is the signed-in account's Bitwarden id, so logout can delete
	// exactly that account's stored key.
	userID      string
	twoFAMethod string // how the pending 2FA code is delivered, for the prompt
	// challenge is the pending second factor, nil outside loginNeeds2FA.
	challenge *twoFactorPrompt
	// warning is a non-fatal notice about a login that succeeded anyway, e.g.
	// a session that could not be persisted. nil when nothing needs saying; the
	// UI maps it to wording.
	warning error
}

// accountBackend is what the Account page and the login window need from a
// Bitwarden account. Calls are synchronous from the caller's point of view and
// run off the UI thread there, because a real login hits the network and may
// block on a human answering 2FA.
type accountBackend interface {
	// session reports the current login state without changing it.
	session() (accountSession, error)
	// restoreSession re-validates the login persisted in the OS credential
	// store (a refresh token, never the master password) and returns the
	// session to show. The account page calls it once, on the first load. It
	// reports errSessionRevoked when the server rejected the token (the stored
	// session is cleared, the key kept) and errSessionUnverified when the
	// server could not be reached (the stored session is kept).
	restoreSession() (accountSession, error)
	// checkServer normalizes a user-entered server address the way login will,
	// so the login window can report a bad one before any call goes out. The
	// normalized address is what login should be given.
	checkServer(serverURL string) (string, error)
	// login starts an email/password login against serverURL. It returns
	// loginNeeds2FA when the account asks for a code, loginLoggedIn when it
	// is done.
	login(email, password, serverURL string) (accountSession, error)
	// cancelLogin abandons a pending login attempt. The session returns to
	// the one in force before the attempt: the signed-in account when a fresh
	// sign-in was under way, signed out otherwise. The parked flow and the
	// pending 2FA challenge are dropped, so a later reload cannot revive the
	// attempt.
	cancelLogin() (accountSession, error)
	// submit2FA finishes a login that is waiting for a two-factor code,
	// answering the offered method identified by provider (see the
	// twoFactor* constants). Like login it consents through Windows Hello
	// before writing the key, and returns loginNeeds2FA when the server
	// rejected the code or asked for another method.
	submit2FA(provider int, code string) (accountSession, error)
	// sendEmailCode asks the server to mail the code for the pending email
	// challenge. A repeat call is a no-op, so the code goes out once.
	sendEmailCode() error
	// logout forgets the session and deletes the signed-in account's stored
	// key, behind a Hello consent like every other key operation.
	logout() error
	// forgetSession drops the persisted login for one account whose stored key
	// was just deleted, so a restart cannot restore a session for a key that no
	// longer exists. It touches no key material and asks for no consent.
	forgetSession(userID string) error
}
