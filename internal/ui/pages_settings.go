package ui

import (
	"github.com/egoist/mygo/ui"
)

// settingsPage mirrors the reference SettingsTab, extended with the log
// settings the Windows tool needs: autostart, Windows Hello availability,
// log path and level, version. Autostart and Hello are real; see below.
func (app *appState) settingsPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Children(func() {
		ui.Column(c).Padding(16, 16, 16, 8).Gap(4).Children(func() {
			ui.Text(c, "设置 (Settings)").FontSize(18).Bold()
			muted(c, "自启动与 Windows Hello 已接真实后端；日志级别与托盘图标仅本次运行内生效。")
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16, 4, 16, 16).Gap(12).Children(func() {
				notice(c, app.settings.notice, app.settings.noticeOK)
				ui.Form(c, func() {
					ui.Fieldset(c, "账号", func() {
						ui.Field(c, "服务器地址", func() {
							// The live session's address, the same value the login window
							// prefills from; read-only, never edited here.
							ui.TextInput(c, &app.account.session.serverURL).ReadOnly(true).FontSize(11)
						})
					})
					ui.Fieldset(c, "入口", func() {
						ui.Field(c, "登录时自动启动", func() {
							if ui.Switch(c, &app.settings.launchAtLogin).Changed() {
								app.settings.setAutostart()
							}
						})
						ui.Field(c, "显示托盘图标", func() {
							if ui.Switch(c, &app.settings.showTray).Changed() {
								// mygo has no per-tray show/hide and rebuilding it is not
								// worth it yet, so this switch is memory-only.
								if app.settings.showTray {
									app.settings.notice = "已显示托盘图标（仅内存：重启后恢复）"
								} else {
									app.settings.notice = "已隐藏托盘图标，退出只能从窗口（仅内存：托盘仍在）"
								}
								app.settings.noticeOK = true
							}
						})
						ui.Field(c, "Windows Hello", func() {
							ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
								switch {
								case app.settings.helloChecking:
									ui.Spinner(c).Label("检测 Windows Hello")
									ui.Text(c, "正在检测...").FontSize(12).TextColor(t.TextMuted)
								case app.settings.helloOK:
									ui.Text(c, "可用（已配置人脸 / 指纹 / PIN）").
										FontSize(12).TextColor(t.Success)
								default:
									ui.Text(c, "不可用").FontSize(12).TextColor(t.Danger)
								}
								ui.Button(c, "重新检测").Disabled(app.settings.helloChecking).
									OnClick(func() {
										app.settings.notice = ""
										app.settings.startHelloCheck()
									})
							})
						})
					})
					ui.Fieldset(c, "日志", func() {
						ui.Field(c, "级别", func() {
							// The host reads its level from an environment variable, so the
							// dropdown cannot reach it; it stays a memory value for now.
							ui.Select(c, &app.settings.logLevel, logLevels)
						})
						ui.Field(c, "路径", func() {
							ui.TextInput(c, &app.settings.logPath).ReadOnly(true).FontSize(11)
						})
					})
					ui.Fieldset(c, "关于", func() {
						ui.Field(c, "版本", func() {
							ui.Text(c, "biowarden "+appVersion)
						})
						ui.Field(c, "说明", func() {
							ui.Text(c, "Windows Hello 只是放行对凭据管理器里密钥的读取；密钥本身没有被生物特征加密。").
								FontSize(12).TextColor(t.TextMuted).Wrap()
						})
					})
				})
			})
		})
	})
}
