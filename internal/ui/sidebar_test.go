package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The sidebar is nav plus the version; quitting lives in the tray menu, which
// the close-to-tray window makes the deliberate exit. A second exit in the nav
// would only be easier to hit by accident, so the sidebar must not offer one.
func TestSidebarKeepsTheVersionWithoutAQuitButton(t *testing.T) {
	appLanguage(t, "zh-Hans")
	app := &appState{}
	tester := ui.NewTester(func(c *ui.Context) { app.sidebar(c) }, sidebarWidth, 400)

	if !tester.HasText("biowarden v" + appVersion) {
		t.Fatalf("the sidebar footer lost the version; texts = %v", tester.Texts())
	}
	if tester.HasText("退出") {
		t.Fatalf("the sidebar still offers a quit button; texts = %v", tester.Texts())
	}

	if err := tester.Click("设置"); err != nil {
		t.Fatalf("the settings nav item is not clickable: %v", err)
	}
	if app.page != pageSettings {
		t.Fatalf("page = %d after clicking the settings item, want the settings page", app.page)
	}
}
