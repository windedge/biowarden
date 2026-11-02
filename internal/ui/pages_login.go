package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/windedge/biowarden/locales"
)

// loginView draws the login window: the credentials form first, then the
// two-factor prompt the server asked for. It is the only login entry; the
// account page opens this window instead of drawing a form of its own.
// iconEye is the password field's reveal toggle, drawn like the nav icons:
// a 24x24 stroked eye, an almond around a pupil.
var iconEye = icon(`<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>`)

func (s *loginWindow) loginView(c *ui.Context) {
	applyPalette(c)
	// Esc means what the window's X does: a close request. Busy swallows it,
	// the same way the Cancel button is disabled then.
	c.OnShortcut(0, ui.KeyEscape, func() {
		if s.busy {
			return
		}
		s.requestClose()
	})
	s.tick(c)
	// A frame after the Advanced disclosure toggled is where the window learns
	// its new height; syncHeight keeps a steady form from resizing every frame.
	s.syncHeight()
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		// The window follows the form's height (syncHeight above), so the scroll
		// view only catches the rare hint that wraps past its measured target.
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(pagePadding).Gap(spaceMedium).Children(func() {
				ui.Text(c, locales.LoginTitle()).FontSize(t.Rem(fontEmphasis)).Bold()
				muted(c, locales.LoginSubtitle())
				if s.challenge == nil {
					s.credentialsForm(c)
				} else {
					s.twoFactorForm(c)
				}
			})
		})
	})
}

// credentialsForm is the email/password step, with the server address behind
// the Advanced disclosure.
func (s *loginWindow) credentialsForm(c *ui.Context) {
	t := c.Theme()
	// entered records Enter in any of the three fields; .Submitted is mygo's
	// Enter-in-a-single-line-input signal, so the form needs no key hook.
	entered := false
	ui.Form(c, func() {
		ui.Field(c, locales.LoginEmail(), func() {
			entered = ui.TextInput(c.Key("login-email"), &s.email).
				Placeholder(locales.LoginEmailPlaceholder()).Disabled(s.busy).AutoFocus().Submitted() || entered
		})
		ui.Field(c, locales.LoginPassword(), func() {
			ui.Row(c).Gap(spaceHair).AlignItems(ui.Center).Children(func() {
				// The eye decides whether the field hides its text; only the
				// frames that hide it are password fields.
				in := ui.TextInput(c.Key("login-password"), &s.password)
				if !s.showPassword {
					in = in.Password()
				}
				entered = in.Placeholder(locales.LoginPasswordPlaceholder()).
					Disabled(s.busy).Grow(1).Submitted() || entered
				plainButton(c, "", s.busy).Children(func() {
					ui.Icon(c, iconEye).FontSize(t.Rem(fontBody)).Shrink(0)
				}).Tooltip(locales.LoginTogglePassword()).OnClick(func() {
					s.showPassword = !s.showPassword
					c.Invalidate()
				})
			})
		})
	})
	ui.Collapsible(c, locales.LoginAdvanced(), &s.advancedOpen, func() {
		ui.Form(c, func() {
			ui.Field(c, locales.LoginServer(), func() {
				entered = ui.TextInput(c.Key("login-server"), &s.serverURL).
					Placeholder(defaultServerURL).Disabled(s.busy).Submitted() || entered
			}).Error(errorText(s.serverErr))
		})
		muted(c, locales.LoginServerHint())
	})
	noticeOf(c, s.notice)
	ready := strings.TrimSpace(s.email) != "" && s.password != ""
	if s.submitOnEnter(entered) {
		c.Invalidate()
	}
	ui.Row(c).Gap(spaceRow).AlignItems(ui.Center).Children(func() {
		if primaryButton(c, locales.LoginSubmit(), !ready || s.busy).Clicked() {
			s.submit()
			c.Invalidate()
		}
		if plainButton(c, locales.LoginCancel(), s.busy).Clicked() {
			s.cancel()
			c.Invalidate()
		}
		if s.busy {
			ui.Spinner(c).Label(locales.LoginSpinnerLoggingIn())
			ui.Text(c, locales.LoginBusySigningIn()).FontSize(t.Rem(fontSecondary)).TextColor(t.TextMuted)
		}
	})
}

