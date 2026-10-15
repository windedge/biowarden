package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// TokenRequest is the password grant body sent to {identity}/connect/token.
// The field names are the wire form names: token requests are
// application/x-www-form-urlencoded, unlike the JSON API calls.
type TokenRequest struct {
	// Email is the account's login address, sent as "username".
	Email string
	// MasterPasswordHash is MasterPasswordHash output, sent as "password".
	// It is never the plaintext master password.
	MasterPasswordHash string
	// DeviceID is sent as "deviceIdentifier". The official desktop client
	// uses its appId GUID; biowarden uses a stable per-install identifier.
	DeviceID string

	// TwoFactorProvider and TwoFactorToken are filled on the retry after
	// Login reported ErrTwoFactorRequired. Remember sets twoFactorRemember=1,
	// which asks the server to return a twoFactorToken for later logins.
	TwoFactorProvider TwoFactorProvider
	TwoFactorToken    string
	Remember          bool

	// NewDeviceOTP carries the code for the new-device-verification flow.
	NewDeviceOTP string
}

// TokenResponse is a successful grant from {identity}/connect/token.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`

	// Key is the master key's wrapped userKey ("Key" in the response, though
	// its real meaning is masterKeyEncryptedUserKey). It is an EncString of
	// type 2 and UnlockUserKey turns it into the 64-byte userKey.
	Key EncString `json:"Key"`
	// PrivateKey is the userKey-wrapped account private key. Not needed to
	// unlock; retained for the vault work.
	PrivateKey EncString `json:"PrivateKey"`

	// TwoFactorToken is the "remember this device" credential, returned when
	// the request set Remember. Store it per account; the next login sends it
	// as TwoFactorToken with TwoFactorProvider = TwoFactorRemember.
	TwoFactorToken string `json:"TwoFactorToken"`

	// Challenge is set only when the token endpoint answered 400 with the
	// available providers, i.e. when Login returns ErrTwoFactorRequired. It
	// carries the full provider list (the TwoFactorProviders /
	// TwoFactorProviders2 fields of the error body).
	Challenge *TwoFactorChallenge
}

// String redacts the bearer tokens and the wrapped key material so they
// cannot reach a log through %v or fmt.Sprintf.
func (TokenResponse) String() string { return "auth.TokenResponse(<redacted>)" }

// Profile is the subset of {api}/accounts/profile the login flow uses.
type Profile struct {
	ID    string `json:"Id"`
	Email string `json:"Email"`
	Name  string `json:"Name"`
}

// preloginRequest is the JSON body both prelogin variants accept.
type preloginRequest struct {
	Email string `json:"email"`
}

// kdfSettingsResponse is the KDF block of the new password prelogin response.
// Pointers distinguish a missing field from a legitimate zero.
type kdfSettingsResponse struct {
	KdfType     *int    `json:"KdfType"`
	Iterations  *uint32 `json:"Iterations"`
	Memory      *uint32 `json:"Memory"`
	Parallelism *uint8  `json:"Parallelism"`
}

// passwordPreloginResponse is
// POST {identity}/accounts/prelogin/password.
type passwordPreloginResponse struct {
	KdfSettings kdfSettingsResponse `json:"KdfSettings"`
	Salt        string              `json:"Salt"`
}

// legacyPreloginResponse is POST {identity}/accounts/prelogin. It has no
// salt; the email supplies it. encoding/json matches the field names
// case-insensitively, so both the PascalCase cloud response and the
// all-caps curl examples decode.
type legacyPreloginResponse struct {
	Kdf            *int    `json:"Kdf"`
	KdfIterations  *uint32 `json:"KdfIterations"`
	KdfMemory      *uint32 `json:"KdfMemory"`
	KdfParallelism *uint8  `json:"KdfParallelism"`
}

// Prelogin fetches the KDF parameters for email before a master key can be
// derived.
//
// The primary endpoint is POST {identity}/accounts/prelogin/password, whose
// response is {KdfSettings:{KdfType,Iterations,Memory,Parallelism}, Salt};
// the server-provided Salt then becomes KdfParams.Salt. If that endpoint is
// missing (older servers, Vaultwarden), it falls back to the legacy
// POST {identity}/accounts/prelogin, which returns KDF/KDFIterations/
// KDFMemory/KDFParallelism and no salt; the salt is then the trimmed,
// lowercased email.
func (c *Client) Prelogin(ctx context.Context, email string) (KdfParams, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return KdfParams{}, errors.New("auth: prelogin: empty email")
	}
	params, err := c.preloginPassword(ctx, email)
	if err == nil {
		return params, nil
	}
	legacy, legacyErr := c.preloginLegacy(ctx, email)
	if legacyErr != nil {
		return KdfParams{}, fmt.Errorf("auth: prelogin: %w", legacyErr)
	}
	return legacy, nil
}

