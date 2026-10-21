package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The pages of every language are written as pictures, so a text that runs
// off its area can be seen rather than measured: the shot shows the page as
// the user gets it, with the text cut where the window cuts it. The pictures
// are for the eye, not for the test to compare, so the test only checks that
// every page of every language was written.
func TestPagesAsPictures(t *testing.T) {
	if testing.Short() {
		t.Skip("the pictures are for the eye, and writing them is slow")
	}
	// The window is as narrow as it may be, so the content area is at its
	// smallest and a long text has the least room.
	const windowWidth = 800
	const windowHeight = 640
	dir := filepath.Join(os.TempDir(), "biowarden-shots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("make the shot directory: %v", err)
	}
	t.Logf("the pictures are in %s", dir)

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
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					// The sidebar and the page stand side by side, as they
					// do in the window, so the picture shows the page as
					// the user gets it.
					tester := ui.NewTester(func(c *ui.Context) {
						ui.Row(c).Fill().Children(func() {
							ui.Column(c).Width(sidebarWidth).FillHeight().Shrink(0)
							ui.Column(c).Grow(1).FillHeight().Children(func() {
								tc.view(c)
							})
						})
					}, windowWidth, windowHeight)
					tester.Frame()

					path := filepath.Join(dir, tag+"-"+tc.name+".png")
					f, err := os.Create(path)
					if err != nil {
						t.Fatalf("create %s: %v", path, err)
					}
					defer f.Close()
					if err := png.Encode(f, tester.Image()); err != nil {
						t.Fatalf("write %s: %v", path, err)
					}
				})
			}
		})
	}
}
