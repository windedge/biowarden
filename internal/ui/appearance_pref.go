package ui

import (
	"github.com/egoist/mygo"

	"github.com/windedge/biowarden/internal/registry"
	"github.com/windedge/biowarden/locales"
)

// appearance is the Settings page's Appearance row: which appearance the app
// shows. It is an enum with a catalog label, as logLevel is, so the labels
// follow the language while the value stays one the app can store and apply.
type appearance int

const (
	appearanceSystem appearance = iota
	appearanceLight
	appearanceDark
)

// appearances are the row's choices in display order, the system's own first.
var appearances = []appearance{appearanceSystem, appearanceLight, appearanceDark}

// label names the choice in the language the app shows.
func (a appearance) label() string {
	switch a {
	case appearanceLight:
		return locales.SettingsAppearanceLight()
	case appearanceDark:
		return locales.SettingsAppearanceDark()
	default:
		return locales.SettingsAppearanceSystem()
	}
}

// source is the mygo theme source that shows a. "system" follows the desktop,
// which is the zero value mygo starts in.
func (a appearance) source() mygo.ThemeSource {
	switch a {
	case appearanceLight:
		return mygo.ThemeLight
	case appearanceDark:
		return mygo.ThemeDark
	default:
		return mygo.ThemeSystem
	}
}

// stored is the value the registry keeps, which is the word of the source.
func (a appearance) stored() registry.Appearance {
	switch a {
	case appearanceLight:
		return registry.AppearanceLight
	case appearanceDark:
		return registry.AppearanceDark
	default:
		return registry.AppearanceSystem
	}
}

// appearanceFromStored reads back a persisted choice; an unknown value is the
// system's own.
func appearanceFromStored(value registry.Appearance) appearance {
	switch value {
	case registry.AppearanceLight:
		return appearanceLight
	case registry.AppearanceDark:
		return appearanceDark
	default:
		return appearanceSystem
	}
}

// appearancePref is the seam over the persisted appearance, so the Settings
// page can read and write the choice without a test touching the registry.
type appearancePref interface {
	// load reports the persisted choice, the system's own when there is none.
	load() appearance
	// save records a as the choice.
	save(a appearance)
}

// registryAppearancePref is the production seam: the HKCU value beside the
// first-run marker.
type registryAppearancePref struct{ pref registry.AppearancePref }

func newAppearancePref() appearancePref {
	return registryAppearancePref{pref: registry.NewAppearancePref()}
}

func (p registryAppearancePref) load() appearance {
	return appearanceFromStored(p.pref.Current())
}

func (p registryAppearancePref) save(a appearance) { p.pref.Remember(a.stored()) }
