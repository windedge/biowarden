package ui

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The login window's opening size. It is a modal dialog: narrow, not resizable
// taller than its content needs, and a child of the main window so it stays
// above it and blocks it until the attempt settles.
const (
	loginWindowWidth  = 420
	loginWindowHeight = 440
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
		MinHeight:       320,
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
