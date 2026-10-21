package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// The pages' texts must stay inside the content area in every language: a
// translation longer than the English one, in a label column that grows
// with it, can run past the content area's edge, where it is cut. The tester
// renders the real pages and reports where each text is, so a text that
// reaches past the content area shows here rather than on the user's screen.
//
// The baseline is the content area, not the window's edge, and that is the
// whole point of the test: a text that overflows its content area by a few
// DIPs still sits inside the window, so a test that only compared against the
// window's bounds would pass on a page whose text is visibly cut. The
// padding of the pages is the gap between the content area and the window's
// edge, so the texts must end before it.
func TestPagesKeepTheirTextsInsideTheWindow(t *testing.T) {
	// The window is as narrow as it may be, so the content area is at its
	// smallest and a long text has the least room.
	const windowWidth = 800
	const windowHeight = 640
	// The pages' padding, which keeps the texts off the window's edge and
	// marks the content area's bounds.
	const pagePadding = 16
	for _, tag := range []string{"en", "de", "fr", "es", "ja", "zh-Hans", "zh-Hant"} {
		t.Run(tag, func(t *testing.T) {
			appLanguage(t, tag)
			app := &appState{browsers: newBrowsersStateWith(&recordingBrowsersBackend{})}
			app.settings.logLevel = logLevelError
			app.settings.logPath = `C:\Users\ada\AppData\Local\biowarden\biowarden.log`

			cases := []struct {
				name string
				view func(*ui.Context)
			}{
				{"account", app.accountPage},
				{"browsers", app.browsersPage},
				{"settings", app.settingsPage},
				{"log", app.logPage},
			}
			// The tester reports the box of the element that shows a text
			// or that is labeled with it, and a widget's box is the
			// widget's own: the log path field is a fixed width and does
			// not fit the column of a verbose language, so its box says
			// nothing about a text running off the page. The labels of
			// the pages' own widgets are left out of the measure.
			widgetLabels := map[string]bool{
				locales.SettingsLogPathLabel(): true,
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					// The sidebar and the page stand side by side, as they
					// do in the window, so a text gets the room the
					// content area really has.
					tester := ui.NewTester(func(c *ui.Context) {
						ui.Row(c).Fill().Children(func() {
							ui.Column(c).Width(sidebarWidth).FillHeight().Shrink(0)
							ui.Column(c).Grow(1).FillHeight().Children(func() {
								tc.view(c)
							})
						})
					}, windowWidth, windowHeight)
					tester.Frame()

					for _, text := range tester.Texts() {
						if strings.TrimSpace(text) == "" {
							continue
						}
						if widgetLabels[text] {
							continue
						}
						r, ok := tester.Find(text)
						if !ok {
							continue
						}
						right := r.X + r.W
						bottom := r.Y + r.H
						// The text must stay inside the content area: it ends
						// before the padding that keeps it off the window's
						// edge, or it is cut there.
						if right > windowWidth-pagePadding {
							t.Errorf("%q ends at %v, past the content area of %v",
								text, right, windowWidth-pagePadding)
						}
						if bottom > windowHeight-pagePadding {
							t.Errorf("%q ends at %v, past the content area of %v",
								text, bottom, windowHeight-pagePadding)
						}
					}
				})
			}
		})
	}
}

// The Language row must offer every language the app ships, in the language
// itself, so a reader finds their own.
func TestLanguageRowOffersEveryLanguage(t *testing.T) {
	appLanguage(t, "en")
	app := &appState{}
	app.settings.locale = &fakeLocale{
		shown:     "en",
		langs:     []string{"en", "zh-Hans", "zh-Hant", "ja", "de", "fr", "es"},
		preferred: []string{"en-US"},
	}
	tags, labels := app.settings.languageOptions()
	if len(tags) != len(labels) {
		t.Fatalf("the row has %d tags and %d labels", len(tags), len(labels))
	}
	if tags[0] != "" || labels[0] != locales.LanguageSystem() {
		t.Errorf("the first choice is %q/%q, want the system's own", tags[0], labels[0])
	}
	for _, want := range []string{"English", "简体中文", "繁體中文", "日本語", "Deutsch", "français", "español"} {
		if !contains(labels, want) {
			t.Errorf("the row has no %q, only %v", want, labels)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
