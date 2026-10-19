package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// fakeMarker is the locale-marker seam without a registry: it holds the choice
// in memory so the explicit-vs-system guess can be driven directly.
type fakeMarker struct {
	tag string
	has bool
}

func (m *fakeMarker) load() (string, bool) { return m.tag, m.has }

func (m *fakeMarker) save(tag string) { m.tag, m.has = tag, true }

func (m *fakeMarker) clear() { m.tag, m.has = "", false }

// A persisted choice wins over the heuristic, and clearing the marker puts the
// page back on the system's languages.
func TestGuessLanguageTagPrefersTheExplicitChoice(t *testing.T) {
	marker := &fakeMarker{}
	s := &settingsState{
		locale: &fakeLocale{shown: "en", langs: []string{"en", "zh-Hans"}, preferred: []string{"fr"}},
		marker: marker,
	}

	// No marker: the app shows a language the system did not ask for, which is
	// the explicit-choice heuristic.
	if got := s.guessLanguageTag(); got != "en" {
		t.Fatalf("without a marker guessLanguageTag = %q, want en", got)
	}

	// A marker with a different tag wins, because mygo's shown language cannot
	// tell an explicit choice apart from the system's own.
	marker.save("zh-Hans")
	if got := s.guessLanguageTag(); got != "zh-Hans" {
		t.Fatalf("with a marker guessLanguageTag = %q, want zh-Hans", got)
	}

	// Clearing the marker goes back to following the system, here "" because
	// the shown language equals the one the preferences ask for.
	marker.clear()
	s.locale = &fakeLocale{shown: "en", langs: []string{"en", "zh-Hans"}, preferred: []string{"en-US"}}
	if got := s.guessLanguageTag(); got != "" {
		t.Fatalf("after clearing the marker guessLanguageTag = %q, want the system choice", got)
	}
}

// Choosing a language records the marker; choosing the system drops it.
func TestSetLanguageRecordsTheMarker(t *testing.T) {
	marker := &fakeMarker{}
	s := &settingsState{
		locale: &fakeLocale{langs: []string{"en", "zh-Hans"}},
		marker: marker,
	}

	s.setLanguage("zh-Hans")
	if !marker.has || marker.tag != "zh-Hans" {
		t.Fatalf("marker = %+v, want the explicit zh-Hans choice", marker)
	}

	s.setLanguage("")
	if marker.has {
		t.Fatalf("marker = %+v, want it cleared for the system choice", marker)
	}
}

// The log level is an enum with a catalog label, so choosing "Debug" sets the
// value rather than writing a translated label back into the field.
func TestLogLevelSelectChoosesByEnum(t *testing.T) {
	appLanguage(t, "en")
	app := &appState{}
	app.settings.logLevel = logLevelInfo
	tester := ui.NewTester(app.settingsPage, 1600, 900)
	tester.Frame()

	if !tester.HasText(locales.SettingsLogLevelInfo()) {
		t.Fatalf("the trigger does not show the current level: %v", tester.Texts())
	}
	if err := tester.Click(locales.SettingsLogLevelInfo()); err != nil {
		t.Fatalf("open the log level menu: %v", err)
	}
	if err := tester.Click(locales.SettingsLogLevelDebug()); err != nil {
		t.Fatalf("choose Debug: %v", err)
	}
	if app.settings.logLevel != logLevelDebug {
		t.Fatalf("logLevel = %v, want logLevelDebug", app.settings.logLevel)
	}
	if !tester.HasText(locales.SettingsLogLevelDebug()) {
		t.Errorf("the trigger does not show Debug after choosing it: %v", tester.Texts())
	}
}

// A page notice keeps its kind, so its wording follows the language at draw
// time instead of freezing the language that was active when it was set.
func TestPageNoticeFollowsTheLanguage(t *testing.T) {
	notices := []pageNotice{
		{kind: noticeAccountKeySaved, ok: true},
		{kind: noticeAccountVerified, args: []string{"user-1"}, ok: true},
		{kind: noticeSettingsHelloUnavailable},
		{kind: noticeLogRefreshed, ok: true},
		{kind: noticeBrowsersManifestWritten, args: []string{"Firefox"}, ok: true},
		{kind: noticeBrowsersBulk, bulk: &bulkSummary{installed: 2}},
	}

	appLanguage(t, "en")
	english := make([]string, len(notices))
	for i, n := range notices {
		english[i] = n.text()
		if english[i] == "" {
			t.Fatalf("notice %d says nothing in English", i)
		}
	}
	appLanguage(t, "zh-Hans")
	for i, n := range notices {
		if got := n.text(); got == "" || got == english[i] {
			t.Errorf("notice %d = %q, want Chinese, not the English %q", i, got, english[i])
		}
	}

	// A failure says the typed error's wording in the current language too.
	failure := failureNotice(errWrongCredentials)
	appLanguage(t, "en")
	if got := failure.text(); got != locales.LoginErrorBadCredentials() {
		t.Errorf("en failure = %q, want the English wording", got)
	}
	appLanguage(t, "zh-Hans")
	if got := failure.text(); got != locales.LoginErrorBadCredentials() {
		t.Errorf("zh failure = %q, want the Chinese wording", got)
	}
}

// The four main pages draw their heading from the catalog in both languages.
func TestMainPagesRenderInBothLanguages(t *testing.T) {
	for _, tag := range []string{"en", "zh-Hans"} {
		t.Run(tag, func(t *testing.T) {
			appLanguage(t, tag)
			app := &appState{browsers: newBrowsersStateWith(&recordingBrowsersBackend{})}
			app.settings.logPath = `C:\logs\host.log`

			cases := []struct {
				name    string
				heading string
				view    func(*ui.Context)
			}{
				{"account", locales.AccountTitle(), app.accountPage},
				{"browsers", locales.BrowsersTitle(), app.browsersPage},
				{"settings", locales.SettingsTitle(), app.settingsPage},
				{"log", locales.LogTitle(), app.logPage},
			}
			for _, tc := range cases {
				tester := ui.NewTester(tc.view, 1200, 800)
				tester.Frame()
				if !tester.HasText(tc.heading) {
					t.Errorf("%s page does not show its heading %q: %v", tc.name, tc.heading, tester.Texts())
				}
			}
		})
	}
}
