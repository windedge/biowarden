package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// sidebarWidth is the nav column's fixed width. The reference desktop client
// keeps its nav narrow so a 960-wide window leaves the content most of the
// room, and 200 still fits the longest label ("浏览器").
const sidebarWidth = 200

// The nav IDs double as the Sidebar's selection keys. Keeping them as
// strings is what ui.Sidebar wants; navPage is what the app stores.
const (
	navIDAccount  = "account"
	navIDBrowsers = "browsers"
	navIDSettings = "settings"
	navIDLog      = "log"
)

// navID is the Sidebar key for a page.
func (p navPage) navID() string {
	switch p {
	case pageBrowsers:
		return navIDBrowsers
	case pageSettings:
		return navIDSettings
	case pageLog:
		return navIDLog
	default:
		return navIDAccount
	}
}

// navPageOf maps a Sidebar choice back to the page enum.
func navPageOf(id string) navPage {
	switch id {
	case navIDBrowsers:
		return pageBrowsers
	case navIDSettings:
		return pageSettings
	case navIDLog:
		return pageLog
	default:
		return pageAccount
	}
}

// icon parses a 24x24 stroked icon, drawn in currentColor the way icon sets
// such as Lucide draw them. Keeping the paths here means no new dependency
// and no asset build step.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	iconAccount  = icon(`<circle cx="12" cy="8" r="4"/><path d="M4 20c0-3.3 3.6-6 8-6s8 2.7 8 6"/>`)
	iconBrowsers = icon(`<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a15 15 0 0 1 0 18a15 15 0 0 1 0-18"/>`)
	iconSettings = icon(`<path d="M4 7h8M18 7h2M4 17h2M10 17h10"/><circle cx="16" cy="7" r="2"/><circle cx="8" cy="17" r="2"/>`)
	iconLog      = icon(`<path d="M14 3v5h5"/><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M9 13h6M9 17h4"/>`)
)

// sidebar draws the left nav: four items with icons and labels, and the
// version and quit button pinned to the bottom. A click switches app.page
// through the Sidebar's string selection.
func (app *appState) sidebar(c *ui.Context) {
	t := c.Theme()
	selected := app.page.navID()
	ui.Column(c).Width(sidebarWidth).FillHeight().Shrink(0).
		Background(t.Surface).Children(func() {
		sb := ui.Sidebar(c, &selected, func() {
			ui.SidebarItem(c, navIDAccount, iconAccount, "账号")
			ui.SidebarItem(c, navIDBrowsers, iconBrowsers, "浏览器")
			ui.SidebarItem(c, navIDSettings, iconSettings, "设置")
			ui.SidebarItem(c, navIDLog, iconLog, "日志")
		})
		sb.Grow(1)
		if sb.Changed() {
			app.page = navPageOf(selected)
		}
		ui.Divider(c)
		ui.Column(c).Padding(12, 10).Gap(6).Children(func() {
			muted(c, appName+" v"+appVersion)
			// Quitting stays deliberate: the tray keeps the app alive when the
			// window is closed, so this is the window's only exit.
			ui.Button(c, "退出").OnClick(mygo.App.Quit)
		})
	})
}
