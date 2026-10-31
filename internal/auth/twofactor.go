package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

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

// ChallengeInteraction is how one offered second-factor method is answered.
// It is the only thing a login window needs in order to render the method, so
// it stays free of protocol detail.
type ChallengeInteraction int

const (
	// InteractionCode means the user supplies a short code: a TOTP code off an
	// authenticator app (provider 0), a code emailed by the server (provider
	// 1), or a YubiKey one-time password read off the device (provider 3).
	InteractionCode ChallengeInteraction = iota
	// InteractionBrowser means the method completes in the system browser and
	// only the resulting token comes back: Duo (providers 2 and 6) and WebAuthn
	// (provider 7). The browser path itself is deferred; see
	// docs/research/2fa-browser-path.md.
	InteractionBrowser
	// InteractionUnsupported means biowarden has no path for the method yet, so
	// a login window must not offer it. Recovery codes (provider 8) are a user
	// input like the code methods but are out of scope for this ticket.
	InteractionUnsupported
)

// String names the interaction for logs and UI.
func (i ChallengeInteraction) String() string {
	switch i {
	case InteractionCode:
		return "code"
	case InteractionBrowser:
		return "browser"
	case InteractionUnsupported:
		return "unsupported"
	}
	return "unknown"
}

// TwoFactorMethod is one offered method and the interaction it needs. Methods
// preserves the server's order so a login window can render the list as sent.
type TwoFactorMethod struct {
	// Provider is the method the submission must echo back as
	// twoFactorProvider.
	Provider TwoFactorProvider
	// Interaction is what the login window must render for this method.
	Interaction ChallengeInteraction
	// Email is where the code was sent for provider 1, as the server reported
	// it; it may be masked.
	Email string
	// BrowserURL is the page to open for InteractionBrowser. Duo fills it from
	// the Frameless AuthUrl; the WebAuthn connector URL is not built yet.
	BrowserURL string
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
	// opened for provider 7 in a browser. Building that URL from the provider
	// payload is deferred to the browser-path work; the mechanism is written
	// up in docs/research/2fa-browser-path.md. WebAuthnChallenge and
	// WebAuthnAllowCredentials are the server's assertion options, reserved
	// for a future native (webauthn.dll) path.
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

// Methods returns one TwoFactorMethod per offered provider, in the server's
// order. A login window renders these directly and picks the interaction it
// supports.
func (c *TwoFactorChallenge) Methods() []TwoFactorMethod {
	if c == nil {
		return nil
	}
	methods := make([]TwoFactorMethod, 0, len(c.Providers))
	for _, p := range c.Providers {
		methods = append(methods, c.method(p))
	}
	return methods
}

// Method returns the offered method for provider p.
func (c *TwoFactorChallenge) Method(p TwoFactorProvider) (TwoFactorMethod, bool) {
	if c == nil {
		return TwoFactorMethod{}, false
	}
	for _, offered := range c.Providers {
		if offered == p {
			return c.method(p), true
		}
	}
	return TwoFactorMethod{}, false
}

// method classifies one provider into its interaction and payload.
func (c *TwoFactorChallenge) method(p TwoFactorProvider) TwoFactorMethod {
	m := TwoFactorMethod{Provider: p, Interaction: InteractionUnsupported}
	switch p {
	case TwoFactorAuthenticator, TwoFactorYubiKey:
		m.Interaction = InteractionCode
	case TwoFactorEmail:
		m.Interaction = InteractionCode
		m.Email = c.Email
	case TwoFactorDuo, TwoFactorOrganizationDuo:
		m.Interaction = InteractionBrowser
		m.BrowserURL = c.DuoURL
	case TwoFactorWebAuthn:
		m.Interaction = InteractionBrowser
		m.BrowserURL = c.WebAuthnURL
	}
	return m
}

// providerPayload is the per-provider data map of TwoFactorProviders2. Only
// the fields the challenge surfaces are decoded here.
type providerPayload struct {
	AuthURL string `json:"AuthUrl"`
	Email   string `json:"Email"`
}

// providerNumbers decodes the TwoFactorProviders array. The Bitwarden identity
// server serializes the provider enum as a JSON *string* array, not numbers:
// the official client's fixtures pin ["0"] / ["1"]
// (bitwarden-clients libs/auth/src/common/login-strategies/login.strategy.spec.ts:359,394
// and password-login.strategy.spec.ts:496,523). Decoding those straight into
// []int made the whole body unmarshal fail, so parseTwoFactorChallenge returned
// nil and the login dead-ended on "two-factor required" with no provider list.
// Numbers are still accepted because older responses and fixtures use them.
type providerNumbers []TwoFactorProvider

// UnmarshalJSON accepts provider numbers as JSON strings or numbers and drops
// entries that are neither, so one unknown entry cannot sink the reply.
func (p *providerNumbers) UnmarshalJSON(data []byte) error {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	out := make([]TwoFactorProvider, 0, len(items))
	for _, item := range items {
		if n, ok := providerNumber(item); ok {
			out = append(out, n)
		}
	}
	*p = out
	return nil
}

// providerNumber parses one TwoFactorProviders element, a JSON string ("0") or
// number (0).
func providerNumber(raw json.RawMessage) (TwoFactorProvider, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return 0, false
		}
		return TwoFactorProvider(n), true
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return TwoFactorProvider(n), true
}

