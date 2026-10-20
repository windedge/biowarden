package ui

import "github.com/windedge/biowarden/locales"

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
	// loginLabel labels the signed-out card's primary button, which opens the
	// login window. The other states offer no sign-in button: refreshing the
	// key goes through sign-out, then sign-in.
	loginLabel string
	// primary marks the login button as the card's primary action, which only
	// the signed-out state needs; the other states lead with the stored key.
	primary bool
	// removable offers the sign-out button for a stored key. Only a stored
	// key can be signed out, so the signed-out state hides it.
	removable bool
	// identity draws the avatar row from the session's email.
	identity bool
}

// accountCardViewFor picks the card's state and wording from the login session
// and the stored account the card describes (which may be nil). A signed-in
// session wins; otherwise a stored key means "configured"; otherwise the page
// asks the user to sign in. The wording comes from the catalog here, at draw
// time, so a language change moves the card with it.
func accountCardViewFor(sess accountSession, primary *accountRow) accountCardView {
	if sess.state == loginLoggedIn || sess.state == loginLoggingIn {
		detail := locales.AccountCardSignedInNoKey(sess.email, sess.serverURL)
		if primary != nil {
			detail = locales.AccountCardSignedInKey(sess.email, sess.serverURL, primary.created)
		}
		return accountCardView{
			state:     cardSignedIn,
			title:     locales.AccountCardSignedIn(),
			detail:    detail,
			removable: primary != nil,
			identity:  true,
		}
	}
	if primary != nil {
		who := primary.email
		if who == "" {
			who = shortID(primary.userID)
		}
		server := primary.serverURL
		if server == "" {
			server = defaultServerURL
		}
		return accountCardView{
			state:     cardConfigured,
			title:     locales.AccountCardConfigured(),
			detail:    locales.AccountCardConfiguredDetail(who, server, primary.created),
			hint:      locales.AccountCardConfiguredHint(),
			removable: true,
		}
	}
	return accountCardView{
		state:      cardSignedOut,
		title:      locales.AccountCardSignedOut(),
		hint:       locales.AccountNoKeyHint(),
		loginLabel: locales.LoginTitle(),
		primary:    true,
	}
}
