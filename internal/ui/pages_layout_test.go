package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// The About note on the settings page is a long paragraph in a form, whose
// label column grows with the longest label of the page. In a verbose
// language that leaves the note less room than the window has, and the note
// then runs past the window's edge, where it is cut. The test builds the
// real page in every language and measures the note, so the text stays
// inside the window. The bug it pins was the page's padding, written in the
// order of the sides rather than MyGo's CSS order, which left the right
// margin a few DIPs short of what the text needs.
func TestSettingsAboutNoteFitsTheWindow(t *testing.T) {
	// The window is as narrow as it may be, so the content area is at its
	// smallest and a long note has the least room.
	const windowWidth = 800
	// The page's padding, which keeps the text off the window's edge.
	const pagePadding = 16
	for _, tag := range []string{"en", "de", "fr", "es", "ja", "zh-Hans", "zh-Hant"} {
		t.Run(tag, func(t *testing.T) {
			appLanguage(t, tag)
			app := &appState{}
			app.settings.logLevel = logLevelError
			app.settings.logPath = `C:\Users\ada\AppData\Local\biowarden\biowarden.log`
			// The sidebar and the page stand side by side, as they do in
			// the window, so the note gets the room the content area
			// really has.
			tester := ui.NewTester(func(c *ui.Context) {
				ui.Row(c).Fill().Children(func() {
					ui.Column(c).Width(sidebarWidth).FillHeight().Shrink(0)
					ui.Column(c).Grow(1).FillHeight().Children(func() {
						app.settingsPage(c)
					})
				})
			}, windowWidth, 640)
			tester.Frame()

			note := locales.In(tag).SettingsAboutText()
			r, ok := tester.Find(note)
			if !ok {
				t.Fatalf("the note is not shown")
			}
			// The text must end inside the page's content area: a note
			// that reaches past it runs under the window's edge, where
			// it is cut.
			if right := r.X + r.W; right > windowWidth-pagePadding {
				t.Errorf("the note ends at %v, past the content area of %v", right, windowWidth-pagePadding)
			}
		})
	}
}
