package auth

import "context"

// TwoFactorProvider is the numbered second-factor method in the Bitwarden
// protocol. The numbers match the official TwoFactorProviderType enum and are
// what the server sends in TwoFactorProviders.
type TwoFactorProvider int

const (
	// TwoFactorAuthenticator is provider 0: a TOTP code from an authenticator
	// app. Pure user input, fully supportable.
	TwoFactorAuthenticator TwoFactorProvider = 0
	// TwoFactorEmail is provider 1: a code emailed after
	// POST {api}/two-factor/send-email-login. Pure user input.
	TwoFactorEmail TwoFactorProvider = 1
	// TwoFactorDuo is provider 2: Duo. The server returns a frameless
	// AuthURL; completion needs a browser.
	TwoFactorDuo TwoFactorProvider = 2
	// TwoFactorYubiKey is provider 3: a YubiKey OTP the user reads off the
	// device. Pure user input.
	TwoFactorYubiKey TwoFactorProvider = 3
	// TwoFactorRemember is provider 5: a previously issued TwoFactorToken,
	// used to skip 2FA on a remembered device.
	TwoFactorRemember TwoFactorProvider = 5
	// TwoFactorOrganizationDuo is provider 6: Duo enforced by the
	// organization. Same browser requirement as provider 2.
	TwoFactorOrganizationDuo TwoFactorProvider = 6
	// TwoFactorWebAuthn is provider 7: a WebAuthn assertion. The official
	// desktop client goes through the web vault's connector page, so it needs
	// a browser unless a native webauthn.dll path is built.
	TwoFactorWebAuthn TwoFactorProvider = 7
	// TwoFactorRecoveryCode is provider 8: a one-time recovery code.
	TwoFactorRecoveryCode TwoFactorProvider = 8
)

// String returns the stable name of the provider for logs and UI.
func (p TwoFactorProvider) String() string {
	switch p {
	case TwoFactorAuthenticator:
		return "authenticator"
	case TwoFactorEmail:
		return "email"
	case TwoFactorDuo:
		return "duo"
	case TwoFactorYubiKey:
		return "yubikey"
	case TwoFactorRemember:
		return "remember"
	case TwoFactorOrganizationDuo:
		return "organization-duo"
	case TwoFactorWebAuthn:
		return "webauthn"
	case TwoFactorRecoveryCode:
		return "recovery-code"
	}
	return "unknown"
}

// TwoFactorChallenge is the server's request for a second factor, extracted
// from the 400 body of {identity}/connect/token. Which fields are meaningful
// depends on the offered providers.
type TwoFactorChallenge struct {
	// Providers is the ordered list the server offered. It comes from
	// TwoFactorProviders (array) and is authoritative for ordering;
	// TwoFactorProviders2 (map) carries the per-provider payloads.
	Providers []TwoFactorProvider

	// Email is the address the email code goes to, when provider 1 is
	// offered.
	Email string
	// EmailSent records whether this flow already requested the code, so the
	// user is not sent a second one.
	EmailSent bool

	// DuoURL is the Duo frameless AuthURL returned for providers 2 and 6.
	DuoURL string

	// WebAuthnURL is the web vault connector page (webauthn-connector.html)
	// opened for provider 7 in a browser. WebAuthnChallenge and
	// WebAuthnAllowCredentials are the server's assertion options, for a
	// future native path.
	WebAuthnURL              string
	WebAuthnChallenge        string
	WebAuthnAllowCredentials [][]byte
}

// Has reports whether the server offered provider p.
func (c *TwoFactorChallenge) Has(p TwoFactorProvider) bool {
	if c == nil {
		return false
	}
	for _, offered := range c.Providers {
		if offered == p {
			return true
		}
	}
	return false
}

// TwoFactorSolver collects the second factor from the user. The GUI and the
// native-messaging host each implement it; the login flow never assumes a
// particular UI.
type TwoFactorSolver interface {
	// TOTP returns a 6-digit authenticator code (provider 0).
	TOTP(ctx context.Context) (string, error)
	// EmailCode requests an email code (provider 1) and returns the code the
	// user received. send performs the actual send-email-login request so the
	// solver can trigger it exactly once.
	EmailCode(ctx context.Context, send func(ctx context.Context) error) (string, error)
	// OpenBrowser opens a Duo AuthURL or a WebAuthn connector page in the
	// system browser and returns the token the browser path yields (providers
	// 2, 6 and 7).
	OpenBrowser(ctx context.Context, url string) (string, error)
	// YubiKey returns the YubiKey one-time password the user typed
	// (provider 3).
	YubiKey(ctx context.Context) (string, error)
}

// UnsupportedSolver is a TwoFactorSolver that pretends the user cancelled
// every method. It keeps the skeleton usable before the UI wires a real
// solver.
type UnsupportedSolver struct{}

// TOTP implements TwoFactorSolver.
func (UnsupportedSolver) TOTP(context.Context) (string, error) { return "", ErrNotImplemented }

// EmailCode implements TwoFactorSolver.
func (UnsupportedSolver) EmailCode(context.Context, func(context.Context) error) (string, error) {
	return "", ErrNotImplemented
}

// OpenBrowser implements TwoFactorSolver.
func (UnsupportedSolver) OpenBrowser(context.Context, string) (string, error) {
	return "", ErrNotImplemented
}

// YubiKey implements TwoFactorSolver.
func (UnsupportedSolver) YubiKey(context.Context) (string, error) { return "", ErrNotImplemented }

// SendEmailLogin triggers POST {api}/two-factor/send-email-login, which mails
// the 2FA code for provider 1. masterPasswordHash may be required by the
// server and is the same value as in the token request.
//
// Contract only: returns ErrNotImplemented today.
func (c *Client) SendEmailLogin(ctx context.Context, email, masterPasswordHash string) error {
	_ = c
	_ = ctx
	_ = email
	_ = masterPasswordHash
	return ErrNotImplemented
}
