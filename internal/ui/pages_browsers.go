package ui

import (
	"github.com/egoist/mygo/ui"
)

// browsersPage mirrors the reference BrowserTab: one row per browser with its
// manifest state, the absolute path we would write, and the actions that
// change that state. Everything is in memory.
func (app *protoState) browsersPage(c *ui.Context) {
	configured := 0
	for _, b := range app.browsers {
		if b.state != browserMissing {
			configured++
		}
	}
	ui.Column(c).Grow(1).Padding(16).Gap(10).Children(func() {
		ui.Text(c, "浏览器 (Browsers)").FontSize(18).Bold()
		muted(c, "已配置 "+itoa(configured)+" / "+itoa(len(app.browsers))+" 个浏览器 · 已存 "+
			itoa(len(app.accounts))+" 个密钥。没有 manifest 的浏览器无法拉起生物识别解锁。")
		if app.confirmCover >= 0 {
			notice(c, "覆盖前确认：该 manifest 可能属于 Bitwarden 官方桌面客户端，覆盖后官方生物解锁会失效。", false)
		} else {
			notice(c, app.browserNotice, app.browserNoticeOK)
		}
		ui.Divider(c)
		ui.List(c, nil, len(app.browsers), func(i int) {
			app.browserRow(c, i)
		}).Grow(1)
		ui.Divider(c)
		muted(c, "manifest 记录本工具 exe 的绝对路径；移动程序位置后到对应行点「修复」重写。")
	})
}

func (app *protoState) browserRow(c *ui.Context, i int) {
	t := c.Theme()
	b := app.browsers[i]
	ui.Row(c).Key(b.name).Gap(12).Padding(10, 8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, b.name).Bold()
			ui.Text(c, browserStatusLabel(b.state)).FontSize(11).TextColor(browserStatusColor(t, b.state))
			ui.Text(c, b.path).FontSize(11).TextColor(t.TextMuted).SingleLine().Tooltip(b.path)
		})
		switch {
		case app.confirmCover == i:
			// The overwrite needs a second click: the reference only writes
			// over a foreign manifest after a human confirms.
			if ui.Button(c, "取消").Clicked() {
				app.confirmCover = -1
			}
			if ui.PrimaryButton(c, "确认覆盖").Clicked() {
				b.state = browserOurs
				app.confirmCover = -1
				app.browserNotice = "已覆盖 " + b.name + " 的 manifest（原型：仅内存）"
				app.browserNoticeOK = true
			}
		case b.state == browserOurs:
			if ui.Button(c, "修复").Clicked() {
				app.browserNotice = "已为 " + b.name + " 重写 manifest 路径（原型：仅内存）"
				app.browserNoticeOK = true
			}
			if ui.Button(c, "卸载").Clicked() {
				b.state = browserMissing
				app.browserNotice = "已删除 " + b.name + " 的 manifest（原型：仅内存）"
				app.browserNoticeOK = true
			}
		case b.state == browserForeign:
			if ui.Button(c, "覆盖安装").Clicked() {
				app.confirmCover = i
				app.browserNotice = ""
			}
			if app.confirmUninstall == i {
				// Deleting a manifest that may be the official client's deserves
				// the same second click as overwriting one.
				if ui.Button(c, "取消").Clicked() {
					app.confirmUninstall = -1
				}
				if ui.PrimaryButton(c, "确认卸载").Clicked() {
					b.state = browserMissing
					app.confirmUninstall = -1
					app.browserNotice = "已删除 " + b.name + " 的 manifest（原型：仅内存）"
					app.browserNoticeOK = true
				}
				break
			}
			if ui.Button(c, "卸载").Clicked() {
				app.confirmUninstall = i
				app.browserNotice = ""
			}
		default:
			if ui.Button(c, "安装").Clicked() {
				b.state = browserOurs
				app.browserNotice = "已为 " + b.name + " 写入 manifest（原型：仅内存）"
				app.browserNoticeOK = true
			}
		}
	})
}

func browserStatusLabel(state int) string {
	switch state {
	case browserOurs:
		return "已安装（biowarden）"
	case browserForeign:
		return "已存在（可能是官方 Bitwarden 桌面客户端的配置）"
	default:
		return "未安装"
	}
}

func browserStatusColor(t *ui.Theme, state int) ui.Color {
	switch state {
	case browserOurs:
		return t.Success
	case browserForeign:
		return t.Warning
	default:
		return t.TextMuted
	}
}
