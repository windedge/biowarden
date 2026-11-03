package registry

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The value lives in the injected root, which newTestRoot backs with a private
// app hive: the real HKCU never sees a language choice from a test.
func TestLocalePrefRoundTrip(t *testing.T) {
	root := newTestRoot(t)
	pref := NewLocalePrefWithRoot(root)

	if tag, ok := pref.Chosen(); ok {
		t.Fatalf("Chosen on a fresh root = %q, true; want no choice", tag)
	}

	pref.Remember("zh-Hans")
	tag, ok := pref.Chosen()
	if !ok || tag != "zh-Hans" {
		t.Fatalf("Chosen after Remember = %q, %v; want the chosen tag", tag, ok)
	}

	// A later pick replaces the value; the store keeps one tag, never a
	// history.
	pref.Remember("en")
	if tag, ok = pref.Chosen(); !ok || tag != "en" {
		t.Fatalf("Chosen after a second Remember = %q, %v; want the newer tag", tag, ok)
	}

	// An empty tag must not blank what is there: the caller then has nothing
	// to record, not "forget it".
	pref.Remember("")
	if tag, ok = pref.Chosen(); !ok || tag != "en" {
		t.Fatalf("Chosen after Remember(\"\") = %q, %v; want the kept tag", tag, ok)
	}

	// Forget is what "follow the system again" writes, and forgetting twice is
	// already the desired state.
	pref.Forget()
	if tag, ok = pref.Chosen(); ok || tag != "" {
		t.Fatalf("Chosen after Forget = %q, %v; want no choice", tag, ok)
	}
	pref.Forget()
	if tag, ok = pref.Chosen(); ok || tag != "" {
		t.Fatalf("Chosen after a second Forget = %q, %v; want no choice", tag, ok)
	}
}

// An empty string value is no choice: a half-written value must read as "follow
// the system" instead of as a tag the app cannot pick a language for.
func TestLocalePrefEmptyValueIsNoChoice(t *testing.T) {
	root := newTestRoot(t)
	key, _, err := registry.CreateKey(root, localeChoiceKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := key.SetStringValue(localeChoiceValueName, ""); err != nil {
		key.Close()
		t.Fatalf("write empty value: %v", err)
	}
	key.Close()

	if tag, ok := NewLocalePrefWithRoot(root).Chosen(); ok || tag != "" {
		t.Fatalf("Chosen = %q, %v; want no choice", tag, ok)
	}
}

// A value of the wrong type reads as no choice instead of failing the read; the
// next explicit pick overwrites it.
func TestLocalePrefWrongValueTypeIsNoChoice(t *testing.T) {
	root := newTestRoot(t)
	key, _, err := registry.CreateKey(root, localeChoiceKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := key.SetDWordValue(localeChoiceValueName, 1); err != nil {
		key.Close()
		t.Fatalf("write wrong-typed value: %v", err)
	}
	key.Close()

	pref := NewLocalePrefWithRoot(root)
	if tag, ok := pref.Chosen(); ok || tag != "" {
		t.Fatalf("Chosen = %q, %v; want no choice", tag, ok)
	}

	pref.Remember("zh-Hans")
	if tag, ok := pref.Chosen(); !ok || tag != "zh-Hans" {
		t.Fatalf("Chosen after Remember = %q, %v; want the tag over the stray value", tag, ok)
	}
}
