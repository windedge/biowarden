package ui

import (
	"testing"

	"github.com/windedge/biowarden/internal/auth"
)

// The pages may not import internal/auth, so the second-factor provider numbers
// exist twice: auth.TwoFactorProvider and the twoFactor* constants here. The
// pairing is invisible to the compiler, and a wrong number does not fail to
// build; the login window just branches on a provider the server never sends, or
// echoes a provider number the server rejects as "not offered". This table is the
// only place the two lists are compared.

func TestTwoFactorProvidersMatchAuth(t *testing.T) {
	pairs := []struct {
		name string
		ui   int
		auth auth.TwoFactorProvider
	}{
		{"twoFactorAuthenticator", twoFactorAuthenticator, auth.TwoFactorAuthenticator},
		{"twoFactorEmail", twoFactorEmail, auth.TwoFactorEmail},
		{"twoFactorDuo", twoFactorDuo, auth.TwoFactorDuo},
		{"twoFactorYubiKey", twoFactorYubiKey, auth.TwoFactorYubiKey},
		{"twoFactorOrgDuo", twoFactorOrgDuo, auth.TwoFactorOrganizationDuo},
		{"twoFactorWebAuthn", twoFactorWebAuthn, auth.TwoFactorWebAuthn},
		{"twoFactorRecoveryCode", twoFactorRecoveryCode, auth.TwoFactorRecoveryCode},
	}
	seen := make(map[int]string, len(pairs))
	for _, p := range pairs {
		if p.ui != int(p.auth) {
			t.Errorf("%s = %d, but auth.%s = %d", p.name, p.ui, p.auth.String(), int(p.auth))
		}
		if other, ok := seen[p.ui]; ok {
			t.Errorf("%s and %s share the value %d: one provider would answer for the other", other, p.name, p.ui)
		}
		seen[p.ui] = p.name
	}

	// The interactions travel as strings (promptFromChallenge copies
	// auth.ChallengeInteraction.String()), so those have to line up too.
	if interactionCode != auth.InteractionCode.String() {
		t.Errorf("interactionCode = %q, want auth.InteractionCode = %q", interactionCode, auth.InteractionCode.String())
	}
	if interactionBrowser != auth.InteractionBrowser.String() {
		t.Errorf("interactionBrowser = %q, want auth.InteractionBrowser = %q", interactionBrowser, auth.InteractionBrowser.String())
	}
}
