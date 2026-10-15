package registry

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The marker writes to the injected root, which newTestEnv already points at a
// fresh HKCU\Software\Biowarden\Test\<random> key: the real HKCU never sees a
// first-run value from a test.
func TestMarkerRoundTrip(t *testing.T) {
	env := newTestEnv(t)
	marker := NewMarkerWithRoot(env.root)

	done, err := marker.Done()
	if err != nil {
		t.Fatalf("Done on a fresh root: %v", err)
	}
	if done {
		t.Fatal("Done = true before anything was written")
	}

	if err := marker.Mark(); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	done, err = marker.Done()
	if err != nil {
		t.Fatalf("Done after Mark: %v", err)
	}
	if !done {
		t.Fatal("Done = false after Mark")
	}

	// A second write is what a repeated setup would do; it must not fail.
	if err := marker.Mark(); err != nil {
		t.Fatalf("second Mark: %v", err)
	}
	if done, err = marker.Done(); err != nil || !done {
		t.Fatalf("Done after a second Mark = %v, %v; want true, nil", done, err)
	}
}

// A key that exists without the value is not a completed setup: a reader must
// not treat a stray subkey as done.
func TestMarkerEmptyKeyIsNotDone(t *testing.T) {
	env := newTestEnv(t)
	key, _, err := registry.CreateKey(env.root, firstRunKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create marker key: %v", err)
	}
	key.Close()

	done, err := NewMarkerWithRoot(env.root).Done()
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if done {
		t.Fatal("Done = true for an empty marker key")
	}
}

// A value of the wrong type reads as "not done" instead of failing the launch.
func TestMarkerWrongValueTypeIsNotDone(t *testing.T) {
	env := newTestEnv(t)
	key, _, err := registry.CreateKey(env.root, firstRunKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create marker key: %v", err)
	}
	if err := key.SetStringValue(firstRunValueName, "yes"); err != nil {
		key.Close()
		t.Fatalf("write wrong-typed value: %v", err)
	}
	key.Close()

	done, err := NewMarkerWithRoot(env.root).Done()
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if done {
		t.Fatal("Done = true for a string value")
	}
}
