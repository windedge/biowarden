// Package ui is GUI mode: a tray icon plus one window. The window's four pages
// (Browsers, Keys, Settings, Log) are a clickable prototype built from fake
// in-memory data, so a human can settle the interactions and the layout before
// the platform seams behind them are wired up.
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

// Run blocks until the app quits.
func Run() error {
	mygo.App.SetName(appName)
	mygo.App.WhenReady(start)
	return mygo.App.Run()
}

// view builds the whole window from the prototype state, rebuilding after
// every click or switch.
func (app *protoState) view(c *ui.Context) {
	app.tick(c)
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Tabs(c, &app.tab, "浏览器", "密钥", "设置", "日志")
		switch app.tab {
		case 0:
			app.browsersPage(c)
		case 1:
			app.keysPage(c)
		case 2:
			app.settingsPage(c)
		case 3:
			app.logPage(c)
		}
	})
}

// start runs on the main thread, which WhenReady guarantees.
func start() {
	app := newProtoState()
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:   appName,
		Width:   820,
		Height:  620,
		Content: ui.View(app.view),
	})
	// Closing the window returns to the tray, as in the reference app; quitting
	// stays deliberate, through the tray menu.
	win.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		win.Hide()
	})

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
		defer tray.Destroy()
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
