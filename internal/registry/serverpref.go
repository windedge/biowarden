package registry

// Remembered server address.
//
// The address a login last succeeded against is a machine preference, not
// account state: the key and the persisted session that also carry an address
// are deleted when the user signs out, but a self-hosted user signs straight
// back in and must not retype it. It lives under HKCU\Software\biowarden beside
// the first-run marker for the same reason the marker does: same lifetime,
// same store, one place to look.

const (
	lastServerKeyPath   = `Software\biowarden`
	lastServerValueName = "LastServerURL"
)

// ServerPref remembers the address the last successful login used. Reads and
// writes are best-effort by contract: the value only pre-fills a form, so a
// failure must never block a login or a launch.
type ServerPref interface {
	// Last reports the remembered address, false when there is none. A missing
	// key, a missing value and a value of the wrong type all mean "none".
	Last() (string, bool)
	// Remember records address, replacing any earlier one. An empty address is
	// ignored, so a caller with nothing to say cannot blank the value.
	Remember(address string)
}
