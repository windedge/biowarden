package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/internal/registry"
)

// shotBrowsersBackend is the fake the pictures are drawn from: a browser page
// with nothing in it reads as a broken product, so the shots show the three
// states a real machine has - ours and recently connected, ours and never
// connected, and a browser that is not installed. The layout tests keep the
// empty state; only the pictures need content.
func shotBrowsersBackend() *recordingBrowsersBackend {
	return &recordingBrowsersBackend{rows: []browserRow{
		{
			browser:         registry.Chrome,
			name:            "Chrome",
			path:            `C:\Users\ada\AppData\Local\biowarden\manifests\chrome.json`,
			installPath:     `C:\Users\ada\AppData\Local\biowarden\manifests\chrome.json`,
			state:           browserOurs,
			installed:       true,
			lastConnectedAt: time.Now().Add(-5 * time.Minute),
		},
		{
			browser:     registry.Edge,
			name:        "Microsoft Edge",
			path:        `C:\Users\ada\AppData\Local\biowarden\manifests\edge.json`,
			installPath: `C:\Users\ada\AppData\Local\biowarden\manifests\edge.json`,
			state:       browserOurs,
			installed:   true,
		},
		{
			browser:     registry.Firefox,
			name:        "Firefox",
			installPath: `C:\Users\ada\AppData\Local\biowarden\manifests\firefox.json`,
			state:       browserMissing,
		},
	}}
}

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

	// Both appearances, every language: the day shots carry no suffix, the
	// night ones "-dark", so the two sets sit side by side in the directory.
	for _, dark := range []bool{false, true} {
		for _, tag := range []string{"en", "de", "fr", "es", "ja", "zh-Hans", "zh-Hant"} {
			t.Run(appearanceName(dark)+"/"+tag, func(t *testing.T) {
				appLanguage(t, tag)
				app := &appState{browsers: newBrowsersStateWith(shotBrowsersBackend())}
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
						// the user gets it. The test's own root paints itself
						// with applyPalette, which the pages cannot do for it.
						tester := ui.NewTester(func(c *ui.Context) {
							applyPalette(c)
							ui.Row(c).Fill().Children(func() {
								app.sidebar(c)
								ui.Column(c).Grow(1).FillHeight().Children(func() {
									tc.view(c)
								})
							})
						}, windowWidth, windowHeight)
						tester.SetDark(dark)
						tester.Frame()

						name := tag + "-" + tc.name
						if dark {
							name += "-dark"
						}
						path := filepath.Join(dir, name+".png")
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
}
