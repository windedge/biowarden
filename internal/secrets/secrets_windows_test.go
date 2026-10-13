package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/danieljoos/wincred"
)

// Credential Manager is a real, account-wide store shared with the user's other
// software, so every test runs under its own BiowardenTest/<random> namespace
// and deletes everything it created.
func newTestStore(t *testing.T) *windowsStore {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	prefix := "BiowardenTest/" + hex.EncodeToString(b[:])
	t.Cleanup(func() {
		creds, err := wincred.FilteredList(prefix + "/*")
		if err != nil {
			t.Logf("cleanup list %s: %v", prefix, err)
			return
		}
		for _, cred := range creds {
			full, err := wincred.GetGenericCredential(cred.TargetName)
			if err != nil {
				t.Logf("cleanup read %s: %v", cred.TargetName, err)
				continue
			}
			if err := full.Delete(); err != nil {
				t.Logf("cleanup delete %s: %v", cred.TargetName, err)
			}
		}
	})
	return newWithPrefix(prefix)
}

func sampleAccount(userID string) Account {
	return Account{
		UserID:    userID,
		Email:     "user@example.com",
		Name:      "Test User",
		CreatedAt: time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestGetMissing(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get("nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}
}

func TestRoundTrip(t *testing.T) {
	s := newTestStore(t)
	acc := sampleAccount("user-1")
	key := Key("secret-key-bytes")
	if err := s.Put(acc, key); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get("user-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(key) {
		t.Fatalf("Get = %q, want %q", got, key)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1 (%+v)", len(list), list)
	}
	if list[0].UserID != acc.UserID || list[0].Email != acc.Email ||
		list[0].Name != acc.Name || !list[0].CreatedAt.Equal(acc.CreatedAt) {
		t.Fatalf("List = %+v, want %+v", list[0], acc)
	}
}

func TestOverwrite(t *testing.T) {
	s := newTestStore(t)
	if err := s.Put(sampleAccount("user-1"), Key("first")); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	if err := s.Put(sampleAccount("user-1"), Key("second")); err != nil {
		t.Fatalf("second Put: %v", err)
	}

	got, err := s.Get("user-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("Get = %q, want second", got)
	}
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}
}

func TestRemoveMissing(t *testing.T) {
	s := newTestStore(t)
	if err := s.Remove("nobody"); err != nil {
		t.Fatalf("Remove missing = %v, want nil", err)
	}
}

func TestRemove(t *testing.T) {
	s := newTestStore(t)
	if err := s.Put(sampleAccount("user-1"), Key("key")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Remove("user-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := s.Get("user-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Remove = %v, want ErrNotFound", err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List after Remove = %+v, want empty", list)
	}
}

// A missing index must be rebuilt from the account credentials, keeping the
// key and the recoverable metadata usable.
func TestIndexSelfHealMissing(t *testing.T) {
	s := newTestStore(t)
	acc := sampleAccount("user-1")
	if err := s.Put(acc, Key("key")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	index, err := wincred.GetGenericCredential(s.indexTarget())
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if err := index.Delete(); err != nil {
		t.Fatalf("delete index: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].UserID != acc.UserID || list[0].Email != acc.Email {
		t.Fatalf("rebuilt List = %+v, want %+v", list, acc)
	}
	if _, err := wincred.GetGenericCredential(s.indexTarget()); err != nil {
		t.Fatalf("index was not rewritten: %v", err)
	}
	got, err := s.Get("user-1")
	if err != nil || string(got) != "key" {
		t.Fatalf("Get after heal = %q, %v", got, err)
	}
}

// A corrupt index must be treated as missing rather than returned as an error.
func TestIndexSelfHealCorrupt(t *testing.T) {
	s := newTestStore(t)
	if err := s.Put(sampleAccount("user-1"), Key("key")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	corrupt := wincred.NewGenericCredential(s.indexTarget())
	corrupt.CredentialBlob = []byte("not json")
	if err := corrupt.Write(); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List with corrupt index: %v", err)
	}
	if len(list) != 1 || list[0].UserID != "user-1" {
		t.Fatalf("rebuilt List = %+v, want user-1", list)
	}
}
