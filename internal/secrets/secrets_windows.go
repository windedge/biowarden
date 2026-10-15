package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/danieljoos/wincred"
)

// Credential namespace. Each account is one generic credential named
// <prefix>/<userId> whose blob is the raw key, plus <prefix>/Index whose blob
// is the JSON account list. Persisted logins live under <prefix>/Session/* and
// are never part of the index.
//
// Credential Manager caps Comment at CRED_MAX_STRING_LENGTH (256); a longer
// value makes CredWrite fail. Metadata that does not fit is dropped and the
// index rebuild falls back to the target name.
const maxCommentLen = 256

type windowsStore struct {
	prefix string
}

// New returns the Credential Manager backed store: Biowarden/<userId> per
// account plus Biowarden/Index for metadata.
func New() Store { return &windowsStore{prefix: Prefix} }

// newWithPrefix is New under an isolated namespace. Tests use it so their
// credentials cannot collide with or be listed as a real installation's.
func newWithPrefix(prefix string) *windowsStore { return &windowsStore{prefix: prefix} }

func (s *windowsStore) accountTarget(userID string) string { return s.prefix + "/" + userID }
func (s *windowsStore) indexTarget() string                { return s.prefix + "/Index" }

// isKeyTarget reports whether a credential in this namespace belongs to the
// key index. The index entry itself and the persisted-login subtree under
// <prefix>/Session/ are not accounts, so they never show up in the GUI list.
func (s *windowsStore) isKeyTarget(target string) bool {
	if target == s.indexTarget() {
		return false
	}
	return !strings.HasPrefix(target, s.prefix+"/"+SessionSubtree+"/")
}

func (s *windowsStore) List() ([]Account, error) {
	accounts, err := s.readIndex()
	if err == nil {
		return accounts, nil
	}
	// A missing or corrupt index must not block the GUI: the per-account
	// credentials are the source of truth, so rebuild the index from them.
	return s.rebuildIndex()
}

func (s *windowsStore) Get(userID string) (Key, error) {
	if userID == "" {
		return nil, ErrNotFound
	}
	cred, err := wincred.GetGenericCredential(s.accountTarget(userID))
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return Key(append([]byte(nil), cred.CredentialBlob...)), nil
}

func (s *windowsStore) Put(acc Account, key Key) error {
	if acc.UserID == "" {
		return errors.New("secrets: account has no user id")
	}
	cred := wincred.NewGenericCredential(s.accountTarget(acc.UserID))
	cred.UserName = acc.UserID
	cred.CredentialBlob = []byte(key)
	// Keep a copy of the metadata next to the key so a lost index can be
	// rebuilt without enumerating the vault for metadata it cannot provide.
	if meta, err := json.Marshal(acc); err == nil && len(meta) <= maxCommentLen {
		cred.Comment = string(meta)
	}
	if err := cred.Write(); err != nil {
		return err
	}
	// Rebuild rather than append: the account count is small and this also
	// repairs any out-of-band change to the vault.
	_, err := s.rebuildIndex()
	return err
}

func (s *windowsStore) Remove(userID string) error {
	cred, err := wincred.GetGenericCredential(s.accountTarget(userID))
	switch {
	case errors.Is(err, wincred.ErrElementNotFound):
		// Removing a missing account is not an error, but the index may still
		// hold a stale entry, so fall through to the rebuild.
	case err != nil:
		return err
	default:
		if err := cred.Delete(); err != nil {
			return err
		}
	}
	_, err = s.rebuildIndex()
	return err
}

func (s *windowsStore) readIndex() ([]Account, error) {
	cred, err := wincred.GetGenericCredential(s.indexTarget())
	if err != nil {
		return nil, err
	}
	var accounts []Account
	if err := json.Unmarshal(cred.CredentialBlob, &accounts); err != nil {
		return nil, fmt.Errorf("secrets: index corrupt: %w", err)
	}
	return accounts, nil
}

func (s *windowsStore) rebuildIndex() ([]Account, error) {
	creds, err := wincred.FilteredList(s.prefix + "/*")
	if err != nil {
		return nil, err
	}
	accounts := make([]Account, 0, len(creds))
	for _, cred := range creds {
		if !s.isKeyTarget(cred.TargetName) {
			continue
		}
		accounts = append(accounts, accountFromCredential(cred, s.prefix))
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].UserID < accounts[j].UserID })
	if err := s.writeIndex(accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

// accountFromCredential recovers an Account from a stored credential. The
// target name is authoritative for the user id; the Comment copy restores the
// display metadata when present.
func accountFromCredential(cred *wincred.Credential, prefix string) Account {
	acc := Account{UserID: strings.TrimPrefix(cred.TargetName, prefix+"/")}
	var meta Account
	if json.Unmarshal([]byte(cred.Comment), &meta) == nil {
		acc.Email = meta.Email
		acc.Name = meta.Name
		acc.ServerURL = meta.ServerURL
		acc.CreatedAt = meta.CreatedAt
	}
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = cred.LastWritten
	}
	return acc
}

func (s *windowsStore) writeIndex(accounts []Account) error {
	if accounts == nil {
		accounts = []Account{}
	}
	data, err := json.Marshal(accounts)
	if err != nil {
		return err
	}
	cred := wincred.NewGenericCredential(s.indexTarget())
	cred.UserName = s.prefix
	cred.CredentialBlob = data
	return cred.Write()
}
