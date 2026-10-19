package ui

import "golang.org/x/sys/windows/registry"

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

// registryLocaleMarker persists the choice under HKCU\Software\biowarden,
// beside the registration values. It writes only to the current user's hive,
// so it never needs elevation.
type registryLocaleMarker struct{}

const (
	localeMarkerKey   = `Software\biowarden`
	localeMarkerValue = "LocaleChoice"
)

// load reads the marker. A missing key, a missing value and an unreadable
// value all mean "no choice", so a fresh machine falls back to the system.
func (registryLocaleMarker) load() (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, localeMarkerKey, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(localeMarkerValue)
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

// save writes the choice. The registry is best-effort: a failure only means
// the row falls back to the system guess on the next launch, never a crash.
func (registryLocaleMarker) save(tag string) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, localeMarkerKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.SetStringValue(localeMarkerValue, tag)
}

// clear drops the choice. A missing key or value is already the desired state.
func (registryLocaleMarker) clear() {
	k, err := registry.OpenKey(registry.CURRENT_USER, localeMarkerKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.DeleteValue(localeMarkerValue)
}
