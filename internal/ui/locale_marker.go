package ui

import "github.com/windedge/biowarden/internal/registry"

// localeMarker is the seam over the persisted language choice. The Settings
// page needs to tell an explicit choice apart from "follow the system": mygo
// keeps the chosen tag for the next launch but reports only the language it
// shows, which is the same value the system's own languages can produce. The
// marker records that a human chose, so the row can say so. It is an interface
// so tests never touch the registry.
type localeMarker interface {
	// load reports the persisted choice, false when there is none.
	load() (string, bool)
	// save records tag as an explicit choice.
	save(tag string)
	// clear drops the choice, so the app follows the system again.
	clear()
}

// registryLocaleMarker is the production seam: the HKCU value beside the
// first-run marker, the appearance and the remembered server.
type registryLocaleMarker struct{ pref registry.LocalePref }

func newLocaleMarker() localeMarker { return registryLocaleMarker{pref: registry.NewLocalePref()} }

func (m registryLocaleMarker) load() (string, bool) { return m.pref.Chosen() }

func (m registryLocaleMarker) save(tag string) { m.pref.Remember(tag) }

func (m registryLocaleMarker) clear() { m.pref.Forget() }
