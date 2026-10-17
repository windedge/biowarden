package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// browsersPage mirrors the reference BrowserTab: one row per browser with its
// manifest state, the absolute path we would write, and the actions that
// change that state. Every action goes through the registry seam, then the
// page reloads so it shows what actually happened.
func (app *appState) browsersPage(c *ui.Context) {
	bs := &app.browsers
	configured := 0
	hijacked := 0
	for _, b := range app.browsers.rows {
		if b.state != browserMissing {
			configured++
		}
		if b.state == browserHijacked {
			hijacked++
		}
	}
	ui.Column(c).Grow(1).Padding(16).Gap(10).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "浏览器 (Browsers)").FontSize(18).Bold()
			ui.Spacer(c)
			if ui.PrimaryButton(c, "一键全部安装").Disabled(len(app.browsers.rows) == 0).Clicked() {
				bs.installAll()
			}
		})
		muted(c, "已配置 "+itoa(configured)+" / "+itoa(len(app.browsers.rows))+" 个浏览器 · 已存 "+
			itoa(len(app.account.accounts))+" 个密钥。没有 manifest 的浏览器无法拉起生物识别解锁。"+
			"「一键全部安装」直接写入未安装和属于我们的行；可能属于官方客户端或被覆盖的注册会先列出确认，确认后一并覆盖。")
		switch {
		case len(bs.bulk) > 0:
			notice(c, "有 "+itoa(len(bs.bulk))+" 个注册需要确认，请在确认对话框中处理。", false)
		case bs.confirmCover >= 0:
			notice(c, "覆盖前确认：该 manifest 可能属于 Bitwarden 官方桌面客户端，覆盖后官方生物解锁会失效。", false)
		case hijacked > 0:
			notice(c, "检测到 "+itoa(hijacked)+" 个浏览器的配置被其他程序覆盖（官方桌面端每次启动都会重写注册表）。恢复后建议关闭或卸载官方桌面客户端，否则会再次被覆盖。", false)
		default:
			notice(c, bs.notice, bs.noticeOK)
		}
		ui.Divider(c)
		ui.List(c, nil, len(app.browsers.rows), func(i int) {
			app.browserRow(c, i)
		}).Grow(1)
		ui.Divider(c)
		muted(c, "manifest 记录本工具 exe 的绝对路径；移动程序位置后到对应行点「修复」重写。")
	})
}

func (app *appState) browserRow(c *ui.Context, i int) {
	t := c.Theme()
	bs := &app.browsers
	b := &app.browsers.rows[i]
	ui.Row(c).Key(b.name).Gap(12).Padding(10, 8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, b.name).Bold()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, browserStatusLabel(b.state)).FontSize(11).TextColor(browserStatusColor(t, b.state))
				ui.Text(c, connectionLabel(time.Now(), b.lastConnectedAt)).FontSize(11).
					TextColor(extensionColor(t, b.lastConnectedAt))
			})
			ui.Text(c, b.path).FontSize(11).TextColor(t.TextMuted).SingleLine().Tooltip(b.path)
		})
		// Actions run after the row is built: install/uninstall reload the rows,
		// which must not happen while this one is still being constructed.
		switch {
		case bs.confirmCover == i:
			// The overwrite needs a second click: the reference only writes
			// over a foreign manifest after a human confirms.
			ui.Button(c, "取消").OnClick(func() { bs.confirmCover = -1 })
			confirm := "确认覆盖"
			if b.state == browserHijacked {
				confirm = "确认恢复"
			}
			ui.PrimaryButton(c, confirm).OnClick(func() {
				bs.confirmCover = -1
				bs.install(b)
			})
		case b.state == browserOurs:
			ui.Button(c, "修复").OnClick(func() { bs.install(b) })
			ui.Button(c, "卸载").OnClick(func() { bs.uninstall(b) })
		case b.state == browserForeign:
			ui.Button(c, "覆盖安装").OnClick(func() {
				bs.confirmCover = i
				bs.notice = ""
			})
			if bs.confirmUninstall == i {
				// Deleting a manifest that may be the official client's deserves
				// the same second click as overwriting one.
				ui.Button(c, "取消").OnClick(func() { bs.confirmUninstall = -1 })
				ui.PrimaryButton(c, "确认卸载").OnClick(func() {
					bs.confirmUninstall = -1
					bs.uninstall(b)
				})
				break
			}
			ui.Button(c, "卸载").OnClick(func() {
				bs.confirmUninstall = i
				bs.notice = ""
			})
		case b.state == browserHijacked:
			// Recovery is an overwrite of a foreign value, so it gets the same
			// two-step confirmation as the foreign row.
			ui.PrimaryButton(c, "恢复安装").OnClick(func() {
				bs.confirmCover = i
				bs.notice = ""
			})
			if bs.confirmUninstall == i {
				ui.Button(c, "取消").OnClick(func() { bs.confirmUninstall = -1 })
				ui.PrimaryButton(c, "确认卸载").OnClick(func() {
					bs.confirmUninstall = -1
					bs.uninstall(b)
				})
				break
			}
			ui.Button(c, "卸载").OnClick(func() {
				bs.confirmUninstall = i
				bs.notice = ""
			})
		default:
			ui.Button(c, "安装").OnClick(func() { bs.install(b) })
		}
	})
}

func browserStatusLabel(state int) string {
	switch state {
	case browserOurs:
		return "已安装（biowarden）"
	case browserHijacked:
		return "已被其他程序覆盖（可能是官方 Bitwarden 桌面客户端）"
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
	case browserHijacked:
		return t.Danger
	case browserForeign:
		return t.Warning
	default:
		return t.TextMuted
	}
}

// connectionLabel is the per-row "has the extension ever connected" indicator.
// A zero time means no host process has recorded this browser yet, which is
// the placeholder the page has always shown.
func connectionLabel(now, at time.Time) string {
	if at.IsZero() {
		return "扩展未连接"
	}
	return "已连接过 · " + relativeTime(now, at)
}

// relativeTime renders a past instant as a short phrase. It is pure so the
// thresholds can be tested without a clock, and it clamps a future stamp (a
// clock that jumped) to "刚刚" rather than printing a negative age.
func relativeTime(now, at time.Time) string {
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + " 分钟前"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + " 小时前"
	case d < 30*24*time.Hour:
		return itoa(int(d.Hours()/24)) + " 天前"
	default:
		return at.Format("2006-01-02")
	}
}

func extensionColor(t *ui.Theme, at time.Time) ui.Color {
	if !at.IsZero() {
		return t.Success
	}
	return t.TextMuted
}