// twoFactorErrorResponse is the 400 body login returns when it wants a second
// factor (identity-two-factor.response.ts). The real server sends
// TwoFactorProviders as a string array and TwoFactorProviders2 as an object
// keyed by the provider number.
type twoFactorErrorResponse struct {
	TwoFactorProviders  providerNumbers            `json:"TwoFactorProviders"`
	TwoFactorProviders2 map[string]json.RawMessage `json:"TwoFactorProviders2"`
	Email               string                     `json:"Email"`
}

// parseTwoFactorChallenge turns the 2FA 400 body into a TwoFactorChallenge.
// It returns nil when the body carries no provider list, so a plain error is
// not mistaken for a challenge. TwoFactorProviders (the array) is
// authoritative for ordering; when it is absent the map keys are sorted
// numerically.
func parseTwoFactorChallenge(body []byte) *TwoFactorChallenge {
	var raw twoFactorErrorResponse
	// Decode best-effort: an unrecognized sibling field must not hide the
	// provider list the caller needs. Provider presence below decides whether
	// this is a challenge.
	_ = json.Unmarshal(body, &raw)
	challenge := &TwoFactorChallenge{Email: raw.Email}
	seen := make(map[TwoFactorProvider]bool)
	for _, p := range raw.TwoFactorProviders {
		provider := TwoFactorProvider(p)
		if !seen[provider] {
			challenge.Providers = append(challenge.Providers, provider)
			seen[provider] = true
		}
	}
	if len(challenge.Providers) == 0 {
		keys := make([]int, 0, len(raw.TwoFactorProviders2))
		for key := range raw.TwoFactorProviders2 {
			if n, err := strconv.Atoi(key); err == nil {
				keys = append(keys, n)
			}
		}
		sort.Ints(keys)
		for _, n := range keys {
			challenge.Providers = append(challenge.Providers, TwoFactorProvider(n))
		}
	}
	if len(challenge.Providers) == 0 {
		return nil
	}
	for key, payload := range raw.TwoFactorProviders2 {
		n, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		var data providerPayload
		_ = json.Unmarshal(payload, &data)
		switch TwoFactorProvider(n) {
		case TwoFactorEmail:
			if challenge.Email == "" {
				challenge.Email = data.Email
			}
		case TwoFactorDuo, TwoFactorOrganizationDuo:
			if challenge.DuoURL == "" && data.AuthURL != "" {
				challenge.DuoURL = data.AuthURL
			}
		}
	}
	return challenge
}

// sendEmailLoginRequest is POST {api}/two-factor/send-email-login, the JSON
// body the official client posts (two-factor-email-login.request.ts).
type sendEmailLoginRequest struct {
	Email              string `json:"email"`
	MasterPasswordHash string `json:"masterPasswordHash"`
	DeviceIdentifier   string `json:"deviceIdentifier"`
}

// SendEmailLogin triggers POST {api}/two-factor/send-email-login, which mails
// the 2FA code for provider 1. masterPasswordHash may be required by the
// server and is the same value as in the token request. The request carries
// the base64url Auth-Email header, which goldwarden also sends
// (cli/agent/bitwarden/http.go:54).
func (c *Client) SendEmailLogin(ctx context.Context, email, masterPasswordHash string) error {
	if email == "" {
		return errors.New("auth: send email login: empty email")
	}
	body := sendEmailLoginRequest{
		Email:              email,
		MasterPasswordHash: masterPasswordHash,
		DeviceIdentifier:   c.deviceID,
	}
	return c.postJSON(ctx, c.apiURL("/two-factor/send-email-login"), body, nil, func(req *http.Request) {
		setAuthEmail(req, email)
	})
}
