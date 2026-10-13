// Package ui is GUI mode: a tray icon plus one window. The window's four pages
// (Browsers, Keys, Settings, Log) come from the UI prototype; this shell only
// carries the MyGo entry path.
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

var pages = []string{"Browsers", "Keys", "Settings", "Log"}

// Run blocks until the app quits.
func Run() error {
	mygo.App.SetName(appName)
	mygo.App.WhenReady(start)
	return mygo.App.Run()
}

// start runs on the main thread, which WhenReady guarantees.
func start() {
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:   appName,
		Width:   760,
		Height:  560,
		Content: ui.View(shell),
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

// shell stands in for the pages until the prototype fixes their layout.
func shell(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(24).Gap(10).Children(func() {
		ui.Text(c, appName).FontSize(22).Bold()
		ui.Text(c, "Shell only: the pages land with the UI prototype.").
			FontSize(12).TextColor(t.TextMuted)
		ui.Divider(c)
		for _, page := range pages {
			ui.Text(c, page)
		}
	})
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
