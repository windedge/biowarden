package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// logPage shows fake host log text, including redacted long strings, and the
// three actions the reference offers. It never reads or writes a real file.
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
				app.log.text += "\n[" + time.Now().Format("2006-01-02 15:04:05.000") +
					"] [INFO] log refreshed (prototype)"
				app.log.notice = "已刷新（原型：追加一条样例，不读取真实文件）"
				app.log.noticeOK = true
			}
			if ui.Button(c, "清空").Clicked() {
				app.log.text = ""
				app.log.notice = "已清空当前视图（原型：不影响任何文件）"
				app.log.noticeOK = true
			}
			if ui.Button(c, "打开日志文件").Clicked() {
				// Deliberately not Shell.OpenPath: the path is fake, and a real
				// call would pop a Windows error dialog at the human.
				app.log.notice = "原型：这里会用系统默认程序打开 " + app.settings.logPath + "（未接真实文件）"
				app.log.noticeOK = false
			}
			ui.Spacer(c)
			muted(c, itoa(len(app.log.text))+" 字符")
		})
	})
}