// preloginPassword reads the new password prelogin variant.
func (c *Client) preloginPassword(ctx context.Context, email string) (KdfParams, error) {
	var resp passwordPreloginResponse
	if err := c.postJSON(ctx, c.identityURL("/accounts/prelogin/password"), preloginRequest{Email: email}, &resp); err != nil {
		return KdfParams{}, err
	}
	if resp.KdfSettings.KdfType == nil || resp.KdfSettings.Iterations == nil {
		return KdfParams{}, errors.New("prelogin response has no KDF settings")
	}
	params := KdfParams{
		Type:       KdfType(*resp.KdfSettings.KdfType),
		Iterations: *resp.KdfSettings.Iterations,
		Salt:       resp.Salt,
	}
	if params.Type == KdfArgon2id {
		if resp.KdfSettings.Memory == nil || resp.KdfSettings.Parallelism == nil {
			return KdfParams{}, errors.New("prelogin argon2id response is missing memory or parallelism")
		}
		params.Memory = *resp.KdfSettings.Memory
		params.Parallelism = *resp.KdfSettings.Parallelism
	}
	if err := params.validate(); err != nil {
		return KdfParams{}, err
	}
	if params.Salt == "" {
		return KdfParams{}, errors.New("prelogin password response has no salt")
	}
	return params, nil
}

// preloginLegacy reads the old prelogin variant, where the email is the salt.
func (c *Client) preloginLegacy(ctx context.Context, email string) (KdfParams, error) {
	var resp legacyPreloginResponse
	if err := c.postJSON(ctx, c.identityURL("/accounts/prelogin"), preloginRequest{Email: email}, &resp); err != nil {
		return KdfParams{}, err
	}
	if resp.Kdf == nil || resp.KdfIterations == nil {
		return KdfParams{}, errors.New("prelogin response has no KDF settings")
	}
	params := KdfParams{
		Type:       KdfType(*resp.Kdf),
		Iterations: *resp.KdfIterations,
		// The legacy variant has no salt, so it is the email trimmed and
		// lowercased (legacy-compat-key.service.ts makeMasterKey).
		Salt: strings.TrimSpace(strings.ToLower(email)),
	}
	if params.Type == KdfArgon2id {
		if resp.KdfMemory == nil || resp.KdfParallelism == nil {
			return KdfParams{}, errors.New("prelogin argon2id response is missing memory or parallelism")
		}
		params.Memory = *resp.KdfMemory
		params.Parallelism = *resp.KdfParallelism
	}
	if err := params.validate(); err != nil {
		return KdfParams{}, err
	}
	return params, nil
}

// Prelogin KDF minimums, copied from the official client's
// validateKdfConfigForPrelogin:
//   - PBKDF2 iterations >= 5000
//     (libs/legacy-crypto/src/models/kdf-config.ts:19,46-52)
//   - Argon2id iterations >= 2, memory >= 16 MiB, parallelism >= 1
//     (libs/legacy-crypto/src/models/kdf-config.ts:75-77,126-144)
//
// A server advertising less may be mounting a pre-login downgrade: the client
// would derive a weak master key from the real master password.
const (
	preloginPBKDF2IterationsMin  uint32 = 5000
	preloginArgon2IterationsMin  uint32 = 2
	preloginArgon2MemoryMin      uint32 = 16
	preloginArgon2ParallelismMin uint8  = 1
)

// validate rejects KDF parameters that cannot produce a key, or that fall
// below the prelogin minimums the official client enforces.
func (p KdfParams) validate() error {
	switch p.Type {
	case KdfPBKDF2SHA256:
		if p.Iterations < preloginPBKDF2IterationsMin {
			return fmt.Errorf("prelogin response has %d PBKDF2 iterations, want at least %d (possible pre-login downgrade attack)", p.Iterations, preloginPBKDF2IterationsMin)
		}
	case KdfArgon2id:
		if p.Iterations < preloginArgon2IterationsMin {
			return fmt.Errorf("prelogin response has %d Argon2id iterations, want at least %d (possible pre-login downgrade attack)", p.Iterations, preloginArgon2IterationsMin)
		}
		if p.Memory < preloginArgon2MemoryMin {
			return fmt.Errorf("prelogin response has %d MiB Argon2id memory, want at least %d (possible pre-login downgrade attack)", p.Memory, preloginArgon2MemoryMin)
		}
		if p.Parallelism < preloginArgon2ParallelismMin {
			return fmt.Errorf("prelogin response has %d Argon2id parallelism, want at least %d (possible pre-login downgrade attack)", p.Parallelism, preloginArgon2ParallelismMin)
		}
	default:
		return fmt.Errorf("unsupported KDF type %d", p.Type)
	}
	return nil
}

