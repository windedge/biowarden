package ui

import (
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// appLanguage sets the language the catalogs say, which mygo.App sets from its
// own language in the app; a Tester's SetLocale only reaches ui.Context.
func appLanguage(t *testing.T, tag string) {
	t.Helper()
	if tag == "" {
		i18n.SetLocaleFunc(nil)
	} else {
		i18n.SetLocaleFunc(func() string { return tag })
	}
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
}

// settingsWithLanguage is a settings page whose Language row shows tag, as the
// app does with the system's languages: "" follows them, and the page's choice
// starts out as the one that shows tag.
func settingsWithLanguage(shown string, preferred ...string) (*appState, *fakeLocale) {
	loc := &fakeLocale{shown: shown, langs: []string{"en", "zh-Hans"}, preferred: preferred}
	app := &appState{}
	app.settings.locale = loc
	app.settings.languageTag = app.settings.guessLanguageTag()
	return app, loc
}

// The Language row names itself and its choices in the language the app shows,
// and names each language in itself, so a reader finds their own in a menu that
// is otherwise in a language they may not read.
func TestSettingsLanguageRowFollowsTheLanguage(t *testing.T) {
	cases := []struct {
		name, tag, label, system string
	}{
		{"english", "", "Language", "Follow the system"},
		{"chinese", "zh-Hans", "语言", "跟随系统"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appLanguage(t, tc.tag)
			app, _ := settingsWithLanguage("en", "en")
			tester := ui.NewTester(app.settingsPage, 1600, 900)
			tester.Frame()

			if _, ok := tester.Find(tc.label); !ok {
				t.Errorf("the Language row is not labeled %q: %v", tc.label, tester.Texts())
			}
			if _, ok := tester.Find(tc.system); !ok {
				t.Errorf("the Language row does not offer %q: %v", tc.system, tester.Texts())
			}
		})
	}
}

// Choosing a language asks mygo to show it, which keeps it for the next
// launches; "Follow the system" is the empty tag. The page stores nothing, so
// the choice is the only thing to check.
func TestSettingsLanguageChoiceReachesTheApp(t *testing.T) {
	appLanguage(t, "")
	app, loc := settingsWithLanguage("en", "en")
	tester := ui.NewTester(app.settingsPage, 1600, 900)
	tester.Frame()

	// The trigger shows the choice; clicking it opens the menu of languages.
	if err := tester.Click("Follow the system"); err != nil {
		t.Fatalf("open the Language menu: %v", err)
	}
	if err := tester.Click("简体中文"); err != nil {
		t.Fatalf("choose 简体中文: %v", err)
	}
	if len(loc.chosen) != 1 || loc.chosen[0] != "zh-Hans" {
		t.Fatalf("the page asked mygo for %v, want [zh-Hans]", loc.chosen)
	}
	if app.settings.languageTag != "zh-Hans" {
		t.Errorf("the row shows %q, want zh-Hans", app.settings.languageTag)
	}
}
