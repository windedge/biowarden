package ui

import "errors"

// errFakeBackend marks every prototype backend: the pages stay clickable,
// nothing touches the registry, the credential store, or Windows Hello.
var errFakeBackend = errors.New("ui: prototype backend, not wired yet")

// fakeBrowsers stands where the registry seam will stand. The fake rows
// include the foreign case this machine really has: the official client's
// registration.
type fakeBrowsers struct{}

func (fakeBrowsers) refresh() ([]browserRow, error) {
	return []browserRow{
		{name: "Chrome", path: `%LOCALAPPDATA%\biowarden\manifests\chrome.json`, state: browserOurs, installed: true},
		{name: "Microsoft Edge", path: `C:\Users\<user>\scoop\apps\bitwarden\...\browsers\edge.json`, state: browserForeign, installed: true},
		{name: "Firefox", path: "", state: browserMissing},
	}, nil
}

func (fakeBrowsers) install(browserRow) error { return errFakeBackend }

func (fakeBrowsers) uninstall(browserRow) error { return errFakeBackend }

// fakeKeys stands where the secrets and hello seams will stand.
type fakeKeys struct{}

func (fakeKeys) accounts() ([]accountRow, error) {
	return []accountRow{
		{userID: "6f1c9a04d2e84b7f9c3a1b8e5d0f2a7c", email: "ada@example.com", created: "2026-01-12 09:24"},
		{userID: "b27ae6f80c1d4a53b6e9d7c240f8a1b5", email: "grace@example.com", created: "2026-02-03 18:07"},
	}, nil
}

func (fakeKeys) save(string, string) error     { return errFakeBackend }
func (fakeKeys) remove(string) error           { return errFakeBackend }
func (fakeKeys) helloAvailable() (bool, error) { return true, nil }
func (fakeKeys) verify(reason string) error    { return errFakeBackend }

// fakeSettings stands where the autostart and log file will stand.
type fakeSettings struct{}

func (fakeSettings) autostart() (bool, error)      { return true, nil }
func (fakeSettings) setAutostart(bool) error       { return errFakeBackend }
func (fakeSettings) logPath() string               { return `%LOCALAPPDATA%\biowarden\host.log` }
func (fakeSettings) logTail(int64) (string, error) { return fakeLog, nil }

// fakeLocale stands where mygo.App's language stands, and records the choice.
type fakeLocale struct {
	shown     string
	langs     []string
	preferred []string
	chosen    []string
}

func (f *fakeLocale) locale() string { return f.shown }

func (f *fakeLocale) languages() []string { return f.langs }

func (f *fakeLocale) preferredLanguages() []string { return f.preferred }

func (f *fakeLocale) setLocale(tag string) {
	f.chosen = append(f.chosen, tag)
	f.shown = tag
	if f.shown == "" {
		f.shown = systemLanguage(f)
	}
}
