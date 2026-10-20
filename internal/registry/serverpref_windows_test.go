package registry

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The value lives in the injected root, which newTestRoot backs with a private
// app hive: the real HKCU never sees a server address from a test.
func TestServerPrefRoundTrip(t *testing.T) {
	root := newTestRoot(t)
	pref := NewServerPrefWithRoot(root)

	if address, ok := pref.Last(); ok {
		t.Fatalf("Last on a fresh root = %q, true; want no address", address)
	}

	pref.Remember("https://vault.example.com")
	address, ok := pref.Last()
	if !ok || address != "https://vault.example.com" {
		t.Fatalf("Last after Remember = %q, %v; want the remembered address", address, ok)
	}

	// A later login to another server replaces the value; the store keeps one
	// address, never a history.
	pref.Remember("https://vault.example.com")
	if address, ok = pref.Last(); !ok || address != "https://vault.example.com" {
		t.Fatalf("Last after a second Remember = %q, %v; want the newer address", address, ok)
	}

	// An empty address must not blank what is there: the caller then has
	// nothing to remember, not "forget it".
	pref.Remember("")
	if address, ok = pref.Last(); !ok || address != "https://vault.example.com" {
		t.Fatalf("Last after Remember(\"\") = %q, %v; want the kept address", address, ok)
	}
}

// An empty string value is not an address: a half-written value must read as
// "none" instead of pre-filling an empty form field.
func TestServerPrefEmptyValueIsNone(t *testing.T) {
	root := newTestRoot(t)
	key, _, err := registry.CreateKey(root, lastServerKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := key.SetStringValue(lastServerValueName, ""); err != nil {
		key.Close()
		t.Fatalf("write empty value: %v", err)
	}
	key.Close()

	address, ok := NewServerPrefWithRoot(root).Last()
	if ok || address != "" {
		t.Fatalf("Last = %q, %v; want no address", address, ok)
	}
}

// A value of the wrong type reads as "none" instead of failing the read: the
// next successful login overwrites it.
func TestServerPrefWrongValueTypeIsNone(t *testing.T) {
	root := newTestRoot(t)
	key, _, err := registry.CreateKey(root, lastServerKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := key.SetDWordValue(lastServerValueName, 1); err != nil {
		key.Close()
		t.Fatalf("write wrong-typed value: %v", err)
	}
	key.Close()

	pref := NewServerPrefWithRoot(root)
	address, ok := pref.Last()
	if ok || address != "" {
		t.Fatalf("Last = %q, %v; want no address", address, ok)
	}

	pref.Remember("https://vault.example.com")
	if address, ok = pref.Last(); !ok || address != "https://vault.example.com" {
		t.Fatalf("Last after Remember = %q, %v; want the address over the stray value", address, ok)
	}
}
