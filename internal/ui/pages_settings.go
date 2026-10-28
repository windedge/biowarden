package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/windedge/biowarden/locales"
)

// The settings controls are capped: the page's fields stretch to the window
// width, so on a wide window a short value sits in a read-only box that reads
// as an editor. The text fields match the login window's own field width for a
// consistent look, and the log level dropdown is narrower because its options
// are a word each.
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
	ui.Column(c).Grow(1).Children(func() {
		// The padding is CSS order: top, right, bottom and left.
		ui.Column(c).Padding(pagePadding, pagePadding, pagePadding, pagePadding).Gap(spaceTiny).Children(func() {
			ui.Text(c, locales.SettingsTitle()).FontSize(t.Rem(fontHeadline)).Bold()
			muted(c, locales.SettingsIntro())
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(pagePadding, pagePadding, pagePadding, pagePadding).Gap(spaceMedium).Children(func() {
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
						ui.Field(c, locales.SettingsAutostart(), func() {
							if ui.Switch(c, &app.settings.launchAtLogin).Changed() {
								app.settings.setAutostart()
							}
						})
						ui.Field(c, locales.SettingsShowTray(), func() {
							if ui.Switch(c, &app.settings.showTray).Changed() {
								// mygo has no per-tray show/hide and rebuilding it is not
								// worth it yet, so this switch is memory-only.
								if app.settings.showTray {
									app.settings.notice = pageNotice{kind: noticeSettingsTrayShown, ok: true}
								} else {
									app.settings.notice = pageNotice{kind: noticeSettingsTrayHidden, ok: true}
								}
							}
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
								ui.Button(c, locales.SettingsHelloRecheck()).Disabled(app.settings.helloChecking).
									OnClick(func() {
										app.settings.notice = pageNotice{}
										app.settings.startHelloCheck()
									})
							})
						})
					})
					ui.Fieldset(c, locales.SettingsGroupLog(), func() {
						ui.Field(c, locales.SettingsLogLevel(), func() {
							// The host reads its level from an environment variable, so the
							// dropdown cannot reach it; it stays a memory value for now.
							app.settings.logLevelSelect(c)
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
