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

// appLanguageFollows ties the catalogs to the fake locale, as the real
// mygo.App does: SetLocale changes what the app shows and the catalogs say at
// once. The other tests keep the catalog language fixed, which hides a row that
// only breaks when the labels it shows change under it.
func appLanguageFollows(t *testing.T, loc *fakeLocale) {
	t.Helper()
	i18n.SetLocaleFunc(func() string { return loc.shown })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
}

// Rendering the Language row must not write the language: a frame the user did
// not act on must leave the persisted choice alone. The bug this pins was the
// row reporting a choice on a frame nobody chose one, which overwrote the saved
// language with the system's on every render.
func TestSettingsLanguageRowDoesNotWriteOnRender(t *testing.T) {
	app, loc := settingsWithLanguage("zh-Hans", "en-US")
	appLanguageFollows(t, loc)
	tester := ui.NewTester(app.settingsPage, 1600, 900)

	for i := 0; i < 5; i++ {
		tester.Frame()
	}
	if len(loc.chosen) != 0 {
		t.Fatalf("rendering the page asked mygo for %v, want no call at all", loc.chosen)
	}
	if app.settings.languageTag != "zh-Hans" {
		t.Errorf("the row shows %q after rendering, want the persisted zh-Hans", app.settings.languageTag)
	}
	if _, ok := tester.Find("简体中文"); !ok {
		t.Errorf("the trigger does not show the persisted language: %v", tester.Texts())
	}
}

// Choosing a language calls SetLocale exactly once, and choosing the system
// again calls it with the empty tag. The value the control carries is the tag,
// so a label changing with the language cannot be written back as one.
func TestSettingsLanguageSwitchWritesExactlyOnce(t *testing.T) {
	app, loc := settingsWithLanguage("en", "en-US")
	appLanguageFollows(t, loc)
	tester := ui.NewTester(app.settingsPage, 1600, 900)
	tester.Frame()

	if err := tester.Click("Follow the system"); err != nil {
		t.Fatalf("open the Language menu: %v", err)
	}
	if err := tester.Click("简体中文"); err != nil {
		t.Fatalf("choose 简体中文: %v", err)
	}
	for i := 0; i < 5; i++ {
		tester.Frame()
	}
	if len(loc.chosen) != 1 || loc.chosen[0] != "zh-Hans" {
		t.Fatalf("the page asked mygo for %v, want [zh-Hans]", loc.chosen)
	}
	if app.settings.languageTag != "zh-Hans" {
		t.Errorf("the row shows %q, want zh-Hans", app.settings.languageTag)
	}

	// The row is now Chinese: "跟随系统" is the system choice in that language.
	if err := tester.Click("简体中文"); err != nil {
		t.Fatalf("open the Language menu again: %v", err)
	}
	if err := tester.Click("跟随系统"); err != nil {
		t.Fatalf("choose 跟随系统: %v", err)
	}
	if len(loc.chosen) != 2 || loc.chosen[1] != "" {
		t.Fatalf("the page asked mygo for %v, want [zh-Hans ]", loc.chosen)
	}
	if app.settings.languageTag != "" {
		t.Errorf("the row shows %q, want the system choice", app.settings.languageTag)
	}
}
