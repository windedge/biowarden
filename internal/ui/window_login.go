package ui

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The login window's opening size. It is a modal dialog: narrow, and a child of
// the main window so it stays above it and blocks it until the attempt settles.
//
// The height is the credentials form's own content (16 padding + title rows +
// two fields + the collapsed Advanced row + the button row + 16 padding, about
// 270 DIPs) plus the roughly 40 DIPs of frame and caption, rounded up so a font
// that wraps the hint differently still fits: the form fills the page, with no
// blank block under the buttons. Opening Advanced or landing on the two-factor
// prompt makes the form taller, and the scroll view around it takes over rather
// than clipping the form or leaving the window oversized for the common case.
const (
	loginWindowWidth  = 420
	loginWindowHeight = 320
)

// showLoginWindow creates the login window, or brings the one already up to
// the front. It runs on the main thread, in the click that asked for it.
func (app *appState) showLoginWindow() {
	if app.loginWin != nil {
		app.loginWin.Focus()
		return
	}
	if app.mainWindow == nil {
		// A login without a parent would not be modal; the tray app always
		// builds one first, so this only guards an early or failed start.
		log.Println("biowarden: 登录窗缺少主窗口，未创建")
		return
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "登录 Bitwarden 账号",
		Width:           loginWindowWidth,
		Height:          loginWindowHeight,
		MinWidth:        loginWindowWidth,
		MinHeight:       loginWindowHeight,
		DisableMaximize: true,
		Parent:          app.mainWindow,
		Modal:           true,
		Content:         ui.View(app.login.loginView),
	})
	app.loginWin = win
	win.OnClose(func(e *mygo.CloseEvent) {
		// Closing the window means Cancel, and the state machine owns the
		// actual close: it destroys the window once the backend confirmed
		// the attempt was abandoned.
		e.PreventDefault()
		app.login.requestClose()
	})
	win.OnClosed(func() {
		app.loginWin = nil
		app.login.showing = false
		// The account page behind the modal was disabled until now, so it has
		// to redraw with whatever the attempt landed.
		if app.mainWindow != nil {
			app.mainWindow.Invalidate()
		}
	})
}

// hideLoginWindow destroys the login window once the attempt settled. Destroy
// skips the OnClose listener, which would read the closing as a second cancel.
func (app *appState) hideLoginWindow() {
	if app.loginWin != nil {
		app.loginWin.Destroy()
	}
}
