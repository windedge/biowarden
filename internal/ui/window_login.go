package ui

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/windedge/biowarden/locales"
)

// The login window's opening size. It is a modal dialog: narrow, and a child of
// the main window so it stays above it and blocks it until the attempt settles.
//
// Every height here is an outer size, the convention WindowOptions and
// Window.SetSize take: the page area plus the frame and caption. The
// credentials form with Advanced closed measures about 270 DIPs of content, so
// loginWindowHeight carries the roughly 50 DIPs of frame on top of it; the
// other targets are that frame plus the measured content of their own state.
const (
	loginWindowWidth = 420
	// loginWindowHeight opens the credentials form with Advanced closed.
	loginWindowHeight = 320
	// loginWindowAdvancedHeight covers the expanded disclosure: the server
	// address field and its hint add about 80 DIPs of content.
	loginWindowAdvancedHeight = 400
	// loginWindowTwoFactorHeight is a single-method code prompt, the smallest
	// target, so MinHeight uses it too.
	loginWindowTwoFactorHeight = 265
	// The two-factor rows below are what a code prompt grows by over a single
	// method: a Segmented control with its label, the email send button and
	// address line, and the line naming the methods biowarden cannot answer.
	loginWindowSegmentHeight     = 40
	loginWindowEmailHeight       = 60
	loginWindowUnsupportedHeight = 40
	// The rows a visible status line and a refused-address field error add,
	// measured against the rendered form: the first notice line is a 16 DIP
	// text row plus the column's 12 DIP gap, each wrap after it another 16 DIP
	// text row, and the refusal under the server address field (only shown with
	// Advanced open) a measured 23 DIP error row.
	loginWindowNoticeHeight     = 28
	loginWindowNoticeLineHeight = 16
	loginWindowServerErrHeight  = 24
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
		log.Println("biowarden: login window has no parent window; not created")
		return
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           locales.LoginTitle(),
		Width:           loginWindowWidth,
		Height:          loginWindowHeight,
		MinWidth:        loginWindowWidth,
		MinHeight:       loginWindowTwoFactorHeight,
		DisableMaximize: true,
		Parent:          app.mainWindow,
		Modal:           true,
		Content:         ui.View(app.login.loginView),
	})
	app.loginWin = win
	// The window is built once per attempt, so its title follows the app's
	// language through a listener that is dropped with the window, as the tray
	// menu is rebuilt when the language changes.
	app.loginLocaleOff = mygo.App.OnLocaleChanged(func(string) {
		win.SetTitle(locales.LoginTitle())
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		// Closing the window means Cancel, and the state machine owns the
		// actual close: it destroys the window once the backend confirmed
		// the attempt was abandoned.
		e.PreventDefault()
		app.login.requestClose()
	})
	win.OnClosed(func() {
		app.unsubscribeLoginLocale()
		app.loginWin = nil
		app.login.showing = false
		// The account page behind the modal was disabled until now, so it has
		// to redraw with whatever the attempt landed.
		if app.mainWindow != nil {
			app.mainWindow.Invalidate()
		}
	})
}

// unsubscribeLoginLocale drops the login window's language listener. The window
// is destroyed once per attempt, so the off function runs with it; nil-ing the
// field keeps a second call a no-op.
func (app *appState) unsubscribeLoginLocale() {
	if app.loginLocaleOff != nil {
		app.loginLocaleOff()
		app.loginLocaleOff = nil
	}
}

// hideLoginWindow destroys the login window once the attempt settled. Destroy
// skips the OnClose listener, which would read the closing as a second cancel.
func (app *appState) hideLoginWindow() {
	if app.loginWin != nil {
		app.loginWin.Destroy()
	}
}

// resizeLoginWindow applies a target height to the window the login state
// opened. The width stays fixed; the state machine only decides the height, and
// only calls here when a form change actually moved its target.
func (app *appState) resizeLoginWindow(height int) {
	if app.loginWin == nil {
		return
	}
	app.loginWin.SetSize(loginWindowWidth, height)
}
