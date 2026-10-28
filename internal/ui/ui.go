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

// appName is the app's name as people read it. It is display only: the
// identifiers the app stores data under (the executable, the registry keys,
// the credential prefix, the protocol's app id) are spelled elsewhere and
// must not follow this name.
const appName = "Biowarden"

// appVersion is shown on the Settings page and substitutes into fakeLog.
// Keep it in sync with mygo.json's version.
const appVersion = "0.1.4"

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
	applyPalette(c)
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
	// Before the first window: the Windows backend fixes the appearance it hands
	// out at NewWindow, so a forced light or dark set afterwards would show the
	// system's own for a frame first. "system" is mygo's default and is left
	// alone, which keeps the app following the desktop until a human chooses.
	if app.settings.appearance != appearanceSystem {
		mygo.Theme.SetSource(app.settings.appearance.source())
	}
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
		// Windows convention: left click activates, right click opens the
		// menu. The backend fires OnClick on left clicks even with a menu set.
		tray.OnClick(func() { Reveal(win) })
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

// Reveal brings a hidden window back to the tray user and focuses it either
// way, so a tray click or a second launch means "show me the window".
func Reveal(w *mygo.Window) {
	if !w.IsVisible() {
		w.Show()
	}
	w.Focus()
}

// trayMenu builds the tray's menu in the language the app shows, again when the
// language changes: RoleQuit relabels itself, the app's own item does not.
func trayMenu(win *mygo.Window) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: trayOpenLabel(), Click: func(*mygo.MenuItem, *mygo.Window) { win.Show() }},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	})
}

// trayOpenLabel is the tray menu's item that shows the window.
//
// The trailing space is not a mistake: MyGo sizes a menu item from the width
// of its text, but Windows draws that text a few pixels wider (the item's
// rect is inset inside the menu, which MyGo's measurement does not know),
// and cuts what does not fit. The cut falls on the last letter, which is why
// the app's name lost its "n". The space gives the text the room it needs;
// the menu shows it as nothing.
func trayOpenLabel() string {
	return locales.TrayOpen(appName) + " "
}

// appIconPNG is resources/icon.png, the same square icon mygo.json points at,
// so the tray shows what the taskbar's icon shows. The tray's SetImage
// decodes the PNG's true size and CreateIconFromResourceEx scales it to the
// notification area's size; one 1024x1024 source feeds both roles.
//
//go:embed resources/icon.png
var appIconPNG []byte
