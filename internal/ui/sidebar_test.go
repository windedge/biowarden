package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The sidebar is nav only; quitting lives in the tray menu, which the
// close-to-tray window makes the deliberate exit. A second exit in the nav
// would only be easier to hit by accident, so the sidebar must not offer one.
func TestSidebarHasNoVersionLabel(t *testing.T) {
	appLanguage(t, "zh-Hans")
	app := &appState{}
	tester := ui.NewTester(func(c *ui.Context) { app.sidebar(c) }, sidebarWidth, 400)

	if tester.HasText("biowarden v"+appVersion) || tester.HasText("v"+appVersion) {
		t.Fatalf("the sidebar still shows the version; texts = %v", tester.Texts())
	}
	if tester.HasText("退出") {
		t.Fatalf("the sidebar still offers a quit button; texts = %v", tester.Texts())
	}
}

// Settings left the main nav for the bottom of the sidebar and it still
// switches to the settings page from there.
func TestSidebarPinsSettingsToTheBottom(t *testing.T) {
	appLanguage(t, "zh-Hans")
	app := &appState{}
	tester := ui.NewTester(func(c *ui.Context) { app.sidebar(c) }, sidebarWidth, 400)

	// The main nav keeps account, browsers and log, in that order; settings
	// follows the divider.
	want := []string{"账号", "浏览器", "日志", "设置"}
	got := tester.Texts()
	if len(got) < len(want) {
		t.Fatalf("the sidebar shows %v, want the items %v", got, want)
	}
	for i, label := range want {
		if got[i] != label {
			t.Fatalf("the sidebar shows %v, want %v", got, want)
		}
	}

	// The bottom settings item still drives app.page.
	if err := tester.Click("设置"); err != nil {
		t.Fatalf("the settings nav item is not clickable: %v", err)
	}
	if app.page != pageSettings {
		t.Fatalf("page = %d after clicking the settings item, want the settings page", app.page)
	}
}

// The main nav keeps exactly three items after settings moved out, and every
// one of them still switches pages.
func TestSidebarMainNavSwitchesPages(t *testing.T) {
	appLanguage(t, "zh-Hans")
	for _, tc := range []struct {
		label string
		page  navPage
	}{
		{"账号", pageAccount},
		{"浏览器", pageBrowsers},
		{"日志", pageLog},
	} {
		app := &appState{}
		tester := ui.NewTester(func(c *ui.Context) { app.sidebar(c) }, sidebarWidth, 400)
		if err := tester.Click(tc.label); err != nil {
			t.Fatalf("the %s nav item is not clickable: %v", tc.label, err)
		}
		if app.page != tc.page {
			t.Fatalf("page = %d after clicking %s, want %d", app.page, tc.label, tc.page)
		}
	}
}
