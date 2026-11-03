package registry

// Explicit language choice.
//
// The language a human picked is a machine preference, not account state: it
// lives under HKCU\Software\biowarden beside the first-run marker, the
// appearance and the remembered server, for the same reason they do: same
// lifetime, same store, one place to look. It exists because mygo keeps the
// chosen tag for the next launch but reports only the language it shows, which
// is a value the system's own languages can produce too; the marker is what
// tells an explicit choice apart from following the system.

const (
	localeChoiceKeyPath   = `Software\biowarden`
	localeChoiceValueName = "LocaleChoice"
)

// LocalePref remembers the language a human chose explicitly. Reads and writes
// are best-effort by contract: the value only decides whether the Settings rows
// say "chosen" or "follow the system", so failing to store it must never block
// a click or a launch.
type LocalePref interface {
	// Chosen reports the stored tag, false when there is none. A missing key, a
	// missing value and a value of the wrong type all mean "none".
	Chosen() (string, bool)
	// Remember records tag as an explicit choice, replacing any earlier one.
	// An empty tag is ignored, so a caller with nothing to say cannot blank it.
	Remember(tag string)
	// Forget drops the choice, so the app follows the system's languages again.
	// A missing key or value is already the desired state.
	Forget()
}
