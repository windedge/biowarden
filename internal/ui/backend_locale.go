package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
)

// localeBackend is what the Settings page's Language row needs from the app's
// language. mygo owns the choice and keeps it for the next launch (a file in
// the user data directory on Windows), so nothing is stored here.
type localeBackend interface {
	// locale is the language the app shows, as a tag of its catalogs.
	locale() string
	// languages are the languages the app is written in, its own first.
	languages() []string
	// preferredLanguages are the languages the user reads, most preferred
	// first, which the app follows while no language is chosen.
	preferredLanguages() []string
	// setLocale shows tag at once and keeps it; "" follows the user's
	// languages again.
	setLocale(tag string)
}

// mygoLocale is the production localeBackend.
type mygoLocale struct{}

func (mygoLocale) locale() string               { return mygo.App.Locale() }
func (mygoLocale) languages() []string          { return mygo.App.Languages() }
func (mygoLocale) preferredLanguages() []string { return mygo.App.PreferredLanguages() }
func (mygoLocale) setLocale(tag string)         { mygo.App.SetLocale(tag) }

// systemLanguage returns the catalog language the user's languages ask for:
// the one the app shows while it follows them.
func systemLanguage(l localeBackend) string {
	langs := l.languages()
	if len(langs) == 0 || len(l.preferredLanguages()) == 0 {
		return ""
	}
	return i18n.Match(l.preferredLanguages(), langs)
}
