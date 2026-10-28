package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// browsersPage mirrors the reference BrowserTab: one row per browser with its
// manifest state, the absolute path we would write, and the actions that
// change that state. Every action goes through the registry seam, then the
// page reloads so it shows what actually happened.
func (app *appState) browsersPage(c *ui.Context) {
	bs := &app.browsers
	t := c.Theme()
	configured := 0
	hijacked := 0
	for _, b := range app.browsers.rows {
		if b.state != browserMissing {
			configured++
		}
		if b.state == browserHijacked {
			hijacked++
		}
	}
	ui.Column(c).Grow(1).Padding(pagePadding).Gap(spaceRow).Children(func() {
		ui.Row(c).Gap(spaceMedium).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.BrowsersTitle()).FontSize(t.Rem(fontHeadline)).Bold()
			ui.Spacer(c)
			if ui.PrimaryButton(c, locales.BrowsersInstallAll()).Disabled(len(app.browsers.rows) == 0).Clicked() {
				bs.installAll()
			}
		})
		muted(c, locales.BrowsersSummary(configured, len(app.browsers.rows), len(app.account.accounts)))
		switch {
		case len(bs.bulk) > 0:
			notice(c, locales.BrowsersBulkConfirm(len(bs.bulk)), false)
		case bs.confirmCover >= 0:
			notice(c, locales.BrowsersConfirmCover(), false)
		case hijacked > 0:
			notice(c, locales.BrowsersHijacked(hijacked), false)
		default:
			noticeOf(c, bs.notice)
		}
		ui.Divider(c)
		ui.List(c, nil, len(app.browsers.rows), func(i int) {
			app.browserRow(c, i)
		}).Grow(1)
		ui.Divider(c)
		muted(c, locales.BrowsersManifestHint())
	})
}

func (app *appState) browserRow(c *ui.Context, i int) {
	t := c.Theme()
	bs := &app.browsers
	b := &app.browsers.rows[i]
	ui.Row(c).Key(b.name).Gap(spaceMedium).Padding(spaceRow, spaceSmall).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(spaceHair).Children(func() {
			ui.Row(c).Gap(spaceSnug).AlignItems(ui.Center).Children(func() {
				ui.Text(c, b.name).Bold()
				if !b.installed {
					ui.Text(c, locales.BrowserNotInstalled()).FontSize(t.Rem(fontCaption)).TextColor(t.TextMuted).Tooltip(browserAbsentHint())
				}
			})
			ui.Row(c).Gap(spaceSmall).AlignItems(ui.Center).Children(func() {
				ui.Text(c, browserStatusLabel(b.state)).FontSize(t.Rem(fontCaption)).TextColor(browserStatusColor(t, b.state))
				ui.Text(c, connectionLabel(time.Now(), b.lastConnectedAt)).FontSize(t.Rem(fontCaption)).
					TextColor(extensionColor(t, b.lastConnectedAt))
			})
			ui.Text(c, b.path).FontSize(t.Rem(fontCaption)).TextColor(t.TextMuted).SingleLine().Tooltip(b.path)
		})
		// Actions run after the row is built: install/uninstall reload the rows,
		// which must not happen while this one is still being constructed.
		switch {
		case bs.confirmCover == i:
			// The overwrite needs a second click: the reference only writes
			// over a foreign manifest after a human confirms.
			ui.Button(c, locales.LoginCancel()).OnClick(func() { bs.confirmCover = -1 })
			confirm := locales.BrowsersActionConfirmOverwrite()
			if b.state == browserHijacked {
				confirm = locales.BrowsersActionConfirmRestore()
			}
			ui.PrimaryButton(c, confirm).OnClick(func() {
				bs.confirmCover = -1
				bs.install(b)
			}).Disabled(!b.installed).Tooltip(browserAbsentHint())
		case b.state == browserOurs:
			ui.Button(c, locales.BrowsersActionRepair()).OnClick(func() { bs.install(b) }).Disabled(!b.installed).Tooltip(browserAbsentHint())
			ui.Button(c, locales.BrowsersActionUninstall()).OnClick(func() { bs.uninstall(b) })
		case b.state == browserForeign:
			ui.Button(c, locales.BrowsersActionOverwrite()).OnClick(func() {
				bs.confirmCover = i
				bs.notice = pageNotice{}
			}).Disabled(!b.installed).Tooltip(browserAbsentHint())
			if bs.confirmUninstall == i {
				// Deleting a manifest that may be the official client's deserves
				// the same second click as overwriting one.
				ui.Button(c, locales.LoginCancel()).OnClick(func() { bs.confirmUninstall = -1 })
				ui.PrimaryButton(c, locales.BrowsersActionConfirmUninstall()).OnClick(func() {
					bs.confirmUninstall = -1
					bs.uninstall(b)
				})
				break
			}
			ui.Button(c, locales.BrowsersActionUninstall()).OnClick(func() {
				bs.confirmUninstall = i
				bs.notice = pageNotice{}
			})
		case b.state == browserHijacked:
			// Recovery is an overwrite of a foreign value, so it gets the same
			// two-step confirmation as the foreign row.
			ui.PrimaryButton(c, locales.BrowsersActionRestore()).OnClick(func() {
				bs.confirmCover = i
				bs.notice = pageNotice{}
			}).Disabled(!b.installed).Tooltip(browserAbsentHint())
			if bs.confirmUninstall == i {
				ui.Button(c, locales.LoginCancel()).OnClick(func() { bs.confirmUninstall = -1 })
				ui.PrimaryButton(c, locales.BrowsersActionConfirmUninstall()).OnClick(func() {
					bs.confirmUninstall = -1
					bs.uninstall(b)
				})
				break
			}
			ui.Button(c, locales.BrowsersActionUninstall()).OnClick(func() {
				bs.confirmUninstall = i
				bs.notice = pageNotice{}
			})
		default:
			ui.Button(c, locales.BrowsersActionInstall()).OnClick(func() { bs.install(b) }).Disabled(!b.installed).Tooltip(browserAbsentHint())
		}
	})
}

