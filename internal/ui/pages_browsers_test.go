package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/internal/hoststate"
	"github.com/windedge/biowarden/internal/registry"
)

func TestRelativeTime(t *testing.T) {
	appLanguage(t, "zh-Hans")
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		at   time.Time
		want string
	}{
		{now, "刚刚"},
		{now.Add(-30 * time.Second), "刚刚"},
		{now.Add(-5 * time.Minute), "5 分钟前"},
		{now.Add(-59 * time.Minute), "59 分钟前"},
		{now.Add(-2 * time.Hour), "2 小时前"},
		{now.Add(-23 * time.Hour), "23 小时前"},
		{now.Add(-25 * time.Hour), "1 天前"},
		{now.Add(-10 * 24 * time.Hour), "10 天前"},
		{now.Add(-40 * 24 * time.Hour), "2026-01-29"},
		// A clock that jumped backwards must not print a negative age.
		{now.Add(time.Hour), "刚刚"},
	}
	for _, tc := range cases {
		if got := relativeTime(now, tc.at); got != tc.want {
			t.Errorf("relativeTime(%v) = %q, want %q", tc.at, got, tc.want)
		}
	}
}

func TestConnectionLabel(t *testing.T) {
	appLanguage(t, "zh-Hans")
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	if got := connectionLabel(now, time.Time{}); got != "扩展未连接" {
		t.Fatalf("connectionLabel(zero) = %q, want the placeholder", got)
	}
	if got := connectionLabel(now, now.Add(-3*time.Hour)); got != "已连接过 · 3 小时前" {
		t.Fatalf("connectionLabel = %q, want a connected label", got)
	}
}

// The connection caption paints its ink on the page's gray and on a white
// card alike, so both surfaces are measured: a connected row must clear the
// 4.5:1 a label needs (the raw success color was 3.62:1 on a card), and the
// never-connected gray must keep doing what it already did.
func TestExtensionColorClearsFourAndAHalfToOne(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := lightPalette()
		if dark {
			p = darkPalette()
		}
		for name, at := range map[string]time.Time{"connected": time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC), "never": {}} {
			ink := extensionColor(p, at)
			for surf, bg := range map[string]ui.Color{"card": p.surface, "page": p.bg} {
				got := contrast(ink, bg)
				t.Logf("%s %s caption on %s: %v on %v = %.2f:1", appearanceName(dark), name, surf, ink, bg, got)
				if got < 4.5 {
					t.Errorf("%s %s caption on the %s: contrast %.2f:1, want >= 4.5:1", appearanceName(dark), name, surf, got)
				}
			}
		}
	}
}

// The Chrome row stands for both chrome.exe and chromium.exe; the other rows
// map one-to-one.
func TestBrowserIDs(t *testing.T) {
	chrome := browserIDs(registry.Chrome)
	if len(chrome) != 2 {
		t.Fatalf("browserIDs(Chrome) = %v, want both engines", chrome)
	}
	seen := map[string]bool{}
	for _, id := range chrome {
		seen[id] = true
	}
	if !seen[hoststate.BrowserChrome] || !seen[hoststate.BrowserChromium] {
		t.Fatalf("browserIDs(Chrome) = %v, want chrome and chromium", chrome)
	}
	if got := browserIDs(registry.Edge); len(got) != 1 || got[0] != hoststate.BrowserEdge {
		t.Fatalf("browserIDs(Edge) = %v, want [edge]", got)
	}
	if got := browserIDs(registry.Firefox); len(got) != 1 || got[0] != hoststate.BrowserFirefox {
		t.Fatalf("browserIDs(Firefox) = %v, want [firefox]", got)
	}
}

func TestLastConnectedForTakesNewestID(t *testing.T) {
	older := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(48 * time.Hour)
	connections := map[string]time.Time{
		hoststate.BrowserChrome:   older,
		hoststate.BrowserChromium: newer,
	}
	if got := lastConnectedFor(connections, registry.Chrome); !got.Equal(newer) {
		t.Fatalf("lastConnectedFor = %v, want the newer chromium stamp %v", got, newer)
	}
	if got := lastConnectedFor(connections, registry.Firefox); !got.IsZero() {
		t.Fatalf("lastConnectedFor(Firefox) = %v, want zero", got)
	}
	if got := lastConnectedFor(nil, registry.Chrome); !got.IsZero() {
		t.Fatalf("lastConnectedFor(nil) = %v, want zero", got)
	}
}

