package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The settings page's fields used to stretch to the window width, so a wide
// window showed a value in a box the size of the page. The capped controls
// carry a label, which the headless tester finds them by, so their width is
// pinned here; the labels also name the read-only fields for assistive
// technology.
func TestSettingsControlsKeepTheFieldWidth(t *testing.T) {
	appLanguage(t, "zh-Hans")
	app := &appState{}
	app.settings.logLevel = logLevelError
	app.settings.logPath = `C:\Users\ada\AppData\Local\biowarden\biowarden.log`
	tester := ui.NewTester(app.settingsPage, 1600, 900)
	tester.Frame()

	cases := []struct {
		label string
		want  float32
	}{
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

// The account page's card already shows the server address, and the settings
// page's copy bound the in-memory session, which falls back to the official
// cloud address after signing out. The account group is gone so nothing here
// can contradict the card.
func TestSettingsPageHasNoAccountGroup(t *testing.T) {
	appLanguage(t, "zh-Hans")
	app := &appState{}
	app.account.session.serverURL = "https://vault.example.com"
	tester := ui.NewTester(app.settingsPage, 1600, 900)
	tester.Frame()

	for _, text := range []string{"账号", "服务器地址", "服务器地址（只读）"} {
		if tester.HasText(text) || hasLabel(tester, text) {
			t.Errorf("settings page still shows %q", text)
		}
	}
}

// hasLabel reports whether a control carries label, which Find matches but the
// page's own texts do not.
func hasLabel(tester *ui.Tester, label string) bool {
	_, ok := tester.Find(label)
	return ok
}
