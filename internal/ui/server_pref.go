package ui

import (
	"strings"

	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/registry"
)

// serverPref is the seam over the remembered server address: the address the
// last successful login used, kept as a preference of this machine. Signing
// out, deleting a key and a revoked session all leave it alone, because the
// address is exactly what a self-hosted user needs back after signing out; only
// the next successful login replaces it. It is an interface so tests never
// touch the registry.
type serverPref interface {
	// load reports the remembered address, false when there is none.
	load() (string, bool)
	// save records address as the remembered one.
	save(address string)
}

// registryServerPref is the production seam: the HKCU value beside the
// first-run marker.
type registryServerPref struct{ pref registry.ServerPref }

func newServerPref() serverPref { return registryServerPref{pref: registry.NewServerPref()} }

func (p registryServerPref) load() (string, bool) { return p.pref.Last() }

func (p registryServerPref) save(address string) { p.pref.Remember(address) }

// nopServerPref remembers nothing, which is what the seams default to so a test
// or the prototype wiring never writes the real registry.
type nopServerPref struct{}

func (nopServerPref) load() (string, bool) { return "", false }
func (nopServerPref) save(string)          {}

// rememberedServerURL is the address a stored preference still means: "" when
// it is blank, unparsable or not https. The value survives restarts and goes
// through the same editing a hand-typed address does, so a broken one falls
// back to the default instead of putting junk in the login form.
func rememberedServerURL(raw string) string {
	normalized, err := auth.NormalizeBaseURL(raw)
	if err != nil {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(normalized), "https://") {
		return ""
	}
	return normalized
}
