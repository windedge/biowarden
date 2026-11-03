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

// The registry read is only worth its cost while the rows are on screen: frames
// on another page must not reach it, however many arrive.
func TestBrowsersTickSkipsFramesOffScreen(t *testing.T) {
	appLanguage(t, "en")
	backend := &countingBrowsersBackend{fakeBrowsersBackend: fakeBrowsersBackend{
		rows: []browserRow{
			{browser: registry.Firefox, name: "Firefox", path: `C:\biowarden\manifests\firefox.json`, state: browserOurs, installed: true},
		},
	}}
	app := &appState{browsers: newBrowsersStateWith(backend)}
	tester := ui.NewTester(app.view, 800, 640)

	// The app lands on the account page, so the construction read is the only
	// one the registry sees until the browsers page is opened.
	built := backend.refreshes
	if built != 1 {
		t.Fatalf("refreshes after construction = %d, want 1", built)
	}
	for i := 0; i < 5; i++ {
		tester.Frame()
	}
	if got := backend.refreshes; got != built {
		t.Fatalf("refreshes over 5 frames on the account page = %d, want %d: the page is not on screen", got, built)
	}

	// Opening the page refreshes at once, even inside the interval the
	// construction read stamped.
	if err := tester.Click("Browsers"); err != nil {
		t.Fatalf("open the browsers page: %v", err)
	}
	onPage := backend.refreshes
	if onPage != built+1 {
		t.Fatalf("refreshes after opening the page = %d, want %d: entering the page refreshes once", onPage, built+1)
	}

	// Leaving drops the throttle's stamps, so coming back inside the interval
	// still refreshes instead of showing the rows from before the human left.
	if err := tester.Click("Account"); err != nil {
		t.Fatalf("go back to the account page: %v", err)
	}
	tester.Frame()
	if got := backend.refreshes; got != onPage {
		t.Fatalf("refreshes after leaving the page = %d, want %d", got, onPage)
	}
	if err := tester.Click("Browsers"); err != nil {
		t.Fatalf("reopen the browsers page: %v", err)
	}
	if got := backend.refreshes; got != onPage+1 {
		t.Fatalf("refreshes after coming back to the page = %d, want %d: the stale rows must not survive the return", got, onPage+1)
	}
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
	// app lands on the account page, where the tick reads nothing; the click
	// below is the first build of the browsers page, so its tick is the exempt
	// first one and reads right away.
	tester.Frame()

	// Switch to the browsers page: the sidebar's item is labeled with it. That
	// build's tick is the exempt first tick on the page, so the count reaches the
	// two reads above: construction, then the page's first frame.
	if err := tester.Click("Browsers"); err != nil {
		t.Fatalf("open the browsers page: %v", err)
	}
	before := backend.refreshes
	if before != 2 {
		t.Fatalf("refreshes after opening the page = %d, want 2 (construction plus the first tick on the page)", before)
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
