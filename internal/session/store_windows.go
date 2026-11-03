//go:build windows

package session

import (
	"errors"

	"github.com/danieljoos/wincred"
)

// WincredStore keeps the persisted login in the Windows Credential Manager
// under the Session subtree (Biowarden/Session/<userId>), separate from the
// account keys. DPAPI encrypts it for the current Windows user, the same
// protection the key entries get.
type WincredStore struct {
	prefix string // the session prefix, e.g. secrets.SessionPrefix
}

// NewWincredStore is the real store over prefix.
func NewWincredStore(prefix string) *WincredStore {
	return &WincredStore{prefix: prefix}
}

func (s *WincredStore) target(userID string) string { return s.prefix + "/" + userID }

func (s *WincredStore) Save(rec Record) error {
	if rec.UserID == "" {
		return errors.New("session: record has no user id")
	}
	blob, err := Encode(rec)
	if err != nil {
		return err
	}
	cred := wincred.NewGenericCredential(s.target(rec.UserID))
	cred.UserName = rec.UserID
	cred.CredentialBlob = blob
	if err := cred.Write(); err != nil {
		return err
	}
	// One active login: drop any older session so a stale account cannot be
	// restored after a different account signed in.
	creds, err := wincred.FilteredList(s.prefix + "/*")
	if err != nil {
		return err
	}
	for _, old := range creds {
		if old.TargetName == s.target(rec.UserID) {
			continue
		}
		full, err := wincred.GetGenericCredential(old.TargetName)
		if err != nil {
			continue
		}
		_ = full.Delete()
	}
	return nil
}

func (s *WincredStore) Load() (Record, error) {
	creds, err := wincred.FilteredList(s.prefix + "/*")
	if err != nil {
		return Record{}, err
	}
	if len(creds) == 0 {
		return Record{}, ErrNoStoredSession
	}
	newest := creds[0]
	for _, c := range creds[1:] {
		if c.LastWritten.After(newest.LastWritten) {
			newest = c
		}
	}
	full, err := wincred.GetGenericCredential(newest.TargetName)
	if err != nil {
		return Record{}, err
	}
	return Decode(full.CredentialBlob)
}

func (s *WincredStore) Clear(userID string) error {
	if userID == "" {
		return nil
	}
	cred, err := wincred.GetGenericCredential(s.target(userID))
	if errors.Is(err, wincred.ErrElementNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return cred.Delete()
}
