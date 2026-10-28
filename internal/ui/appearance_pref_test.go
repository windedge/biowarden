package ui

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// fakeAppearancePref is the seam a test drives instead of the registry.
type fakeAppearancePref struct {
	current appearance
	saved   []appearance
}

func (p *fakeAppearancePref) load() appearance  { return p.current }
func (p *fakeAppearancePref) save(a appearance) { p.saved = append(p.saved, a) }

// settingsWithAppearance is an app whose Appearance row starts at current and
// whose choice reaches a recorder. The other seams are left nil, which the
// settings state already guards.
func settingsWithAppearance(current appearance) (*appState, *fakeAppearancePref, *[]mygo.ThemeSource) {
	pref := &fakeAppearancePref{current: current}
	var sources []mygo.ThemeSource
	app := &appState{}
	app.settings.appearance = current
	app.settings.appearancePref = pref
	app.settings.setSource = func(s mygo.ThemeSource) { sources = append(sources, s) }
	app.settings.logLevel = logLevelError
	return app, pref, &sources
}

// The row names itself and its choices in the language the app shows.
func TestAppearanceRowFollowsTheLanguage(t *testing.T) {
	cases := []struct {
		name, tag, label, system, light, dark string
	}{
		{"english", "", "Appearance", "Follow the system", "Light", "Dark"},
		{"chinese", "zh-Hans", "外观", "跟随系统", "浅色", "深色"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appLanguage(t, tc.tag)
			app, _, _ := settingsWithAppearance(appearanceSystem)
			tester := ui.NewTester(app.settingsPage, 1600, 900)
			tester.Frame()

			for _, want := range []string{tc.label, tc.system, tc.light, tc.dark} {
				if _, ok := tester.Find(want); !ok {
					t.Errorf("the Appearance row does not show %q: %v", want, tester.Texts())
				}
			}
		})
	}
}

// Choosing a side shows it at once and remembers it; the choice reaches mygo's
// theme source and the persisted value, and nothing else is written.
func TestAppearanceChoiceReachesTheApp(t *testing.T) {
	appLanguage(t, "")
	app, pref, sources := settingsWithAppearance(appearanceSystem)
	tester := ui.NewTester(app.settingsPage, 1600, 900)
	tester.Frame()

	if err := tester.Click("Dark"); err != nil {
		t.Fatalf("choose Dark: %v", err)
	}
	if len(*sources) != 1 || (*sources)[0] != mygo.ThemeDark {
		t.Fatalf("the row asked mygo for %v, want [dark]", *sources)
	}
	if len(pref.saved) != 1 || pref.saved[0] != appearanceDark {
		t.Fatalf("the row persisted %v, want [dark]", pref.saved)
	}
	if app.settings.appearance != appearanceDark {
		t.Errorf("the row shows %v, want dark", app.settings.appearance)
	}
	if prefs := app.settings.appearance.stored(); prefs != "dark" {
		t.Errorf("the stored value would be %q, want dark", prefs)
	}
}

// Rendering the row must not write the appearance: a frame the user did not act
// on leaves the persisted choice and the app's theme source alone.
func TestAppearanceRowDoesNotWriteOnRender(t *testing.T) {
	appLanguage(t, "")
	app, pref, sources := settingsWithAppearance(appearanceLight)
	tester := ui.NewTester(app.settingsPage, 1600, 900)

	for i := 0; i < 5; i++ {
		tester.Frame()
	}
	if len(pref.saved) != 0 {
		t.Fatalf("rendering the page persisted %v, want no write at all", pref.saved)
	}
	if len(*sources) != 0 {
		t.Fatalf("rendering the page asked mygo for %v, want no call at all", *sources)
	}
	if app.settings.appearance != appearanceLight {
		t.Errorf("the row shows %v after rendering, want the persisted light", app.settings.appearance)
	}
}