// A browser this machine does not have is marked "未安装" by its name, and its
// install action is disabled: there is no registration point to write.
func TestBrowsersRowMarksAndDisablesAbsentBrowser(t *testing.T) {
	appLanguage(t, "zh-Hans")
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Firefox, name: "Firefox", path: `C:\official\firefox.json`, state: browserForeign},
	}}
	app := &appState{browsers: newBrowsersStateWith(backend)}
	tester := ui.NewTester(func(c *ui.Context) { app.browserRow(c, 0) }, 640, 120)
	tester.Frame()

	if !tester.HasText("未安装") {
		t.Fatalf("row does not mark the absent browser; texts = %v", tester.Texts())
	}
	if err := tester.Click("覆盖安装"); err != nil {
		t.Fatalf("click the disabled install button: %v", err)
	}
	if app.browsers.confirmCover != -1 {
		t.Fatalf("the disabled button started a confirmation (confirmCover = %d)", app.browsers.confirmCover)
	}
}

// The marker belongs only to absent browsers: an installed one is not marked.
func TestBrowsersRowDoesNotMarkInstalledBrowser(t *testing.T) {
	appLanguage(t, "zh-Hans")
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Firefox, name: "Firefox", path: `C:\official\firefox.json`, state: browserForeign, installed: true},
	}}
	app := &appState{browsers: newBrowsersStateWith(backend)}
	tester := ui.NewTester(func(c *ui.Context) { app.browserRow(c, 0) }, 640, 120)
	tester.Frame()

	if tester.HasText("未安装") {
		t.Fatalf("installed row shows the absent marker; texts = %v", tester.Texts())
	}
	if err := tester.Click("覆盖安装"); err != nil {
		t.Fatalf("click the install button: %v", err)
	}
	if app.browsers.confirmCover != 0 {
		t.Fatalf("the enabled button did not start a confirmation (confirmCover = %d)", app.browsers.confirmCover)
	}
}

// The plain "安装" button is the disabled one on a row with no registration,
// so the click never reaches the registry.
func TestBrowsersRowDisablesInstallForAbsentBrowser(t *testing.T) {
	appLanguage(t, "zh-Hans")
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Firefox, name: "Firefox", state: browserMissing},
	}}
	app := &appState{browsers: newBrowsersStateWith(backend)}
	tester := ui.NewTester(func(c *ui.Context) { app.browserRow(c, 0) }, 640, 120)
	tester.Frame()

	if err := tester.Click("安装"); err != nil {
		t.Fatalf("click the install button: %v", err)
	}
	if len(backend.installs) != 0 {
		t.Fatalf("installs = %v, want none from a disabled button", backend.installs)
	}
}

// An installed row keeps the working "安装" button.
func TestBrowsersRowInstallsWhenBrowserPresent(t *testing.T) {
	appLanguage(t, "zh-Hans")
	backend := &recordingBrowsersBackend{rows: []browserRow{
		{browser: registry.Firefox, name: "Firefox", state: browserMissing, installed: true},
	}}
	app := &appState{browsers: newBrowsersStateWith(backend)}
	tester := ui.NewTester(func(c *ui.Context) { app.browserRow(c, 0) }, 640, 120)
	tester.Frame()

	if err := tester.Click("安装"); err != nil {
		t.Fatalf("click the install button: %v", err)
	}
	if len(backend.installs) != 1 || backend.installs[0] != "Firefox" {
		t.Fatalf("installs = %v, want [Firefox]", backend.installs)
	}
}

func TestRealBrowsersRefreshAttachesConnections(t *testing.T) {
	fake := newFakeRegistrar()
	fake.status[registry.Firefox] = registry.Status{Registered: true, Path: `C:\ours\firefox.json`, IsOurs: true}
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	connections := func() (map[string]time.Time, error) {
		return map[string]time.Time{hoststate.BrowserFirefox: at}, nil
	}

	rows, err := newRealBrowsersWith(fake, connections, nil).refresh()
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !rows[2].lastConnectedAt.Equal(at) {
		t.Fatalf("Firefox lastConnectedAt = %v, want %v", rows[2].lastConnectedAt, at)
	}
	if !rows[0].lastConnectedAt.IsZero() {
		t.Fatalf("Chrome lastConnectedAt = %v, want zero", rows[0].lastConnectedAt)
	}
}

// A failing state read only loses the indicator; the registry rows must still
// come back so the page stays usable.
func TestRealBrowsersRefreshSurvivesConnectionReadError(t *testing.T) {
	fake := newFakeRegistrar()
	connections := func() (map[string]time.Time, error) {
		return nil, errors.New("state dir unreadable")
	}
	rows, err := newRealBrowsersWith(fake, connections, nil).refresh()
	if err != nil {
		t.Fatalf("refresh returned %v, want the rows with no indicator", err)
	}
	if len(rows) != len(registry.Browsers) {
		t.Fatalf("refresh returned %d rows, want %d", len(rows), len(registry.Browsers))
	}
	for _, row := range rows {
		if !row.lastConnectedAt.IsZero() {
			t.Fatalf("row %s has a connection time despite the read failure", row.name)
		}
	}
}
