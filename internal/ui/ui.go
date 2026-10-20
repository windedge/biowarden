// Package ui is GUI mode: a tray icon plus one window. The window is a left
// sidebar of four pages (Account, Browsers, Settings, Log) next to the page's
// content. The account page runs the real login, logout and key-refresh flow
// through internal/auth, with Windows Hello gating every key write.
package ui

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/windedge/biowarden/locales"
)

const appName = "biowarden"

// appVersion is shown in the sidebar's footer and on the Settings page, and
// substitutes into fakeLog. Keep it in sync with mygo.json's version.
const appVersion = "0.1.0"

// Run blocks until the app quits.
func Run() error {
	mygo.App.SetName(appName)
	mygo.App.WhenReady(start)
	return mygo.App.Run()
}

// view builds the whole window from the prototype state, rebuilding after
// every click or switch: a fixed sidebar on the left, the chosen page
// filling the rest.
func (app *appState) view(c *ui.Context) {
	app.tick(c)
	t := c.Theme()
	ui.Row(c).Fill().Background(t.Background).Children(func() {
		app.sidebar(c)
		ui.Column(c).Grow(1).FillHeight().Children(func() {
			switch app.page {
			case pageBrowsers:
				app.browsersPage(c)
			case pageSettings:
				app.settingsPage(c)
			case pageLog:
				app.logPage(c)
			default:
				app.accountPage(c)
			}
		})
	})
}

// start runs on the main thread, which WhenReady guarantees.
func start() {
	app := newAppState()
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     locales.AppTitle(),
		Width:     960,
		Height:    640,
		MinWidth:  800,
		MinHeight: 520,
		Content:   ui.View(app.view),
	})
	// The login window is this window's modal child, so the page that opens it
	// needs the handle.
	app.mainWindow = win
	// Closing the window returns to the tray, as in the reference app; quitting
	// stays deliberate, through the tray menu. App.Quit must still be able to
	// close the window, so OnBeforeQuit lets that close pass through.
	quitting := false
	win.OnClose(func(e *mygo.CloseEvent) {
		if !quitting {
			e.PreventDefault()
			win.Hide()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })

	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:    appIconPNG,
		ToolTip: appName,
		Menu:    trayMenu(win),
	})
	if err != nil {
		// Unlocking never depended on the GUI running, so a missing tray only
		// costs the menu.
		log.Println("biowarden: tray unavailable:", err)
	} else {
		// The tray lives until the app quits; destroying it here would remove
		// it the moment start returns.
		mygo.App.OnWillQuit(func(*mygo.QuitEvent) { tray.Destroy() })
	}
	// What the app set once - the window title and its tray menu's own label -
	// is set again in the new language.
	mygo.App.OnLocaleChanged(func(string) {
		win.SetTitle(locales.AppTitle())
		if tray != nil {
			tray.SetMenu(trayMenu(win))
		}
	})
	win.Show()
	// Zero-configuration first launch: register the browsers this machine has,
	// then ask the human to log in. It runs after the window is up because the
	// login window is that window's modal child.
	app.runFirstRun()
}

// trayMenu builds the tray's menu in the language the app shows, again when the
// language changes: RoleQuit relabels itself, the app's own item does not.
func trayMenu(win *mygo.Window) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.TrayOpen(appName), Click: func(*mygo.MenuItem, *mygo.Window) { win.Show() }},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	})
}

// appIconPNG is resources/icon.png, the same square icon mygo.json points at,
// so the tray shows what the taskbar's icon shows. The tray's SetImage
// decodes the PNG's true size and CreateIconFromResourceEx scales it to the
// notification area's size; one 1024x1024 source feeds both roles.
//
//go:embed resources/icon.png
var appIconPNG []byte
