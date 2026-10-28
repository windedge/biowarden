package registry

// Appearance preference.
//
// The appearance the app shows is a machine preference, not account state: it
// lives under HKCU\Software\biowarden beside the first-run marker and the
// remembered server, for the same reason they do: same lifetime, same store,
// one place to look.

// Appearance is the appearance the app shows: the system's own, or one side
// forced.
type Appearance string

const (
	AppearanceSystem Appearance = "system"
	AppearanceLight  Appearance = "light"
	AppearanceDark   Appearance = "dark"
)

// ParseAppearance reads a stored value. Anything but light or dark is the
// system's own, so a hand-edited or half-written value cannot leave the app in
// an appearance it cannot name.
func ParseAppearance(value string) Appearance {
	switch Appearance(value) {
	case AppearanceLight, AppearanceDark:
		return Appearance(value)
	}
	return AppearanceSystem
}

// AppearancePref remembers the appearance the Settings page chose. Reads and
// writes are best-effort by contract: the choice only changes how the app
// looks, so failing to store it must never block a launch or a click.
type AppearancePref interface {
	// Current reports the stored choice, the system's own when there is none.
	Current() Appearance
	// Remember records a, replacing any earlier choice.
	Remember(a Appearance)
}