// browserAbsentHint explains a disabled write action: a browser this machine
// does not have has no registration point to write.
func browserAbsentHint() string {
	return locales.BrowserAbsentHint()
}

func browserStatusLabel(state int) string {
	switch state {
	case browserOurs:
		return locales.BrowserStatusOurs()
	case browserHijacked:
		return locales.BrowserStatusHijacked()
	case browserForeign:
		return locales.BrowserStatusForeign()
	default:
		return locales.BrowserStatusMissing()
	}
}

func browserStatusColor(t *ui.Theme, state int) ui.Color {
	switch state {
	case browserOurs:
		return t.Success
	case browserHijacked:
		return t.Danger
	case browserForeign:
		return t.Warning
	default:
		return t.TextMuted
	}
}

// connectionLabel is the per-row "has the extension ever connected" indicator.
// A zero time means no host process has recorded this browser yet, which is
// the placeholder the page has always shown.
func connectionLabel(now, at time.Time) string {
	if at.IsZero() {
		return locales.BrowserConnectionNever()
	}
	return locales.BrowserConnectionConnected(relativeTime(now, at))
}

// relativeTime renders a past instant as a short phrase. It is pure so the
// thresholds can be tested without a clock, and it clamps a future stamp (a
// clock that jumped) to "just now" rather than printing a negative age.
func relativeTime(now, at time.Time) string {
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return locales.BrowserRelativeJustNow()
	case d < time.Hour:
		return locales.BrowserRelativeMinutes(int(d.Minutes()))
	case d < 24*time.Hour:
		return locales.BrowserRelativeHours(int(d.Hours()))
	case d < 30*24*time.Hour:
		return locales.BrowserRelativeDays(int(d.Hours() / 24))
	default:
		return at.Format("2006-01-02")
	}
}

func extensionColor(t *ui.Theme, at time.Time) ui.Color {
	if !at.IsZero() {
		return t.Success
	}
	return t.TextMuted
}
