package ui

// The account seam is where the Account page meets Bitwarden's account and
// login machinery. The pages talk only to accountBackend and never import
// internal/auth; backend_account_real.go is the one adapter that drives the
// real auth.LoginFlow plus the hello and secrets seams, and it does so through
// the small interfaces its constructor takes, so tests inject fakes and never
// touch the network, the credential store or a Hello prompt.

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

// label is the page's wording for the state.
func (s loginState) label() string {
	switch s {
	case loginLoggingIn:
		return "登录中"
	case loginNeeds2FA:
		return "等待两步验证码"
	case loginLoggedIn:
		return "已登录"
	default:
		return "未登录"
	}
}

// twoFactorPrompt is the challenge a login that stopped for a second factor
// hands to the prompt: the offered methods in server order plus the data each
// one needs. It mirrors auth.TwoFactorChallenge in local terms so the page
// types stay free of internal/auth.
type twoFactorPrompt struct {
	providers   []string // provider names, server order
	email       string   // where the email code goes (provider 1)
	duoURL      string   // frameless Duo URL (providers 2 and 6)
	webAuthnURL string   // web vault connector page (provider 7)
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
}

// accountBackend is what the Account page needs from a Bitwarden account.
// Calls are synchronous from the page's point of view and run off the UI
// thread there, because a real login hits the network and may block on a
// human answering 2FA.
type accountBackend interface {
	// session reports the current login state without changing it.
	session() (accountSession, error)
	// login starts an email/password login against serverURL. It returns
	// loginNeeds2FA when the account asks for a code, loginLoggedIn when it
	// is done.
	login(email, password, serverURL string) (accountSession, error)
	// cancelLogin abandons a pending login attempt. The session returns to
	// the one in force before the attempt: the signed-in account when a
	// relogin was under way, signed out otherwise. The in-memory credentials
	// and the pending 2FA challenge are dropped, so a later reload cannot
	// revive the attempt.
	cancelLogin() (accountSession, error)
	// submit2FA finishes a login that is waiting for a two-factor code. The
	// login window ticket owns it; the real backend reports it as not
	// implemented for now.
	submit2FA(code string) (accountSession, error)
	// logout forgets the session and deletes the signed-in account's stored
	// key, behind a Hello consent like every other key operation.
	logout() error
	// refreshKey re-derives the account key by logging in again with the
	// credentials kept in memory, so the local store matches the vault after
	// a password or KDF change. It needs a Hello consent before the write.
	refreshKey() error
	// relogin runs the login flow again from the beginning, with the same
	// in-memory credentials and the same Hello-before-write rule.
	relogin() error
}
