package ui

import (
	"testing"

	"github.com/windedge/biowarden/internal/hello"
)

// fakeServerPref is the remembered-address seam without a registry: it holds
// one address in memory and records every write, so a test can see both what a
// login stored and that a sign-out left it alone.
type fakeServerPref struct {
	address string
	has     bool
	saved   []string
}

func (p *fakeServerPref) load() (string, bool) { return p.address, p.has }

func (p *fakeServerPref) save(address string) {
	p.saved = append(p.saved, address)
	p.address, p.has = address, address != ""
}

// A stored value survives restarts and hand edits, so anything that is not a
// usable https address must be ignored instead of reaching the login form.
func TestRememberedServerURLToleratesBrokenValues(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"blank", "   ", ""},
		{"no host", "not a url", ""},
		{"not https", "http://vault.example.com", ""},
		{"bare host", "vault.example.com", "https://vault.example.com"},
		{"trailing slash", "https://vault.example.com/", "https://vault.example.com"},
		{"upper case", "HTTPS://Vault.Example.Com", "https://vault.example.com"},
		{"path prefix", "https://example.com/bitwarden", "https://example.com/bitwarden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rememberedServerURL(tc.raw); got != tc.want {
				t.Fatalf("rememberedServerURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// A successful login records the address it used, and signing out afterwards
// keeps it: the session and the stored key go, the preference stays.
func TestLoginRemembersTheServerAndLogoutKeepsIt(t *testing.T) {
	store := newFakeStore()
	verifier := &fakeVerifier{result: hello.Verified}
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	prefs := &fakeServerPref{}
	a := newRealAccount(store, verifier, fixedFlow(flow))
	a.prefs = prefs

	if _, err := a.login("ada@example.com", "pw", "https://vault.example.com"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if !prefs.has || prefs.address != "https://vault.example.com" {
		t.Fatalf("remembered = %q, %v; want the login's address", prefs.address, prefs.has)
	}

	if err := a.logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if !prefs.has || prefs.address != "https://vault.example.com" {
		t.Fatalf("remembered after logout = %q, %v; want it kept", prefs.address, prefs.has)
	}
	if len(store.accounts) != 0 {
		t.Fatalf("stored accounts = %d after logout, want 0", len(store.accounts))
	}
}

// A login that never finishes must not record anything: the preference says
// where a login succeeded, not where one was attempted.
func TestFailedLoginDoesNotRememberTheServer(t *testing.T) {
	flow := &scriptedAuth{step: doneStep("user-1", "ada@example.com", []byte("key"))}
	prefs := &fakeServerPref{}
	a := newRealAccount(newFakeStore(), &fakeVerifier{result: hello.Canceled}, fixedFlow(flow))
	a.prefs = prefs

	if _, err := a.login("ada@example.com", "pw", "https://vault.example.com"); err == nil {
		t.Fatal("login with canceled consent = nil, want an error")
	}
	if len(prefs.saved) != 0 {
		t.Fatalf("remembered %v, want nothing", prefs.saved)
	}
}
