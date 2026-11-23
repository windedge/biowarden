package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/windedge/biowarden/locales"
)

// The log controls are capped: that page's fields stretch to the window
// width, so on a wide window a short value sits in a read-only box that reads
// as an editor. The log level dropdown is narrower because its options are a
// word each.
//
// The cap is a maximum, not a fixed width: a form's label column grows with
// the longest label, so in a verbose language the room a control gets can be
// less than the cap, and a fixed width would then run past the content area,
// where the window cuts the control's border off.
const (
	settingsFieldWidth  = 400
	settingsSelectWidth = 260
)

// settingsPage mirrors the reference SettingsTab, extended with the log
// settings the Windows tool needs: autostart, Windows Hello availability,
// log path and level, version. Autostart and Hello are real; see below.
func (app *appState) settingsPage(c *ui.Context) {
	t := c.Theme()
	// The padding wraps the scroll viewport, not the scrolled content: a
	// text the viewport cuts mid-line must end at the content area's edge,
	// not at the window's, or it reads as cut in half.
	ui.Column(c).Grow(1).Padding(pagePadding).Gap(spaceTiny).Children(func() {
		ui.Text(c, locales.SettingsTitle()).FontSize(t.Rem(fontHeadline)).Bold()
		muted(c, locales.SettingsIntro())
		ui.Scroll(c).Grow(1).Children(func() {
			// Right padding keeps the rows out of the overlay scrollbar's track.
			ui.Column(c).Padding(0, scrollbarGutter, 0, 0).Gap(spaceMedium).Children(func() {
				notice(c, app.settings.notice.text(), app.settings.notice.ok)
				ui.Form(c, func() {
					ui.Fieldset(c, locales.SettingsGroupEntry(), func() {
						// The language row is the first text of the page the catalogs
						// say; the rest of it moves there in a later ticket.
						ui.Field(c, locales.Language(), func() {
							// The row works in tags, the labels are display only, so a
							// label changing with the language can never be written back
							// as a tag.
							if tag, changed := app.settings.languageSelect(c); changed {
								app.settings.setLanguage(tag)
							}
						})
						ui.Field(c, locales.SettingsAppearance(), func() {
							// The row works in an enum, so its labels follow the language
							// while the value stays one the app can store and apply.
							if a, changed := app.settings.appearanceSegmented(c); changed {
								app.settings.setAppearance(a)
								// This frame was painted in the old palette; ask for one
								// more, where applyPalette reads the appearance the
								// choice just set.
								c.After(time.Millisecond)
							}
						})
						ui.Field(c, locales.SettingsAutostart(), func() {
							if ui.Switch(c, &app.settings.launchAtLogin).Changed() {
								app.settings.setAutostart()
							}
						})
						ui.Field(c, locales.SettingsShowTray(), func() {
							ui.Column(c).Gap(spaceHair).Children(func() {
								if ui.Switch(c, &app.settings.showTray).Changed() {
									// mygo has no per-tray show/hide and rebuilding it is not
									// worth it yet, so this switch is memory-only.
									if app.settings.showTray {
										app.settings.notice = pageNotice{kind: noticeSettingsTrayShown, ok: true}
									} else {
										app.settings.notice = pageNotice{kind: noticeSettingsTrayHidden, ok: true}
									}
								}
								muted(c, locales.SettingsShowTrayNote())
							})
						})
						ui.Field(c, locales.SettingsHello(), func() {
							ui.Row(c).Gap(spaceRow).AlignItems(ui.Center).Children(func() {
								switch {
								case app.settings.helloChecking:
									ui.Spinner(c).Label(locales.SettingsHelloCheckingSpinner())
									ui.Text(c, locales.SettingsHelloChecking()).FontSize(t.Rem(fontSecondary)).TextColor(t.TextMuted)
								case app.settings.helloOK:
									ui.Text(c, locales.SettingsHelloAvailable()).
										FontSize(t.Rem(fontSecondary)).TextColor(t.Success)
								default:
									ui.Text(c, locales.SettingsHelloUnavailable()).FontSize(t.Rem(fontSecondary)).TextColor(t.Danger)
								}
								secondaryButton(c, locales.SettingsHelloRecheck(), app.settings.helloChecking).
									OnClick(func() {
										app.settings.notice = pageNotice{}
										app.settings.startHelloCheck()
									})
							})
						})
					})
					ui.Fieldset(c, locales.SettingsGroupNetwork(), func() {
						ui.Field(c, locales.SettingsProxyMode(), func() {
							if mode, changed := app.settings.proxyModeSelect(c); changed {
								app.settings.setProxyMode(mode)
							}
						})
						if app.settings.proxyMode == proxyManual {
							ui.Field(c, locales.SettingsProxyAddress(), func() {
								ui.TextInput(c.Key("proxy-address"), &app.settings.proxyAddress).
									Placeholder(locales.SettingsProxyAddressPlaceholder()).
									Label(locales.SettingsProxyAddress()).MaxWidth(settingsSelectWidth)
							})
							// An empty label puts the checkbox in the form's content column.
							ui.Field(c, "", func() {
								ui.Checkbox(c, &app.settings.proxyUseCredentials, locales.SettingsProxyCredentials())
							})
							if app.settings.proxyUseCredentials {
								ui.Field(c, locales.SettingsProxyUser(), func() {
									ui.TextInput(c.Key("proxy-user"), &app.settings.proxyUser).
										Placeholder(locales.SettingsProxyUser()).
										Label(locales.SettingsProxyUser()).MaxWidth(settingsSelectWidth)
								})
								ui.Field(c, locales.SettingsProxyPassword(), func() {
									ui.Row(c).Gap(spaceRow).AlignItems(ui.Center).Children(func() {
										in := ui.TextInput(c.Key("proxy-password"), &app.settings.proxyPassword)
										if !app.settings.showProxyPassword {
											in = in.Password()
										}
										in.Label(locales.SettingsProxyPassword()).
											Placeholder(locales.SettingsProxyPassword()).MaxWidth(settingsSelectWidth)
										plainButton(c, "", false).Children(func() {
											ui.Icon(c, iconEye).FontSize(t.Rem(fontBody)).Shrink(0)
										}).Tooltip(locales.SettingsProxyTogglePassword()).OnClick(func() {
											app.settings.showProxyPassword = !app.settings.showProxyPassword
											c.Invalidate()
										})
									})
								})
							}
						}
						if app.settings.proxyMode != proxyOff {
							ui.Field(c, locales.SettingsProxyTestLabel(), func() {
								ui.Column(c).Gap(spaceHair).Children(func() {
									secondaryButton(c, locales.SettingsProxyTest(), app.settings.proxyTesting).
										MaxWidth(settingsSelectWidth).
										OnClick(func() {
											app.settings.startProxyTest()
											c.Invalidate()
										})
									ink := t.TextMuted
									switch app.settings.proxyTest {
									case proxyTestOK:
										ink = t.Success
									case proxyTestRefused, proxyTestUnreachable:
										ink = t.Danger
									}
									// A long result wraps instead of being elided to fit a row.
									ui.Text(c, app.settings.proxyTest.label(app.settings.proxyTestHost())).
										FontSize(t.Rem(fontCaption)).TextColor(ink)
								})
							})
						}
					})
					ui.Fieldset(c, locales.SettingsGroupLog(), func() {
						ui.Field(c, locales.SettingsLogLevel(), func() {
							// The host reads its level from an environment variable, so the
							// dropdown cannot reach it; it stays a memory value for now.
							ui.Column(c).Gap(spaceHair).Children(func() {
								app.settings.logLevelSelect(c)
								muted(c, locales.SettingsLogLevelNote())
							})
						})
						ui.Field(c, locales.SettingsLogPath(), func() {
							ui.TextInput(c, &app.settings.logPath).ReadOnly(true).FontSize(t.Rem(fontCaption)).
								MaxWidth(settingsFieldWidth).Label(locales.SettingsLogPathLabel())
						})
					})
					ui.Fieldset(c, locales.SettingsGroupAbout(), func() {
						ui.Field(c, locales.SettingsVersion(), func() {
							ui.Text(c, appName+" "+appVersion)
						})
						ui.Field(c, locales.SettingsAboutNote(), func() {
							// A text wraps at the width it gets, so it needs no Wrap
							// of its own: that is for containers, and it would measure
							// the note as one long line. The width is capped because
							// a form's label column grows with the longest label, which
							// in a verbose language leaves the note less room than the
							// window has, and the note then runs past the window.
							ui.Text(c, locales.SettingsAboutText()).
								FontSize(t.Rem(fontSecondary)).TextColor(t.TextMuted).MaxWidth(settingsFieldWidth)
						})
					})
				})
			})
		})
	})
}
