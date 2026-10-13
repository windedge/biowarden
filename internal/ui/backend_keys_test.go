package ui

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/secrets"
)

// fakeStore is an in-memory secrets.Store, so these tests never touch the
// real Credential Manager.
type fakeStore struct {
	accounts    map[string]secrets.Account
	keys        map[string][]byte
	putCalls    int
	removeCalls int
}

func newFakeStore() *fakeStore {
	return &fakeStore{accounts: map[string]secrets.Account{}, keys: map[string][]byte{}}
}

func (s *fakeStore) List() ([]secrets.Account, error) {
	out := make([]secrets.Account, 0, len(s.accounts))
	for _, a := range s.accounts {
		out = append(out, a)
	}
	return out, nil
}

func (s *fakeStore) Get(userID string) (secrets.Key, error) {
	k, ok := s.keys[userID]
	if !ok {
		return nil, secrets.ErrNotFound
	}
	return secrets.Key(append([]byte(nil), k...)), nil
}

func (s *fakeStore) Put(acc secrets.Account, key secrets.Key) error {
	s.putCalls++
	s.accounts[acc.UserID] = acc
	s.keys[acc.UserID] = append([]byte(nil), key...)
	return nil
}

func (s *fakeStore) Remove(userID string) error {
	s.removeCalls++
	delete(s.accounts, userID)
	delete(s.keys, userID)
	return nil
}

// fakeVerifier answers with a fixed result, recording whether it was asked.
type fakeVerifier struct {
	available  bool
	result     hello.Result
	err        error
	calls      int
	lastReason string
}

func (v *fakeVerifier) Available() (bool, error) { return v.available, v.err }

func (v *fakeVerifier) Verify(reason string) (hello.Result, error) {
	v.calls++
	v.lastReason = reason
	return v.result, v.err
}

func TestSaveVerifiedWritesDecodedKey(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	k := newRealKeysWith(store, verifier)

	key := []byte{0x00, 0x01, 0xff, 0x10}
	if err := k.save("user-1", base64.StdEncoding.EncodeToString(key)); err != nil {
		t.Fatalf("save: %v", err)
	}
	if store.putCalls != 1 {
		t.Fatalf("Put calls = %d, want 1", store.putCalls)
	}
	if got := store.keys["user-1"]; !bytes.Equal(got, key) {
		t.Fatalf("stored key = %x, want %x", got, key)
	}
	if store.accounts["user-1"].UserID != "user-1" {
		t.Fatalf("stored account = %+v, want UserID user-1", store.accounts["user-1"])
	}
	if verifier.calls != 1 || verifier.lastReason == "" {
		t.Fatalf("Verify calls = %d, reason = %q", verifier.calls, verifier.lastReason)
	}
}

func TestSaveRefusedLeavesStoreUntouched(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Canceled}
	k := newRealKeysWith(store, verifier)

	err := k.save("user-1", base64.StdEncoding.EncodeToString([]byte("secret")))
	if err == nil {
		t.Fatal("save with canceled consent = nil, want error")
	}
	if store.putCalls != 0 {
		t.Fatalf("Put calls = %d, want 0", store.putCalls)
	}
	if len(store.keys) != 0 {
		t.Fatalf("store has %d keys, want 0", len(store.keys))
	}
}

// A malformed paste must not make the user answer a Hello dialog, so decoding
// has to happen before Verify.
func TestSaveRejectsBadBase64BeforeVerify(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	k := newRealKeysWith(store, verifier)

	if err := k.save("user-1", "not-base64!!"); err == nil {
		t.Fatal("save with bad base64 = nil, want error")
	}
	if verifier.calls != 0 {
		t.Fatalf("Verify calls = %d, want 0", verifier.calls)
	}
	if store.putCalls != 0 {
		t.Fatalf("Put calls = %d, want 0", store.putCalls)
	}
}

func TestRemoveRefusedLeavesStoreUntouched(t *testing.T) {
	store := newFakeStore()
	store.Put(secrets.Account{UserID: "user-1"}, secrets.Key("secret"))
	verifier := &fakeVerifier{result: hello.Locked}
	k := newRealKeysWith(store, verifier)

	if err := k.remove("user-1"); err == nil {
		t.Fatal("remove with locked consent = nil, want error")
	}
	if store.removeCalls != 0 {
		t.Fatalf("Remove calls = %d, want 0", store.removeCalls)
	}
	if _, ok := store.keys["user-1"]; !ok {
		t.Fatal("Remove ran despite refused consent")
	}
}

func TestRemoveVerifiedDeletes(t *testing.T) {
	store := newFakeStore()
	store.Put(secrets.Account{UserID: "user-1"}, secrets.Key("secret"))
	verifier := &fakeVerifier{result: hello.Verified}
	k := newRealKeysWith(store, verifier)

	if err := k.remove("user-1"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if store.removeCalls != 1 {
		t.Fatalf("Remove calls = %d, want 1", store.removeCalls)
	}
	if _, ok := store.keys["user-1"]; ok {
		t.Fatal("key survived a verified remove")
	}
}

func TestHelloAvailablePassthrough(t *testing.T) {
	k := newRealKeysWith(newFakeStore(), &fakeVerifier{available: false})
	available, err := k.helloAvailable()
	if err != nil {
		t.Fatalf("helloAvailable: %v", err)
	}
	if available {
		t.Fatal("helloAvailable = true, want the verifier's false")
	}
}
