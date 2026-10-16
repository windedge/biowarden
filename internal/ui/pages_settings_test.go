package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The settings page's fields used to stretch to the window width, so a wide
// window showed a read-only address in a box the size of the page. The capped
// controls carry a label, which the headless tester finds them by, so their
// width is pinned here; the labels also name the read-only fields for
// assistive technology.
func TestSettingsControlsKeepTheFieldWidth(t *testing.T) {
	app := &appState{}
	app.account.session.serverURL = "https://vault.example.com"
	app.settings.logLevel = logLevels[0]
	app.settings.logPath = `C:\Users\ada\AppData\Local\biowarden\biowarden.log`
	tester := ui.NewTester(app.settingsPage, 1600, 900)
	tester.Frame()

	cases := []struct {
		label string
		want  float32
	}{
		{"服务器地址（只读）", settingsFieldWidth},
		{"日志级别", settingsSelectWidth},
		{"日志路径（只读）", settingsFieldWidth},
	}
	for _, tc := range cases {
		r, ok := tester.Find(tc.label)
		if !ok {
			t.Fatalf("no settings control labeled %q", tc.label)
		}
		if r.W > tc.want {
			t.Errorf("control %q is %.0f DIPs wide, want <= %.0f", tc.label, r.W, tc.want)
		}
	}
}
