package ui

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The bulk-overwrite confirmation is a modal child of the main window, like
// the login window: narrow enough to read the paths, tall enough for the four
// supported browsers, and scrollable when a machine has more rows than fit.
const (
	browsersConfirmWidth  = 520
	browsersConfirmHeight = 480
)

// showBrowsersConfirmWindow creates the confirmation dialog, or brings the one
// already up to the front. It runs on the main thread, in the click on
// "一键全部安装".
func (app *appState) showBrowsersConfirmWindow() {
	if app.browsersWin != nil {
		app.browsersWin.Focus()
		return
	}
	if app.mainWindow == nil {
		// Without a parent the dialog would not be modal; the tray app always
		// builds one first, so this only guards an early or failed start.
		log.Println("biowarden: 覆盖确认窗缺少主窗口，未创建")
		return
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "确认覆盖浏览器注册",
		Width:           browsersConfirmWidth,
		Height:          browsersConfirmHeight,
		MinWidth:        browsersConfirmWidth,
		MinHeight:       browsersConfirmHeight,
		DisableMaximize: true,
		Parent:          app.mainWindow,
		Modal:           true,
		Content:         ui.View(app.browsers.confirmView),
	})
	app.browsersWin = win
	win.OnClose(func(e *mygo.CloseEvent) {
		// Closing the dialog means Cancel: nothing is written. The window
		// closes on its own, and OnClosed drops the handle.
		app.browsers.applyBulkCancel()
	})
	win.OnClosed(func() {
		app.browsersWin = nil
		// The page behind was disabled while the modal was up, so it has to
		// redraw with whatever the decision landed.
		if app.mainWindow != nil {
			app.mainWindow.Invalidate()
		}
	})
}

// hideBrowsersConfirmWindow destroys the dialog once the human decided.
// Destroy skips the OnClose listener, which would read it as a second cancel.
func (app *appState) hideBrowsersConfirmWindow() {
	if app.browsersWin != nil {
		app.browsersWin.Destroy()
	}
}

// confirmView is the dialog's page: the intro, one card per registration the
// bulk install would take over, and the two decisions. The column sits in a
// Scroll so a machine with several rows never clips the buttons.
func (s *browsersState) confirmView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16).Gap(12).Children(func() {
				ui.Text(c, "确认覆盖浏览器注册").FontSize(16).Bold()
				muted(c, s.confirmIntro())
				for _, row := range s.bulk {
					s.confirmRow(c, row)
				}
				muted(c, "覆盖只写本机注册表与 manifest 文件，不涉及密钥，不需要 Windows Hello 确认。")
				if s.hasForeign() {
					muted(c, "官方 Bitwarden 桌面端每次启动可能再次重写注册表；不再使用它时建议关闭或卸载。")
				}
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					if ui.PrimaryButton(c, s.confirmLabel()).Clicked() {
						s.confirmBulk()
						c.Invalidate()
					}
					if ui.Button(c, "取消").Clicked() {
						s.cancelBulk()
						c.Invalidate()
					}
				})
			})
		})
	})
}

// confirmIntro explains what the decision does, with the hijacked-only case
// worded as a recovery rather than a takeover of someone else's registration.
func (s *browsersState) confirmIntro() string {
	if s.hasForeign() {
		return "以下注册当前不属于 biowarden，可能来自官方 Bitwarden 桌面端或其他工具。" +
			"确认后改由 biowarden 接管，官方桌面端的生物解锁将失效；取消则不做任何改动。"
	}
	return "以下浏览器里 biowarden 的配置已被其他程序覆盖。确认后恢复为 biowarden 的 manifest；取消则不做任何改动。"
}

// hasForeign reports whether the dialog lists a plain foreign registration,
// which is the case that needs the official-client warning.
func (s *browsersState) hasForeign() bool {
	for _, row := range s.bulk {
		if row.state == browserForeign {
			return true
		}
	}
	return false
}

// confirmLabel names the primary action after what it actually does: a
// hijacked-only list is a recovery, anything else is a takeover.
func (s *browsersState) confirmLabel() string {
	if s.hasForeign() {
		return "确认覆盖"
	}
	return "确认恢复"
}

// confirmRow is one registration card: whose it is now, what it points at, and
// the biowarden manifest that would replace it.
func (s *browsersState) confirmRow(c *ui.Context, row browserRow) {
	t := c.Theme()
	ui.Column(c).Padding(10, 8).Gap(4).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, row.name+" · "+takeoverActionLabel(row.state)).Bold()
		current := "当前指向：" + row.path + takeoverOwnerHint(row.state)
		ui.Text(c, current).FontSize(11).TextColor(t.TextMuted).SingleLine().Tooltip(current)
		target := "覆盖后指向：" + row.installPath
		if row.state == browserHijacked {
			target = "恢复为：" + row.installPath
		}
		ui.Text(c, target).FontSize(11).TextColor(t.TextMuted).SingleLine().Tooltip(target)
	})
}

// takeoverActionLabel is the per-row headline: a foreign value is someone
// else's registration being taken over, a hijacked one is ours being restored.
func takeoverActionLabel(state int) string {
	if state == browserHijacked {
		return "恢复 biowarden 的配置"
	}
	return "接管此前属于其他程序的注册"
}

// takeoverOwnerHint says who the current value likely belongs to. A hijacked
// row is our own config that another program overwrote, which is a different
// worry from a value we never wrote.
func takeoverOwnerHint(state int) string {
	if state == browserHijacked {
		return "（biowarden 的配置已被它覆盖）"
	}
	return "（可能是官方 Bitwarden 桌面端）"
}
