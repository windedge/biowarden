package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func itoa(n int) string { return strconv.Itoa(n) }

// appState is the GUI's whole state: which page is showing, that page's
// working data, and the shared notice lines. Page-specific fields live with
// their pages, in state_<page>.go, so parallel work on one page does not
// collide with another.
type appState struct {
	tab int

	browsers browsersState
	keys     keysState
	settings settingsState
	log      logState
}

// newAppState wires the pages onto their backend. The prototype's fake
// backend stays until the page tickets replace it; see backend.go.
func newAppState() *appState {
	return &appState{
		browsers: newBrowsersState(),
		keys:     newKeysState(),
		settings: newSettingsState(),
		log:      newLogState(),
	}
}

// tick advances simulated work and asks for the frame that finishes it.
// Every page calls it first, so a pending action still settles if the human
// switches tabs before it ends.
func (app *appState) tick(c *ui.Context) {
	app.browsers.tick(c)
	app.keys.tick(c)
	app.settings.tick(c)
}

// notice renders a page's status line: orange for what still needs a human,
// green for a success. Empty text draws nothing, so a settled page keeps its
// layout.
func notice(c *ui.Context, text string, ok bool) {
	if text == "" {
		return
	}
	col := c.Theme().Warning
	if ok {
		col = c.Theme().Success
	}
	ui.Text(c, text).FontSize(12).TextColor(col)
}

func muted(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted)
}

// groupBox stands in for the reference's GroupBox: visible grouping without
// a real form layout behind it.
func groupBox(c *ui.Context, title string, body func()) {
	t := c.Theme()
	ui.Column(c).Padding(12).Gap(8).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, title).Bold()
		body()
	})
}

// shortID keeps a long userId from pushing the row buttons out of view.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "..."
}

// trimSpace is the pages' shared input hygiene.
func trimSpace(s string) string { return strings.TrimSpace(s) }

// when formats a timestamp the way the reference's rows do.
func when(t time.Time) string { return t.Format("2006-01-02 15:04") }
