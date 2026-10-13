package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// keysPage mirrors the reference KeyTab: take the two values out of the
// extension, then store them behind Windows Hello. The extraction script is
// invented, and no value is written anywhere.
func (app *protoState) keysPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16).Gap(12).Children(func() {
				ui.Text(c, "密钥 (Keys)").FontSize(18).Bold()
				muted(c, "在已解锁的浏览器扩展里取出 User ID 与加密密钥（keyB64），保存在本机、由 Windows Hello 放行读取。")
				notice(c, app.keysNotice, app.keysNoticeOK)

				groupBox(c, "第一步：在已解锁的浏览器扩展里取出两个值", func() {
					muted(c, "1. 浏览器扩展需处于已解锁状态。")
					muted(c, "2. Chrome / Edge：chrome://extensions → 开发者模式 → Bitwarden → 「服务工作进程」。")
					muted(c, "3. Firefox：about:debugging#/runtime/this-firefox → Bitwarden → 「检查」。")
					muted(c, "4. 复制下面的脚本，粘贴到那个控制台回车，再复制输出的 JSON。")
					ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
						if ui.Button(c, "复制提取脚本").Clicked() {
							mygo.Clipboard.WriteText(extractSnippet)
							app.copiedAt = time.Now()
						}
						if !app.copiedAt.IsZero() {
							ui.Text(c, "已复制到剪贴板（"+itoa(len(extractSnippet))+" 字符），去控制台粘贴运行").
								FontSize(12).TextColor(t.Success)
						}
					})
					// The reference hides the snippet because a long script wrecks
					// its fixed-size SwiftUI layout; the page scrolls here, so the
					// script can stay auditable behind a collapsed disclosure.
					ui.Accordion(c, func() {
						ui.AccordionItem(c, "查看提取脚本（可审计）", &app.scriptOpen, func() {
							ui.TextArea(c, &app.scriptBody).ReadOnly(true).Lines(6, 14).
								FontSize(11).Placeholder("（脚本预览）")
						})
					})
				})

				groupBox(c, "第二步：保存到本机（Windows Hello 保护）", func() {
					ui.Form(c, func() {
						ui.Field(c, "User ID", func() {
							ui.TextInput(c.Key("userId"), &app.userId).
								Placeholder("控制台输出的 userId")
						})
						ui.Field(c, "加密密钥 (base64)", func() {
							ui.TextInput(c.Key("keyB64"), &app.keyB64).Password().
								Placeholder("keyB64")
						})
					})
					ready := strings.TrimSpace(app.userId) != "" && strings.TrimSpace(app.keyB64) != ""
					ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
						if ui.PrimaryButton(c, "保存并验证").Disabled(!ready || app.verifying).Clicked() {
							app.startVerify()
						}
						if app.verifying {
							ui.Spinner(c).Label("验证中")
							ui.Text(c, "正在调用 Windows Hello 验证身份...（原型：模拟 0.9 秒）").
								FontSize(12).TextColor(t.TextMuted)
						}
					})
					muted(c, "输出含 User ID 和加密密钥，只保存在本机凭据管理器，请勿外传。")
					muted(c, "保存成功后，还要在 Bitwarden 扩展 → 设置 → 安全中开启「解锁时使用生物识别」。")
				})

				groupBox(c, "已保存的账户", func() {
					if len(app.accounts) == 0 {
						muted(c, "暂无。填写上面的表单后点「保存并验证」。")
						return
					}
					for _, acc := range app.accounts {
						acc := acc
						ui.Row(c).Key(acc.userID).Gap(12).Padding(8, 6).
							AlignItems(ui.Center).Children(func() {
							ui.Column(c).Grow(1).Gap(2).Children(func() {
								ui.Text(c, acc.userID).FontSize(12).SingleLine().Tooltip(acc.userID)
								muted(c, acc.email+" · 创建于 "+acc.created)
							})
							if ui.Button(c, "测试解锁").Clicked() {
								app.keysNotice = "验证通过：Windows Hello 已确认身份（原型，userId=" +
									shortID(acc.userID) + "）"
								app.keysNoticeOK = true
							}
							// The row collection changes, so delete waits until
							// the view has finished building it.
							ui.Button(c, "删除").OnClick(func() { app.removeAccount(acc) })
						})
					}
				})
			})
		})
	})
}
