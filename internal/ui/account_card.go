package ui

// accountCardState is which of the account card's three shapes applies. It is
// derived from the login session plus the stored key, so the card can say
// "configured, unlocking works" instead of "sign in" whenever a key is
// present.
type accountCardState int

const (
	// cardSignedOut: no session and no stored key: the sign-in guidance.
	cardSignedOut accountCardState = iota
	// cardConfigured: a stored key but no verified session. Unlocking works;
	// only refreshing the key needs a sign-in.
	cardConfigured
	// cardSignedIn: a verified session (with or without a stored key).
	cardSignedIn
)

// accountCardView is the account card's wording for one state. accountCardViewFor
// is a pure function so the three states can be tested without a window.
type accountCardView struct {
	state  accountCardState
	title  string
	detail string
	hint   string
	// loginLabel is the button that opens the sign-in window. The configured
	// state words it as a refresh, because that is the only reason to sign in
	// again when a key is already stored.
	loginLabel string
	// relogin offers the secondary sign-in button beside a signed-in identity.
	relogin bool
	// logout offers the destructive button that deletes the stored key.
	logout bool
	// identity draws the avatar row from the session's email.
	identity bool
}

// accountCardViewFor picks the card's state and wording from the login session
// and the account the key card foregrounds (which may be nil). A signed-in
// session wins; otherwise a stored key means "configured"; otherwise the page
// asks the user to sign in.
func accountCardViewFor(sess accountSession, primary *accountRow) accountCardView {
	switch sess.state {
	case loginLoggedIn, loginLoggingIn:
		detail := sess.email + " · 服务器 " + sess.serverURL
		if primary != nil {
			detail += " · 密钥存储于 " + primary.created
		} else {
			detail += " · 本机尚未保存密钥"
		}
		return accountCardView{
			state:    cardSignedIn,
			title:    "已登录",
			detail:   detail,
			relogin:  true,
			logout:   true,
			identity: true,
		}
	case loginNeeds2FA:
		// The 2FA branch renders its own prompt; this keeps the pure function
		// total for callers that ask anyway.
		return accountCardView{state: cardSignedIn, title: "等待两步验证码", identity: true}
	}
	if primary != nil {
		who := primary.email
		if who == "" {
			who = shortID(primary.userID)
		}
		return accountCardView{
			state:      cardConfigured,
			title:      "已配置",
			detail:     who + " · 密钥存储于 " + primary.created + " · 解锁可用",
			hint:       "解锁已经可用，无需重新登录；只有主密码或 KDF 改过、需要刷新密钥时才要重新登录。",
			loginLabel: "重新登录（用于刷新密钥）",
		}
	}
	return accountCardView{
		state:      cardSignedOut,
		title:      "尚未登录",
		detail:     "登录 Bitwarden 账号后，biowarden 会自动取出并保存密钥，无需逐项手动配置。",
		loginLabel: "登录 Bitwarden 账号",
	}
}
