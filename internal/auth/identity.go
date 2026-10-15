package auth

import "context"

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

// Profile is the subset of {api}/accounts/profile the login flow uses.
type Profile struct {
	ID    string `json:"Id"`
	Email string `json:"Email"`
	Name  string `json:"Name"`
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
//
// Contract only: returns ErrNotImplemented today.
func (c *Client) Prelogin(ctx context.Context, email string) (KdfParams, error) {
	_ = c
	_ = ctx
	_ = email
	return KdfParams{}, ErrNotImplemented
}

// Login posts the password grant to {identity}/connect/token.
//
// It must send client_id=desktop, deviceType=6, deviceName=windows,
// scope="api offline_access", grant_type="password" and the desktop identity
// headers, and it must report a 400 carrying TwoFactorProviders2 as
// ErrTwoFactorRequired with TokenResponse.Challenge filled in.
//
// Contract only: returns ErrNotImplemented today.
func (c *Client) Login(ctx context.Context, req TokenRequest) (TokenResponse, error) {
	_ = c
	_ = ctx
	_ = req
	return TokenResponse{}, ErrNotImplemented
}

// Refresh exchanges a refresh token at the same {identity}/connect/token
// endpoint with grant_type=refresh_token.
//
// Contract only: returns ErrNotImplemented today.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (TokenResponse, error) {
	_ = c
	_ = ctx
	_ = refreshToken
	return TokenResponse{}, ErrNotImplemented
}

// Logout clears the session. The official clients drop local state and do not
// call a revocation endpoint; this is the seam for that local cleanup.
//
// Contract only: returns ErrNotImplemented today.
func (c *Client) Logout(ctx context.Context, accessToken string) error {
	_ = c
	_ = ctx
	_ = accessToken
	return ErrNotImplemented
}

// Profile reads GET {api}/accounts/profile with a bearer token, to confirm
// the account and obtain the user id.
//
// Contract only: returns ErrNotImplemented today.
func (c *Client) Profile(ctx context.Context, accessToken string) (Profile, error) {
	_ = c
	_ = ctx
	_ = accessToken
	return Profile{}, ErrNotImplemented
}
