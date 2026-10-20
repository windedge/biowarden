package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// accountPage is the landing page: who is signed in, whether a key is stored,
// and the v1 escape hatches behind the Advanced disclosure. It keeps the
// reference KeyTab's store-and-verify flow, now under the account it belongs
// to.
func (app *appState) accountPage(c *ui.Context) {
	a := &app.account
	ui.Column(c).Grow(1).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16).Gap(12).Children(func() {
				ui.Text(c, locales.AccountTitle()).FontSize(18).Bold()
				muted(c, locales.AccountIntro())
				notice(c, a.notice.text(), a.notice.ok)

				a.accountCard(c, &app.login)
				a.advanced(c)
			})
		})
	})
}

// accountCard is the login state: identity and server when signed in, the
// stored-key "configured" state when a key exists without a live session, and
// the sign-in guidance with nothing stored. The wording for all three comes
// from accountCardViewFor; the login window owns the form and the two-factor
// prompt, so the card only mirrors the session the backend reports. Sign out
// is the card's destructive action, gated by Windows Hello inside removeAccount.
func (a *accountState) accountCard(c *ui.Context, login *loginWindow) {
	t := c.Theme()
	groupBox(c, locales.AccountCardGroup(), func() {
		switch a.session.state {
		case loginNeeds2FA:
			// The login window answers challenges; this state means the
			// attempt is parked with nobody showing the prompt.
			a.identityRow(c, t.Warning)
			if a.session.twoFAMethod != "" {
				muted(c, locales.AccountCard2FAMethod(a.session.twoFAMethod))
			}
			muted(c, locales.AccountCard2FAUnfinished())
			ui.Row(c).Gap(10).Children(func() {
				if ui.PrimaryButton(c, locales.AccountCardContinue()).Disabled(a.busy).Clicked() {
					login.begin()
					c.Invalidate()
				}
				if ui.Button(c, locales.AccountCardCancel()).Disabled(a.busy).Clicked() {
					login.cancel()
					c.Invalidate()
				}
			})
			return
		}

		if a.verifying {
			muted(c, locales.AccountCardVerifying())
		}
		primary := a.primary()
		v := accountCardViewFor(a.session, primary)
		// Sign out is the low-frequency corner action on the title row, out of
		// the main flow; the card has no other action row.
		a.accountIdentity(c, v, t, func() {
			if !v.removable {
				return
			}
			if ui.Button(c, locales.AccountCardSignOut()).Disabled(a.busy).
				FontSize(t.Rem(0.85)).Clicked() {
				a.removeAccount(primary)
				c.Invalidate()
			}
		})
		// No action row when the view names none: refreshing the key goes
		// through sign-out, then sign-in, and unlocking never needs the login
		// window.
		if v.loginLabel == "" {
			return
		}
		// A view that names a login action gets the same row: the signed-out
		// card, and a signed-in card whose key is gone (removed on another
		// device), which would otherwise be a dead end with no button.
		// Every write in the GUI reloads the card by itself, so the row holds
		// only the login action.
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.PrimaryButton(c, v.loginLabel).Disabled(a.busy).Clicked() {
				login.begin()
				c.Invalidate()
			}
		})
	})
}

// accountIdentity draws the title, the identity/detail line and any hint for
// one card view. The title color follows the state: green for a verified
// login, the normal text color for the configured and signed-out states.
// corner, when not nil, draws at the right end of the title row.
func (a *accountState) accountIdentity(c *ui.Context, v accountCardView, t *ui.Theme, corner func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		if v.identity {
			ui.Avatar(c, avatarName(a.session.email), nil)
		}
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			color := t.Text
			if v.state == cardSignedIn {
				color = t.Success
			}
			ui.Text(c, v.title).Bold().TextColor(color)
			if v.detail != "" {
				muted(c, v.detail)
			}
			if v.hint != "" {
				muted(c, v.hint)
			}
		})
		if corner != nil {
			corner()
		}
		if a.session.state == loginLoggingIn {
			ui.Spinner(c).Label(locales.LoginSpinnerLoggingIn())
		}
	})
}

// identityRow is the avatar, email, server and state line the 2FA branch
// draws while it waits for a code.
func (a *accountState) identityRow(c *ui.Context, stateColor ui.Color) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Avatar(c, avatarName(a.session.email), nil)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, a.session.email).Bold()
			muted(c, locales.AccountServerLabel(a.session.serverURL))
		})
		ui.Text(c, a.session.state.label()).FontSize(12).TextColor(stateColor)
	})
}

