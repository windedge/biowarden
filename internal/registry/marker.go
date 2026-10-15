package registry

// First-run marker.
//
// The marker is one value under HKCU\Software\biowarden, next to the
// registrations it describes: it answers "has this user's biowarden already
// configured itself", which is the same lifetime and the same store the rest
// of the registration state lives in. A file under %LOCALAPPDATA% would work
// too, but that would be a second place to look for the same answer.

const (
	firstRunKeyPath   = `Software\biowarden`
	firstRunValueName = "FirstRunCompleted"
)

// Marker remembers that the first-run setup ran, so every later launch stays
// quiet: no automatic registration and no login window popping up. Reads and
// writes are best-effort by contract; the caller decides how loudly to report
// a failure, because the failure must never block the login window.
type Marker interface {
	// Done reports whether an earlier launch wrote the marker. A missing key
	// or value is false, not an error.
	Done() (bool, error)
	// Mark records the completed setup. Writing it twice is not an error.
	Mark() error
}
