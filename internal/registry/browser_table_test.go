package registry

import "testing"

// A new browser is configured in several places the compiler cannot see: the
// Browser enum here, the key paths in browserKeyPaths, the App Paths names in
// appPathExecutables, hoststate.Identify and the UI's browserIDs. Each table is
// a map lookup, so a browser missing from one of them does not fail to build;
// it silently loses a feature. The tests below pin the three tables that live
// in this package; internal/ui pins the other two.

func TestBrowsersHaveKeyPaths(t *testing.T) {
	for _, b := range Browsers {
		keyPaths, ok := browserKeyPaths[b]
		if !ok {
			t.Errorf("browserKeyPaths has no entry for %q: Status, Install and Uninstall report it as unsupported", b)
			continue
		}
		if len(keyPaths) == 0 {
			t.Errorf("browserKeyPaths[%q] is empty: Install would write no registration key at all", b)
		}
		for _, keyPath := range keyPaths {
			if keyPath == "" {
				t.Errorf("browserKeyPaths[%q] holds an empty key path", b)
			}
		}
	}
}

func TestBrowsersHaveAppPathExecutables(t *testing.T) {
	for _, b := range Browsers {
		executables, ok := appPathExecutables[b]
		if !ok {
			t.Errorf("appPathExecutables has no entry for %q: Detect never reports it installed", b)
			continue
		}
		if len(executables) == 0 {
			t.Errorf("appPathExecutables[%q] is empty: Detect never reports it installed", b)
		}
		for _, exe := range executables {
			if exe == "" {
				t.Errorf("appPathExecutables[%q] holds an empty executable name", b)
			}
		}
	}
}

// TestBrowserTablesHaveNoOrphans is the other direction: a row in Browsers is
// what the GUI renders, so a table entry for a browser that Browsers does not
// list is configuration nothing reads.
func TestBrowserTablesHaveNoOrphans(t *testing.T) {
	listed := make(map[Browser]bool, len(Browsers))
	for _, b := range Browsers {
		if listed[b] {
			t.Errorf("%q is listed twice in Browsers: the GUI would draw two rows for it", b)
		}
		listed[b] = true
	}
	for b := range browserKeyPaths {
		if !listed[b] {
			t.Errorf("browserKeyPaths registers %q, which Browsers never lists: it has no row and no Install path", b)
		}
	}
	for b := range appPathExecutables {
		if !listed[b] {
			t.Errorf("appPathExecutables probes %q, which Browsers never lists: the probe result is discarded", b)
		}
	}
}
