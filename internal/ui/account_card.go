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
	// loginLabel is the button that opens the sign-in window. The configured
	// state words it as a re-login, because refreshing the key is the only
	// reason to sign in again when a key is already stored.
	loginLabel string
	// logout offers the destructive button that deletes the stored key.
	logout bool
	// identity draws the avatar row from the session's email.
	identity bool
}

// accountCardViewFor picks the card's state and wording from the login session
// and the account the key card foregrounds (which may be nil). A signed-in
// session wins; otherwise a stored key means "configured"; otherwise the page
// asks the user to sign in. The wording comes from the catalog here, at draw
// time, so a language change moves the card with it.
func accountCardViewFor(sess accountSession, primary *accountRow) accountCardView {
	switch sess.state {
	case loginLoggedIn, loginLoggingIn:
		detail := locales.AccountCardSignedInNoKey(sess.email, sess.serverURL)
		if primary != nil {
			detail = locales.AccountCardSignedInKey(sess.email, sess.serverURL, primary.created)
		}
		return accountCardView{
			state:    cardSignedIn,
			title:    locales.AccountCardSignedIn(),
			detail:   detail,
			logout:   true,
			identity: true,
		}
	case loginNeeds2FA:
		// The 2FA branch renders its own prompt; this keeps the pure function
		// total for callers that ask anyway.
		return accountCardView{state: cardSignedIn, title: locales.AccountCard2FATitle(), identity: true}
	}
	if primary != nil {
		who := primary.email
		if who == "" {
			who = shortID(primary.userID)
		}
		return accountCardView{
			state:      cardConfigured,
			title:      locales.AccountCardConfigured(),
			detail:     locales.AccountCardConfiguredDetail(who, primary.created),
			hint:       locales.AccountCardConfiguredHint(),
			loginLabel: locales.AccountCardReLogin(),
		}
	}
	return accountCardView{
		state:      cardSignedOut,
		title:      locales.AccountCardSignedOut(),
		detail:     locales.AccountCardSignedOutDetail(),
		loginLabel: locales.LoginTitle(),
	}
}
