package ui

import (
	"testing"

	"github.com/windedge/biowarden/internal/registry"
)

// recordingLocalePref is the registry seam without HKCU: the UI adapter's only
// job is to delegate, so a fake is enough to prove it does.
type recordingLocalePref struct {
	tag     string
	has     bool
	forgets int
}

func (p *recordingLocalePref) Chosen() (string, bool) { return p.tag, p.has }

func (p *recordingLocalePref) Remember(tag string) { p.tag, p.has = tag, true }

func (p *recordingLocalePref) Forget() { p.tag, p.has, p.forgets = "", false, p.forgets+1 }

// The UI seam delegates straight to the registry value, so moving the marker out
// of this package changed no behaviour: save stores, clear forgets, load reads.
func TestRegistryLocaleMarkerDelegates(t *testing.T) {
	pref := &recordingLocalePref{}
	marker := registryLocaleMarker{pref: pref}

	if tag, ok := marker.load(); ok || tag != "" {
		t.Fatalf("load on a fresh pref = %q, %v; want no choice", tag, ok)
	}

	marker.save("zh-Hans")
	if pref.tag != "zh-Hans" || !pref.has {
		t.Fatalf("pref = %q, %v; want the saved tag", pref.tag, pref.has)
	}
	if tag, ok := marker.load(); !ok || tag != "zh-Hans" {
		t.Fatalf("load = %q, %v; want zh-Hans", tag, ok)
	}

	marker.clear()
	if pref.forgets != 1 || pref.has {
		t.Fatalf("forgets = %d, has = %v; want one forget and no choice", pref.forgets, pref.has)
	}
	if tag, ok := marker.load(); ok || tag != "" {
		t.Fatalf("load after clear = %q, %v; want no choice", tag, ok)
	}

	// The production constructor must build a seam over the registry, not a
	// nil that a Settings click would panic on.
	if newLocaleMarker() == nil {
		t.Fatal("newLocaleMarker returned nil")
	}
}

// A state built by hand, as the page tests do, has no marker and no locale: the
// language row must report "follow the system" instead of panicking.
func TestNilMarkerFollowsTheSystem(t *testing.T) {
	s := &settingsState{}
	if tag := s.guessLanguageTag(); tag != "" {
		t.Fatalf("guessLanguageTag with a nil marker = %q, want the system choice", tag)
	}
	s.setLanguage("zh-Hans")
	if s.languageTag != "zh-Hans" {
		t.Fatalf("languageTag = %q, want the choice kept without a marker", s.languageTag)
	}
}

var _ registry.LocalePref = (*recordingLocalePref)(nil)
