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
	ui.Column(c).Grow(1).Padding(16).Gap(8).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.LogTitle()).FontSize(18).Bold()
			ui.Spacer(c)
			muted(c, locales.LogLevelLine(app.settings.logLevel.label(), app.settings.logPath))
		})
		notice(c, app.log.notice.text(), app.log.notice.ok)
		ui.TextArea(c, &app.log.text).ReadOnly(true).Grow(1).FontSize(11).
			Label(locales.LogContent()).Placeholder(locales.LogPlaceholder())
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, locales.Refresh()).Clicked() {
				app.log.refresh()
			}
			if ui.Button(c, locales.LogClear()).Clicked() {
				app.log.text = ""
				app.log.notice = pageNotice{kind: noticeLogCleared, ok: true}
			}
			if ui.Button(c, locales.LogOpen()).Clicked() {
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
