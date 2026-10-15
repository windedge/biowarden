// Package ui is GUI mode: a tray icon plus one window. The window is a left
// sidebar of four pages (Account, Browsers, Settings, Log) next to the page's
// content. The account page runs the real login, logout and key-refresh flow
// through internal/auth, with Windows Hello gating every key write.
package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
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
		Title:     appName,
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
		Icon:    trayIcon(),
		ToolTip: appName,
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "Open " + appName, Click: func(*mygo.MenuItem, *mygo.Window) { win.Show() }},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}),
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
	win.Show()
}

// trayIcon draws a ring, blue so it stays visible on light and dark taskbars.
func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	ink := color.NRGBA{R: 79, G: 124, B: 255, A: 255}
	draw.Draw(img, image.Rect(6, 4, 26, 28), image.NewUniform(ink), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(9, 7, 23, 25), image.NewUniform(color.NRGBA{}), image.Point{}, draw.Src)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