// advanced holds the two v1 escape hatches: the extraction script for a
// browser extension the auto-login path cannot reach yet, and the manual
// paste form. Both were the whole Keys page before; folding them away keeps
// the default account page short.
func (a *accountState) advanced(c *ui.Context) {
	ui.Collapsible(c, locales.AccountAdvanced(), &a.advancedOpen, func() {
		ui.Column(c).Gap(12).Children(func() {
			ui.Text(c, locales.AccountExtractTitle()).Bold()
			muted(c, locales.AccountExtractHint())
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, locales.AccountCopyScript()).Clicked() {
					mygo.Clipboard.WriteText(extractSnippet)
					a.copiedAt = time.Now()
				}
				if !a.copiedAt.IsZero() {
					ui.Text(c, locales.AccountCopied(len(extractSnippet))).
						FontSize(12).TextColor(c.Theme().Success)
				}
			})
			// The reference hides the snippet because a long script wrecks its
			// fixed-size SwiftUI layout; the page scrolls here, but the script
			// still stays behind a collapsed disclosure so it can be audited.
			ui.Collapsible(c, locales.AccountViewScript(), &a.scriptOpen, func() {
				ui.TextArea(c, &a.scriptBody).ReadOnly(true).Lines(6, 14).
					FontSize(11).Placeholder(locales.AccountScriptPreview())
			})
			ui.Divider(c)
			a.manualEntry(c)
		})
	})
}

// manualEntry is the v1 form and the stored-account list, kept as the
// fallback for accounts the script path still handles.
func (a *accountState) manualEntry(c *ui.Context) {
	ui.Text(c, locales.AccountManualTitle()).Bold()
	muted(c, locales.AccountManualHint())
	ui.Form(c, func() {
		ui.Field(c, locales.AccountManualUserId(), func() {
			ui.TextInput(c.Key("userId"), &a.userId).
				Placeholder(locales.AccountManualUserIdPlaceholder())
		})
		ui.Field(c, locales.AccountManualKey(), func() {
			ui.TextInput(c.Key("keyB64"), &a.keyB64).Password().
				Placeholder(locales.AccountManualKeyPlaceholder())
		})
	})
	ready := strings.TrimSpace(a.userId) != "" && strings.TrimSpace(a.keyB64) != ""
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.PrimaryButton(c, locales.AccountManualSave()).Disabled(!ready || a.busy).Clicked() {
			a.saveKey()
			// The consent runs off the UI thread, so ask for the frame
			// that starts polling for its result.
			c.Invalidate()
		}
		if a.busy {
			ui.Spinner(c).Label(locales.AccountVerifyingSpinner())
			ui.Text(c, locales.AccountCallingHello()).
				FontSize(12).TextColor(c.Theme().TextMuted)
		}
	})
	muted(c, locales.AccountManualWarning())
	muted(c, locales.AccountManualEnableBiometric())

	if len(a.accounts) == 0 {
		muted(c, locales.AccountNoAccounts())
		return
	}
	ui.Divider(c)
	muted(c, locales.AccountStoredAccounts())
	for _, acc := range a.accounts {
		acc := acc
		ui.Row(c).Key(acc.userID).Gap(12).Padding(8, 6).
			AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, acc.userID).FontSize(12).SingleLine().Tooltip(acc.userID)
				meta := locales.AccountRowCreated(acc.created)
				if acc.email != "" {
					meta = locales.AccountRowMeta(acc.email, meta)
				}
				muted(c, meta)
			})
			if ui.Button(c, locales.AccountTestUnlock()).Disabled(a.busy).Clicked() {
				a.testUnlock(acc)
				c.Invalidate()
			}
			// Selecting a row foregrounds it in the account card; it is the
			// whole of account switching without parallel sessions.
			if primary := a.primary(); primary == nil || primary.userID != acc.userID {
				if ui.Button(c, locales.AccountSetCurrent()).Disabled(a.busy).Clicked() {
					a.selectAccount(acc.userID)
					c.Invalidate()
				}
			}
			// Removal runs off the UI thread, so the row is dropped by
			// a later tick rather than while this list is built.
			if ui.Button(c, locales.Delete()).Disabled(a.busy).Clicked() {
				a.removeAccount(acc)
				c.Invalidate()
			}
		})
	}
}

// avatarName gives ui.Avatar a name to take initials and a color from. An
// empty email still needs something, or the avatar draws nothing.
func avatarName(email string) string {
	if strings.TrimSpace(email) == "" {
		return "biowarden"
	}
	return email
}