// twoFactorForm renders the pending challenge. Duo and WebAuthn have no path
// in biowarden, so they are named but not offered; when the server offers
// nothing else, the window says so and keeps Cancel as the only exit.
func (s *loginWindow) twoFactorForm(c *ui.Context) {
	t := c.Theme()
	methods := s.challenge.codeMethods()
	if len(methods) == 0 {
		s.unsupportedMethods(c)
		return
	}
	if len(methods) > 1 {
		labels := make([]string, 0, len(methods))
		for _, m := range methods {
			labels = append(labels, methodTitle(m))
		}
		ui.Segmented(c, &s.methodIndex, labels...).Label(locales.LoginTwoFactorMethod())
	}
	m, _ := s.selectedMethod()
	muted(c, locales.LoginTwoFactorPrompt(methodTitle(m)))
	entered := false
	ui.Form(c, func() {
		ui.Field(c, methodFieldLabel(m), func() {
			entered = ui.TextInput(c.Key("login-2fa"), &s.code).
				Placeholder(methodHint(m)).Disabled(s.busy).AutoFocus().Submitted() || entered
		})
	})
	if m.provider == twoFactorEmail {
		ui.Row(c).Gap(spaceRow).AlignItems(ui.Center).Children(func() {
			if secondaryButton(c, locales.LoginSendEmailCode(), s.busy || s.emailSent).Clicked() {
				s.sendEmailCode()
				c.Invalidate()
			}
			if s.emailSent {
				ui.Text(c, locales.LoginEmailCodeSent()).FontSize(t.Rem(fontSecondary)).TextColor(t.Success)
			}
		})
		if m.email != "" {
			muted(c, locales.LoginEmailCodeSentTo(m.email))
		}
	}
	if unsupported := s.challenge.unsupported(); len(unsupported) > 0 {
		muted(c, locales.LoginUnsupportedAlso(methodTitles(unsupported), locales.LoginUnsupportedNote()))
	}
	noticeOf(c, s.notice)
	if s.submitTwoFactorOnEnter(entered) {
		c.Invalidate()
	}
	ui.Row(c).Gap(spaceRow).AlignItems(ui.Center).Children(func() {
		if primaryButton(c, locales.LoginSubmitCode(), strings.TrimSpace(s.code) == "" || s.busy).Clicked() {
			s.submit2FA()
			c.Invalidate()
		}
		if plainButton(c, locales.LoginCancel(), s.busy).Clicked() {
			s.cancel()
			c.Invalidate()
		}
		if s.busy {
			ui.Spinner(c).Label(locales.LoginSpinnerVerifying())
		}
	})
}

// unsupportedMethods is the challenge with no method biowarden can answer:
// the wording points at the account page's extraction script, and Cancel is
// the way out. This is the decision the 2FA browser-path research recorded,
// not an oversight.
func (s *loginWindow) unsupportedMethods(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Padding(spaceMedium).Gap(spaceSmall).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, locales.LoginUnsupportedTitle()).Bold()
		muted(c, locales.LoginUnsupportedOffered(methodTitles(s.challenge.methods)))
	})
	noticeOf(c, s.notice)
	ui.Row(c).Gap(spaceRow).AlignItems(ui.Center).Children(func() {
		if primaryButton(c, locales.LoginCancel(), s.busy).Clicked() {
			s.cancel()
			c.Invalidate()
		}
		if s.busy {
			ui.Spinner(c).Label(locales.LoginSpinnerCancelling())
		}
	})
}

// methodTitle names a method the way the prompt shows it.
func methodTitle(m twoFactorMethod) string {
	switch m.provider {
	case twoFactorAuthenticator:
		return locales.LoginMethodAuthenticator()
	case twoFactorEmail:
		return locales.LoginMethodEmail()
	case twoFactorYubiKey:
		return locales.LoginMethodYubiKey()
	case twoFactorDuo, twoFactorOrgDuo:
		return locales.LoginMethodDuo()
	case twoFactorWebAuthn:
		return locales.LoginMethodWebAuthn()
	case twoFactorRecoveryCode:
		return locales.LoginMethodRecoveryCode()
	default:
		return m.name
	}
}

// methodTitles joins method names for the one-line hints, with the list
// separator the language uses.
func methodTitles(methods []twoFactorMethod) string {
	names := make([]string, 0, len(methods))
	for _, m := range methods {
		names = append(names, methodTitle(m))
	}
	return strings.Join(names, locales.LoginMethodListSeparator())
}

// methodFieldLabel is what the code field asks for, per method.
func methodFieldLabel(m twoFactorMethod) string {
	switch m.provider {
	case twoFactorEmail:
		return locales.LoginCodeEmail()
	case twoFactorYubiKey:
		return locales.LoginCodeYubiKey()
	default:
		return locales.LoginCode()
	}
}

// methodHint is the field's placeholder, per method.
func methodHint(m twoFactorMethod) string {
	switch m.provider {
	case twoFactorEmail:
		return locales.LoginCodeHintEmail()
	case twoFactorYubiKey:
		return locales.LoginCodeHintYubiKey()
	default:
		return locales.LoginCodeHint()
	}
}
