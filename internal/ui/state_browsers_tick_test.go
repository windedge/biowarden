package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/internal/registry"
)

// countingBrowsersBackend counts the refreshes the page makes, so a test can
// watch how often the tick re-reads the registry.
type countingBrowsersBackend struct {
	fakeBrowsersBackend
	refreshes int
}

func (f *countingBrowsersBackend) refresh() ([]browserRow, error) {
	f.refreshes++
	return f.rows, nil
}

// The tick re-reads the registry on every rebuild, and a rebuild comes with
// every animation frame: a button's AnimateColor fill keeps frames coming while
// it fades, so hovering a button used to re-read the manifest keys once per
// frame. The throttle keeps the background path at one read per
// browsersReloadInterval; the counted rebuilds here are the animation frames a
// hover produces. Reloads through the actions stay immediate (each refresh here
// is one read; a write calls reload directly, not through the tick).
func TestBrowsersTickThrottlesPerFrameReloads(t *testing.T) {
	appLanguage(t, "en")
	backend := &countingBrowsersBackend{fakeBrowsersBackend: fakeBrowsersBackend{
		rows: []browserRow{
			{browser: registry.Firefox, name: "Firefox", path: `C:\biowarden\manifests\firefox.json`, state: browserOurs, installed: true},
		},
	}}
	app := &appState{browsers: newBrowsersStateWith(backend)}
	tester := ui.NewTester(app.view, 800, 640)
	// The tester has no native window to hover, so the animation frames a hover
	// would keep coming are driven by frames alone: each Frame() is one rebuild,
	// and the pre-existing rows from newBrowsersStateWith are one refresh. The
	// first Frame's tick is the exempt first tick, so it re-reads right away.
	tester.Frame()

	// Switch to the browsers page: the sidebar's item is labeled with it. The
	// click's rebuild tick is inside the throttle window the first tick just
	// stamped, so the count stays at the two reads above.
	if err := tester.Click("Browsers"); err != nil {
		t.Fatalf("open the browsers page: %v", err)
	}
	before := backend.refreshes
	if before != 2 {
		t.Fatalf("refreshes after opening the page = %d, want 2 (construction plus the first frame's exempt tick)", before)
	}
	// Animation frames: a rebuild per frame with no action between them.
	for i := 0; i < 5; i++ {
		tester.Frame()
	}
	if got := backend.refreshes - before; got != 0 {
		t.Fatalf("refreshes over 5 idle rebuilds = %d, want 0 (throttled after the exempt first tick)", got)
	}

	// Inside the throttle window a write still refreshes at once, and the tick
	// after it does not read again: one registry read per human action.
	if err := tester.Click("Uninstall"); err != nil {
		t.Fatalf("click Uninstall: %v", err)
	}
	tester.Frame()
	after := backend.refreshes
	// The click's reload, then two rebuilds (the click's frame and the frame
	// after) must not read again inside the window.
	tester.Frame()
	tester.Frame()
	if got := backend.refreshes - after; got != 0 {
		t.Fatalf("refreshes after an action inside the throttle window = %d, want 0", got)
	}

	// When the interval passes, the background path reads again. The tick
	// throttles on the wall clock, which Advance's frozen frame clock cannot
	// move, so a real sleep opens the window instead.
	time.Sleep(browsersReloadInterval + 10*time.Millisecond)
	tester.Frame()
	if backend.refreshes <= after {
		t.Fatalf("no refresh after the throttle window passed")
	}
}
