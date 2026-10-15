package ui

// The account seam is where the Account page meets Bitwarden's account and
// login machinery. This ticket defines the interface and ships the fake; the
// auth ticket replaces the fake with an implementation of internal/auth,
// without the pages changing. The UI must never import internal/auth itself:
// every account capability travels through this interface.

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

// accountSession is what the Account page shows: the login state and the
// identity behind it. The key itself never travels through here.
type accountSession struct {
	state       loginState
	email       string
	serverURL   string
	twoFAMethod string // how the pending 2FA code is delivered, for the prompt
}

// accountBackend is what the Account page needs from a Bitwarden account.
// Calls are synchronous from the page's point of view and run off the UI
// thread there, because a real login hits the network and may block on a
// human answering 2FA.
type accountBackend interface {
	// session reports the current login state without changing it.
	session() (accountSession, error)
	// login starts an email/password login. It returns loginNeeds2FA when
	// the account asks for a code, loginLoggedIn when it is done.
	login(email, password string) (accountSession, error)
	// submit2FA finishes a login that is waiting for a two-factor code.
	submit2FA(code string) (accountSession, error)
	// logout forgets the session, leaving stored keys alone.
	logout() error
	// refreshKey re-reads the account key from Bitwarden so the local store
	// matches the vault.
	refreshKey() error
	// relogin runs the login flow again from the beginning.
	relogin() error
}
