package ui

import (
	"testing"

	"github.com/windedge/biowarden/internal/hoststate"
	"github.com/windedge/biowarden/internal/registry"
)

// A new browser is configured in four places the compiler cannot see: the
// registry.Browser enum, registry's browserKeyPaths, hoststate.Identify and
// browserIDs here. Every one of them is a table or a switch, so a missing entry
// still builds; it just loses a feature silently. internal/registry pins the
// tables that live there, this file pins the cross-package link: a browser with
// no host-side id loses its "connected" indicator, and an id no row claims is a
// browser that connects without ever showing up.

// launcherExe is the launcher name the host sees for each id it can record, so
// it stands in for hoststate.Identify's switch: an id with no row here is an id
// Identify cannot return, and a row whose Identify result disagrees with the id
// means the switch moved under this table.
var launcherExe = map[string]string{
	hoststate.BrowserChrome:   "chrome.exe",
	hoststate.BrowserChromium: "chromium.exe",
	hoststate.BrowserEdge:     "msedge.exe",
	hoststate.BrowserFirefox:  "firefox.exe",
	hoststate.BrowserBrave:    "brave.exe",
	hoststate.BrowserVivaldi:  "vivaldi.exe",
}

// hostIDsWithoutRow are ids the host records but no registration row covers:
// Brave's and Vivaldi's manifests live under vendor keys browserKeyPaths does
// not know, so biowarden cannot register them and their connection is never
// shown. Naming them keeps the gap explicit instead of silent; fixing one, or
// meeting a new one, fails the test below and forces the decision.
var hostIDsWithoutRow = []string{hoststate.BrowserBrave, hoststate.BrowserVivaldi}

func TestBrowserIDsCoverEveryRegisteredBrowser(t *testing.T) {
	for id, exe := range launcherExe {
		if got := hoststate.Identify(exe); got != id {
			t.Fatalf("hoststate.Identify(%q) = %q, want %q: the launcher table and Identify disagree", exe, got, id)
		}
	}

	covered := make(map[string]bool, len(launcherExe))
	for _, b := range registry.Browsers {
		ids := browserIDs(b)
		if len(ids) == 0 {
			t.Errorf("browserIDs(%q) is empty: the row can never show that the extension connected", b)
			continue
		}
		for _, id := range ids {
			if id == "" {
				t.Errorf("browserIDs(%q) holds an empty id", b)
				continue
			}
			if _, ok := launcherExe[id]; !ok {
				t.Errorf("browserIDs(%q) lists %q, which hoststate.Identify never returns: no connection can ever match it", b, id)
			}
			covered[id] = true
		}
	}

	// Reverse: an id the host can record but no row claims is a browser whose
	// connection is invisible, which is the same loss seen from the other side.
	for id := range launcherExe {
		if covered[id] || knownUncovered(id) {
			continue
		}
		t.Errorf("host id %q reaches no browserIDs entry: that browser connects without a \"connected\" indicator", id)
	}
	// The gap list has to stay honest: an id in it that is covered now means the
	// list outlived the gap it documents.
	for _, id := range hostIDsWithoutRow {
		if covered[id] {
			t.Errorf("%q is listed as having no row, but browserIDs covers it now: drop it from hostIDsWithoutRow", id)
		}
	}
}

func knownUncovered(id string) bool {
	for _, known := range hostIDsWithoutRow {
		if known == id {
			return true
		}
	}
	return false
}
