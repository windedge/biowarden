// Package hello is the biometric verification seam: the user-presence check
// before a stored key is released. Biometrics gate access to a key held in the
// OS credential store; they encrypt nothing.
package hello

import "errors"

var ErrNotImplemented = errors.New("hello: verification not implemented yet")

// Result is the outcome of a verification attempt, as reported to the user.
type Result int

const (
	Verified    Result = iota // the user approved
	Canceled                  // the user dismissed the prompt
	Unavailable               // no Hello credential is configured for this user
	Locked                    // too many failed attempts
	Busy                      // the device could not run the check right now
)

func (r Result) String() string {
	switch r {
	case Verified:
		return "verified"
	case Canceled:
		return "canceled"
	case Unavailable:
		return "unavailable"
	case Locked:
		return "locked"
	case Busy:
		return "busy"
	}
	return "unknown"
}

// Verifier performs the user-presence check.
type Verifier interface {
	// Available reports whether a Hello credential exists, so callers can
	// explain an unusable setup instead of failing at unlock time.
	Available() (bool, error)

	// Verify shows the system prompt for reason; a non-nil error means the
	// check itself failed rather than the user refusing.
	Verify(reason string) (Result, error)
}

type stub struct{}

func (stub) Available() (bool, error)      { return false, ErrNotImplemented }
func (stub) Verify(string) (Result, error) { return Unavailable, ErrNotImplemented }
