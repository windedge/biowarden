// Package session is the persisted login: what a sign-in keeps between runs,
// where the OS credential store keeps it, and how a stored refresh token is
// re-verified at startup. None of it is page state, so the GUI and a future
// host/CLI share one implementation.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNoStoredSession reports that no login is persisted: the normal state on a
// fresh install, after a logout, and after a revoked session was cleared.
var ErrNoStoredSession = errors.New("session: no stored session")

// Record is the persisted login: the only thing biowarden keeps between runs
// besides the key. It carries the refresh token so a restart can verify the
// session without the master password; it never carries the password or the
// account key.
type Record struct {
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	ServerURL    string `json:"serverURL"`
	RefreshToken string `json:"refreshToken"`
}

// String redacts the refresh token so a record cannot reach a log through %v.
func (Record) String() string { return "session.Record(<redacted>)" }

// Encode is the exact bytes a store writes, so a test can assert the master
// password and the account key never appear in them.
func Encode(r Record) ([]byte, error) { return json.Marshal(r) }

// Decode reads the bytes Encode wrote.
func Decode(b []byte) (Record, error) {
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return Record{}, fmt.Errorf("session: record corrupt: %w", err)
	}
	return r, nil
}

// Store is the persisted-login seam. Save, Load and Clear never see the master
// password or the user key, only the refresh token and the identity.
type Store interface {
	Save(Record) error
	Load() (Record, error) // ErrNoStoredSession when nothing is stored
	Clear(userID string) error
}

// NopStore is the injected default for callers that do not persist a session.
// It stores nothing.
type NopStore struct{}

func (NopStore) Save(Record) error     { return nil }
func (NopStore) Clear(string) error    { return nil }
func (NopStore) Load() (Record, error) { return Record{}, ErrNoStoredSession }
