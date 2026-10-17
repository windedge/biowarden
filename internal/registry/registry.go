// Package registry is the manifest registration seam: it points a browser at
// biowarden's native messaging manifest, per user under HKCU. An existing value
// may be the official Bitwarden desktop client's, so Status reports what is
// there and Install only runs once the caller decides to replace it.
package registry

import "errors"

var ErrNotImplemented = errors.New("registry: not implemented yet")

// Browser identifies a browser family with its own registration point.
type Browser string

const (
	Chrome  Browser = "chrome"
	Edge    Browser = "edge"
	Firefox Browser = "firefox"
)

// Browsers lists the supported browsers in display order.
var Browsers = []Browser{Chrome, Edge, Firefox}

func (b Browser) String() string {
	switch b {
	case Chrome:
		return "Chrome"
	case Edge:
		return "Edge"
	case Firefox:
		return "Firefox"
	}
	return string(b)
}

// Status is a browser's current registration state.
type Status struct {
	Registered bool   // a manifest path is set for this browser
	Path       string // the manifest the browser points at, empty when none
	// IsOurs reports whether Path is the manifest biowarden writes. Set but
	// not ours means it may be the official client's, which the GUI shows
	// before offering to replace it.
	IsOurs bool
	// OursInstalled reports whether our manifest file is still on disk. It is
	// the evidence that biowarden was installed for this browser even after a
	// foreign client overwrote the registration, which is how a hijack is told
	// apart from a browser we never touched.
	OursInstalled bool
}

// Registrar registers and unregisters biowarden's native messaging host.
type Registrar interface {
	Status(b Browser) (Status, error)
	// ManifestPath is where biowarden's manifest for b lives, whether or not it
	// is registered yet. The GUI shows it as the overwrite target before a
	// human confirms replacing a registration that belongs to another client.
	ManifestPath(b Browser) (string, error)
	// Install writes biowarden's manifest and points the browser at it,
	// replacing whatever was registered before.
	Install(b Browser) error
	// Uninstall removes biowarden's registration and manifest, leaving a
	// registration that is not ours alone.
	Uninstall(b Browser) error
}
