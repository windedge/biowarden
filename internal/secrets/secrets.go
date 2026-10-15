// Package secrets is the key storage seam: where an account's encryption key is
// kept between unlocks. Keys never live in plain files.
package secrets

import (
	"errors"
	"time"
)

// Prefix is the Credential Manager namespace biowarden owns.
const Prefix = "Biowarden"

// SessionSubtree is the namespace below Prefix that holds persisted logins. It
// is a sibling of the account keys, and the key index never lists it as an
// account.
const SessionSubtree = "Session"

// SessionPrefix is the full Credential Manager prefix for persisted logins,
// e.g. Biowarden/Session/<userId>.
const SessionPrefix = Prefix + "/" + SessionSubtree

var (
	ErrNotImplemented = errors.New("secrets: store not implemented yet")
	ErrNotFound       = errors.New("secrets: account not found")
)

// Key is an account's encryption key. String is redacted so the key cannot
// reach a log through %v.
type Key []byte

func (Key) String() string { return "secrets.Key(<redacted>)" }

// Account is the metadata kept for a stored key. The key itself is not part of
// it, so an Account is safe to log or show in the GUI.
type Account struct {
	UserID    string
	Email     string
	Name      string
	CreatedAt time.Time
}

// Store keeps account keys in the OS credential store.
type Store interface {
	List() ([]Account, error)
	Get(userID string) (Key, error) // ErrNotFound when nothing is stored
	Put(acc Account, key Key) error
	Remove(userID string) error // removing a missing account is not an error
}
