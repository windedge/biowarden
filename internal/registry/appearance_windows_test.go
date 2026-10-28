package registry

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The value lives in the injected root, which newTestRoot backs with a private
// app hive: the real HKCU never sees an appearance from a test.
func TestAppearancePrefRoundTrip(t *testing.T) {
	root := newTestRoot(t)
	pref := NewAppearancePrefWithRoot(root)

	if got := pref.Current(); got != AppearanceSystem {
		t.Fatalf("Current on a fresh root = %q, want the system's own", got)
	}

	for _, want := range []Appearance{AppearanceDark, AppearanceLight, AppearanceSystem} {
		pref.Remember(want)
		if got := pref.Current(); got != want {
			t.Fatalf("Current after Remember(%q) = %q", want, got)
		}
	}

	// The stored value is the plain word, so a fresh install's default and a
	// hand-written value mean what the app means by them.
	if got := readAppearanceValue(t, root); got != "system" {
		t.Fatalf("the stored value is %q, want %q", got, "system")
	}
}

// An unknown or wrong-typed value is no choice at all: the system's own.
func TestAppearancePrefUnknownValuesAreTheSystem(t *testing.T) {
	cases := []struct {
		name  string
		write func(registry.Key) error
	}{
		{"garbage", func(k registry.Key) error { return k.SetStringValue(appearanceValueName, "dunkel") }},
		{"empty", func(k registry.Key) error { return k.SetStringValue(appearanceValueName, "") }},
		{"number", func(k registry.Key) error { return k.SetDWordValue(appearanceValueName, 1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newTestRoot(t)
			key, _, err := registry.CreateKey(root, appearanceKeyPath, registry.SET_VALUE)
			if err != nil {
				t.Fatalf("create key: %v", err)
			}
			if err := tc.write(key); err != nil {
				key.Close()
				t.Fatalf("write value: %v", err)
			}
			key.Close()

			pref := NewAppearancePrefWithRoot(root)
			if got := pref.Current(); got != AppearanceSystem {
				t.Fatalf("Current = %q, want the system's own", got)
			}

			pref.Remember(AppearanceDark)
			if got := pref.Current(); got != AppearanceDark {
				t.Fatalf("Current after Remember(dark) = %q, want dark over the stray value", got)
			}
		})
	}
}

func readAppearanceValue(t *testing.T, root registry.Key) string {
	t.Helper()
	key, err := registry.OpenKey(root, appearanceKeyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("open key: %v", err)
	}
	defer key.Close()
	value, _, err := key.GetStringValue(appearanceValueName)
	if err != nil {
		t.Fatalf("read value: %v", err)
	}
	return value
}