// Login posts the password grant to {identity}/connect/token.
//
// It sends client_id=desktop, deviceType=6, deviceName=windows,
// scope="api offline_access", grant_type="password" and the desktop identity
// headers. A 400 carrying TwoFactorProviders2 is reported as
// ErrTwoFactorRequired and the returned TokenResponse has Challenge filled in.
func (c *Client) Login(ctx context.Context, req TokenRequest) (TokenResponse, error) {
	form := url.Values{
		"scope":      {"api offline_access"},
		"client_id":  {ClientID},
		"deviceType": {DeviceType},
		"deviceName": {DeviceName},
		"grant_type": {"password"},
		"username":   {req.Email},
		"password":   {req.MasterPasswordHash},
	}
	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = c.deviceID
	}
	form.Set("deviceIdentifier", deviceID)
	if req.TwoFactorToken != "" || req.TwoFactorProvider != 0 {
		form.Set("twoFactorProvider", strconv.Itoa(int(req.TwoFactorProvider)))
		form.Set("twoFactorToken", req.TwoFactorToken)
		form.Set("twoFactorRemember", boolFormValue(req.Remember))
	}
	if req.NewDeviceOTP != "" {
		form.Set("newDeviceOtp", req.NewDeviceOTP)
	}

	body, err := c.postFormBody(ctx, c.identityURL("/connect/token"), form)
	if err != nil {
		if errors.Is(err, ErrTwoFactorRequired) {
			// Structural, value-free diagnostic: which fields the reply
			// carried and which provider numbers it offered. Written to the
			// shared host log only when BIOWARDEN_LOG=debug.
			logTwoFactorShape(body)
			challenge := parseTwoFactorChallenge(body)
			if challenge == nil {
				// The server's message classified as 2FA but the body carried no
				// provider list. Returning a challenge-less response would leave
				// the caller in a state it can neither answer nor retry, so keep
				// the error and make the cause explicit.
				return TokenResponse{}, fmt.Errorf("%w: the response has no two-factor provider list", err)
			}
			return TokenResponse{Challenge: challenge}, err
		}
		return TokenResponse{}, err
	}
	var out TokenResponse
	if err := decodeJSON(body, &out); err != nil {
		return TokenResponse{}, err
	}
	return out, nil
}

// postFormBody sends a form body and returns the raw response body even when
// the server answered non-2xx, so Login can read a structured 2FA challenge.
func (c *Client) postFormBody(ctx context.Context, rawurl string, form url.Values) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rawurl, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	c.applyHeaders(httpReq)
	return c.do(httpReq)
}

// Refresh exchanges a refresh token at the same {identity}/connect/token
// endpoint with grant_type=refresh_token. The official client refreshes with
// the client_id decoded from the access token (api.service.ts
// refreshAccessToken); biowarden always logs in as the desktop client, so it
// reuses ClientID.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (TokenResponse, error) {
	if refreshToken == "" {
		return TokenResponse{}, errors.New("auth: refresh: empty refresh token")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {ClientID},
	}
	var out TokenResponse
	if err := c.postForm(ctx, c.identityURL("/connect/token"), form, &out); err != nil {
		return TokenResponse{}, err
	}
	return out, nil
}

// Logout clears the session. There is no endpoint to call: the official
// clients drop local tokens and vault state on logout and never hit
// {identity}/connect/revocation, and goldwarden likewise only purges its local
// config. This method exists as the seam for local cleanup; the caller clears
// the stored key through secrets.Store.Remove.
func (c *Client) Logout(ctx context.Context, accessToken string) error {
	_ = ctx
	_ = accessToken
	return nil
}

// Profile reads GET {api}/accounts/profile with a bearer token, to confirm
// the account and obtain the user id.
func (c *Client) Profile(ctx context.Context, accessToken string) (Profile, error) {
	if accessToken == "" {
		return Profile{}, errors.New("auth: profile: empty access token")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL("/accounts/profile"), nil)
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	c.applyHeaders(httpReq)
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	var out Profile
	if err := c.decode(httpReq, &out); err != nil {
		return Profile{}, err
	}
	return out, nil
}

// boolFormValue renders a form flag the way the identity server expects.
func boolFormValue(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
