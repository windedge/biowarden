package ui

import (
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// logPage tails the host's real log file. "清空" only clears the view, and
// "打开日志文件" hands the path to the shell's default program.
func (app *appState) logPage(c *ui.Context) {
	ui.Column(c).Grow(1).Padding(16).Gap(8).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "日志 (Log)").FontSize(18).Bold()
			ui.Spacer(c)
			muted(c, "级别 "+app.settings.logLevel+" · "+app.settings.logPath)
		})
		notice(c, app.log.notice, app.log.noticeOK)
		ui.TextArea(c, &app.log.text).ReadOnly(true).Grow(1).FontSize(11).
			Label("日志内容").Placeholder("暂无日志")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "刷新").Clicked() {
				app.log.refresh()
			}
			if ui.Button(c, "清空").Clicked() {
				app.log.text = ""
				app.log.notice = "已清空当前视图（文件未改动）"
				app.log.noticeOK = true
			}
			if ui.Button(c, "打开日志文件").Clicked() {
				app.openLogFile()
			}
			ui.Spacer(c)
			muted(c, itoa(len(app.log.text))+" 字符")
		})
	})
}

// openLogFile opens the log with the default program through mygo.Shell,
// which marshals onto the main loop. A missing file is reported instead of
// letting the shell pop its own error dialog.
func (app *appState) openLogFile() {
	path := app.settings.logPath
	if _, err := os.Stat(path); err != nil {
		app.log.notice = "日志文件还不存在：" + path
		app.log.noticeOK = false
		return
	}
	if err := mygo.Shell.OpenPath(path); err != nil {
		app.log.notice = "打开日志文件失败：" + err.Error()
		app.log.noticeOK = false
		return
	}
	app.log.notice = "已用系统默认程序打开日志文件"
	app.log.noticeOK = true
}
