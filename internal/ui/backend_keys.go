package ui

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/secrets"
	"github.com/windedge/biowarden/locales"
)

// The Hello failures are typed sentinels, never user-facing text: errorText
// maps each to wording in the language the app shows, and every caller that
// puts one on screen goes through it.
var (
	errHelloCallFailed  = errors.New("ui: windows hello call failed")
	errHelloCanceled    = errors.New("ui: windows hello canceled")
	errHelloUnavailable = errors.New("ui: windows hello unavailable")
	errHelloLocked      = errors.New("ui: windows hello locked")
	errHelloBusy        = errors.New("ui: windows hello busy")
	errHelloUnknown     = errors.New("ui: windows hello unknown result")
	errKeyNotBase64     = errors.New("ui: key is not valid base64")
)

// realKeys is the Keys page against the real seams: the Credential Manager
// holds the key and Windows Hello gates every write and delete. Biometrics
// only gate access to a key kept in the OS store; they encrypt nothing.
type realKeys struct {
	store    secrets.Store
	verifier hello.Verifier
}

// newRealKeysWith injects the two seams so tests can drive consent outcomes
// without touching the real credential store or prompting a human. The account
// backend builds it with the same store box it uses, so the key card and the
// login path share one instance.
func newRealKeysWith(store secrets.Store, verifier hello.Verifier) *realKeys {
	return &realKeys{store: store, verifier: verifier}
}

func (k *realKeys) accounts() ([]accountRow, error) {
	list, err := k.store.List()
	if err != nil {
		return nil, err
	}
	rows := make([]accountRow, 0, len(list))
	for _, acc := range list {
		rows = append(rows, accountRow{
			userID:    acc.UserID,
			email:     acc.Email,
			created:   when(acc.CreatedAt),
			serverURL: acc.ServerURL,
		})
	}
	return rows, nil
}

// save stores one account's key. The base64 decode comes before the prompt so
// a malformed paste never makes the user answer a pointless Hello dialog, and
// the consent comes before Put so a refusal leaves the store untouched. The
// reference stores first and verifies after, which can leave a key behind when
// the user cancels.
func (k *realKeys) save(userID, keyB64 string) error {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keyB64))
	if err != nil {
		return newLoginError(errKeyNotBase64, err)
	}
	if err := k.verify(locales.HelloReasonSaveKey()); err != nil {
		return err
	}
	acc := secrets.Account{UserID: strings.TrimSpace(userID), CreatedAt: time.Now()}
	return k.store.Put(acc, secrets.Key(key))
}

// remove deletes one account's key behind the same consent as save: the
// reference unlocks before it touches stored key material, so deletion is not
// a silent bypass of the gate.
func (k *realKeys) remove(userID string) error {
	if err := k.verify(locales.HelloReasonRemoveKey()); err != nil {
		return err
	}
	return k.store.Remove(userID)
}

func (k *realKeys) helloAvailable() (bool, error) {
	return k.verifier.Available()
}

func (k *realKeys) verify(reason string) error {
	return verifyHello(k.verifier, reason)
}

// verifyHello turns a Hello outcome into the typed sentinel the UI words. A
// refused or unusable check must read differently from a broken call. It is
// shared by the key card and the account backend so both write gates behave
// and word their failures the same way.
func verifyHello(verifier hello.Verifier, reason string) error {
	result, err := verifier.Verify(reason)
	if err != nil {
		return newLoginError(errHelloCallFailed, err)
	}
	switch result {
	case hello.Verified:
		return nil
	case hello.Canceled:
		return errHelloCanceled
	case hello.Unavailable:
		return errHelloUnavailable
	case hello.Locked:
		return errHelloLocked
	case hello.Busy:
		return errHelloBusy
	default:
		return newLoginError(errHelloUnknown, result)
	}
}
