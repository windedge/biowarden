package ui

import (
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// logPage tails the host's real log file. Clear only clears the view, and
// Open log file hands the path to the shell's default program.
func (app *appState) logPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Padding(pagePadding).Gap(spaceSmall).Children(func() {
		ui.Row(c).Gap(spaceRow).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.LogTitle()).FontSize(t.Rem(fontHeadline)).Bold()
			ui.Spacer(c)
			muted(c, locales.LogLevelLine(app.settings.logLevel.label(), app.settings.logPath))
		})
		noticeOf(c, app.log.notice)
		ui.TextArea(c, &app.log.text).ReadOnly(true).Grow(1).FontSize(t.Rem(fontCaption)).
			Label(locales.LogContent()).Placeholder(locales.LogPlaceholder())
		ui.Row(c).Gap(spaceSmall).AlignItems(ui.Center).Children(func() {
			if secondaryButton(c, locales.Refresh(), false).Clicked() {
				app.log.refresh()
			}
			if plainButton(c, locales.LogClear(), false).Clicked() {
				app.log.text = ""
				app.log.notice = pageNotice{kind: noticeLogCleared, ok: true}
			}
			if plainButton(c, locales.LogOpen(), false).Clicked() {
				app.openLogFile()
			}
			ui.Spacer(c)
			muted(c, locales.LogCharacters(len(app.log.text)))
		})
	})
}

// openLogFile opens the log with the default program through mygo.Shell,
// which marshals onto the main loop. A missing file is reported instead of
// letting the shell pop its own error dialog.
func (app *appState) openLogFile() {
	path := app.settings.logPath
	if _, err := os.Stat(path); err != nil {
		app.log.notice = pageNotice{kind: noticeLogMissing, args: []string{path}}
		return
	}
	if err := mygo.Shell.OpenPath(path); err != nil {
		app.log.notice = pageNotice{kind: noticeLogOpenFailed, err: err}
		return
	}
	app.log.notice = pageNotice{kind: noticeLogOpened, ok: true}
}
