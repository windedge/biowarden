package ui

import (
	"github.com/egoist/mygo/ui"
)

// settingsPage mirrors the reference SettingsTab, extended with the log
// settings the Windows tool needs: autostart, Windows Hello availability,
// log path and level, version. No switch writes anything.
func (app *appState) settingsPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Children(func() {
		ui.Column(c).Padding(16, 16, 16, 8).Gap(4).Children(func() {
			ui.Text(c, "设置 (Settings)").FontSize(18).Bold()
			muted(c, "原型状态：开关只在本次运行内生效，不写注册表、不写配置文件。")
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16, 4, 16, 16).Gap(12).Children(func() {
				notice(c, app.settings.notice, app.settings.noticeOK)
				ui.Form(c, func() {
					ui.Fieldset(c, "入口", func() {
						ui.Field(c, "登录时自动启动", func() {
							if ui.Switch(c, &app.settings.launchAtLogin).Changed() {
								if app.settings.launchAtLogin {
									app.settings.notice = "已开启登录时自动启动（原型：未写入注册表）"
								} else {
									app.settings.notice = "已关闭登录时自动启动（原型：未写入注册表）"
								}
								app.settings.noticeOK = true
							}
						})
						ui.Field(c, "显示托盘图标", func() {
							if ui.Switch(c, &app.settings.showTray).Changed() {
								if app.settings.showTray {
									app.settings.notice = "已显示托盘图标（原型：仅内存）"
								} else {
									app.settings.notice = "已隐藏托盘图标，退出只能从窗口（原型：仅内存）"
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
									OnClick(app.settings.startHelloCheck)
							})
						})
					})
					ui.Fieldset(c, "日志", func() {
						ui.Field(c, "级别", func() {
							ui.Select(c, &app.settings.logLevel, logLevels)
						})
						ui.Field(c, "路径", func() {
							ui.TextInput(c, &app.settings.logPath).ReadOnly(true).FontSize(11)
						})
					})
					ui.Fieldset(c, "关于", func() {
						ui.Field(c, "版本", func() {
							ui.Text(c, "biowarden 0.1.0（原型：假数据）")
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
