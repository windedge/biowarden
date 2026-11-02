package ui

import (
	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// sidebarWidth is the nav column's fixed width. The reference desktop client
// keeps its nav narrow so a 960-wide window leaves the content most of the
// room, and 200 still fits the longest label ("browsers").
const sidebarWidth = 200

// The nav IDs double as keys and a11y labels of the nav rows. navPage is
// what the app stores.
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

// navPageOf maps a nav row's ID back to the page enum.
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
// and no asset build step. The 2.0 stroke keeps the glyph solid at the nav's
// 16 DIP size, where 1.8 renders too thin to read.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.0" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	iconAccount  = icon(`<circle cx="12" cy="8" r="4"/><path d="M4 20c0-3.3 3.6-6 8-6s8 2.7 8 6"/>`)
	iconBrowsers = icon(`<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a15 15 0 0 1 0 18a15 15 0 0 1 0-18"/>`)
	iconSettings = icon(`<path d="M4 7h8M18 7h2M4 17h2M10 17h10"/><circle cx="16" cy="7" r="2"/><circle cx="8" cy="17" r="2"/>`)
	iconLog      = icon(`<path d="M14 3v5h5"/><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M9 13h6M9 17h4"/>`)
)

// sidebar draws the left nav: three main items, then settings pinned at the
// bottom. Rows are plain clickable Rows rather than ui.SidebarItem, so the
// chosen row controls its whole face: the wash, the ink of the label and the
// icon alike, and the leading bar inside the row's own rounded outline,
// which painting over a stock item could not reach. Quitting lives in the
// tray menu, which the close-to-tray window makes the deliberate exit.
func (app *appState) sidebar(c *ui.Context) {
	t := c.Theme()
	p := paletteFor(c)
	selected := app.page.navID()
	// navRow draws one item: icon, label, and the chosen row's wash with the
	// readable accent ink on it, the same pair a badge paints. The unchosen
	// rows keep the muted ink and take the surface hover under the pointer.
	// The bar is a 3 DIP ink pill inside the row's leading edge, centered
	// vertically, so it hugs the row's rounded outline instead of floating
	// outside it. OnClick fires for the pointer and for Enter or Space once
	// the row has the focus, which keeps the old keyboard nav.
	navRow := func(id string, ic *ui.SVG, label string) {
		row := ui.Row(c).Key(id).AlignItems(ui.Center).Gap(spaceSnug).
			Padding(spaceRow, spaceMedium).Radius(radiusControl).Shrink(0).
			Role(ui.RoleTreeItem).Focusable().Cursor(ui.CursorPointer).
			OnClick(func() { app.page = navPageOf(id) })
		ink, fill := t.Text, ui.Transparent
		iconColor, weight := t.TextMuted, 400
		if id == selected {
			ink = p.accentInkOnSoft()
			fill = p.sidebarWash()
			iconColor = ink
			weight = 600
		} else if row.Hovered() {
			fill = t.SurfaceHover
		}
		row.Background(fill).TextColor(ink).FontWeight(weight)
		if id == selected {
			row.DrawOver(func(pt *ui.Painter, r ui.Rect) {
				if row.FocusVisible() {
					pt.FocusRing(r, [4]float32{radiusControl, radiusControl, radiusControl, radiusControl})
				}
				barW := float32(3)
				barH := r.H * 0.55
				if barH < 14 {
					barH = 14
				}
				if max := r.H - 8; barH > max {
					barH = max
				}
				pt.Fill(ui.Rect{X: r.X + 1, Y: r.Y + (r.H-barH)/2, W: barW, H: barH},
					ink, barW/2)
			})
		}
		row.Children(func() {
			ui.Icon(c, ic).FontSize(16).TextColor(iconColor)
			ui.Text(c, label).Grow(1).SingleLine()
		})
	}
	ui.Column(c).Width(sidebarWidth).FillHeight().Shrink(0).
		Background(t.Surface).Children(func() {
		ui.Column(c).Grow(1).Padding(t.Space(2.5)).Gap(t.Space(0.5)).Children(func() {
			navRow(navIDAccount, iconAccount, locales.SidebarAccount())
			navRow(navIDBrowsers, iconBrowsers, locales.SidebarBrowsers())
			navRow(navIDLog, iconLog, locales.SidebarLog())
		})
		divider(c)
		ui.Column(c).Padding(t.Space(2.5)).Gap(t.Space(0.5)).Children(func() {
			navRow(navIDSettings, iconSettings, locales.SidebarSettings())
		})
	})
}
