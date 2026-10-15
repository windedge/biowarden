package auth

// KdfType is the key derivation function the account uses. The numbers are
// the wire values from Bitwarden's KdfType enum and must not change.
type KdfType int

const (
	// KdfPBKDF2SHA256 is KdfType 0. Derivation is
	// PBKDF2-HMAC-SHA256(password, lower(email), iterations, 32).
	KdfPBKDF2SHA256 KdfType = 0
	// KdfArgon2id is KdfType 1. Derivation is
	// Argon2id(password, sha256(lower(email)), iterations, memory, parallelism, 32).
	KdfArgon2id KdfType = 1
)

// KdfParams is what the prelogin endpoint tells the client before it can
// derive a master key.
//
// Memory is in MiB, the unit the Bitwarden API uses; note that
// golang.org/x/crypto/argon2 wants KiB, so the implementation must multiply by
// 1024. This is the single easiest place for a silent interop bug.
type KdfParams struct {
	// Type selects PBKDF2-SHA256 or Argon2id.
	Type KdfType
	// Iterations is the PBKDF2 round count, or Argon2id time cost.
	Iterations uint32
	// Memory is Argon2id memory in MiB. Ignored for PBKDF2.
	Memory uint32
	// Parallelism is Argon2id lanes. Ignored for PBKDF2.
	Parallelism uint8
	// Salt is the server-provided salt from the new
	// {identity}/accounts/prelogin/password response. Empty means the legacy
	// variant, where the salt is the lowercased, trimmed email.
	Salt string
}

// MasterKey is the 32-byte key derived from the master password. It is the
// root secret the userKey is unwrapped with and never leaves the process.
type MasterKey []byte

// String redacts the key so it cannot reach a log through %v.
func (MasterKey) String() string { return "auth.MasterKey(<redacted>)" }

// DeriveMasterKey turns the master password and the server's KDF parameters
// into the 32-byte master key.
//
// The salt is params.Salt when the server supplied one, otherwise the email
// trimmed and lowercased. Argon2id must convert Memory from MiB to KiB before
// calling argon2.IDKey.
//
// Contract only: the core login ticket implements this. It returns
// ErrNotImplemented today.
func DeriveMasterKey(params KdfParams, password string) (MasterKey, error) {
	_ = params
	_ = password
	return nil, ErrNotImplemented
}

// MasterPasswordHash produces the value sent as the "password" form field of
// the token request. It is not the password and not the master key: it is
// PBKDF2-HMAC-SHA256(masterKey, password, iterations=1, 32) base64-encoded.
// Note the argument order: the master key is the PBKDF2 password and the
// plaintext master password is the salt.
//
// Contract only: returns ErrNotImplemented today.
func MasterPasswordHash(masterKey MasterKey, password string) (string, error) {
	_ = masterKey
	_ = password
	return "", ErrNotImplemented
}

// StretchMasterKey expands the 32-byte master key into the 64-byte
// enc||mac key that wraps the userKey: HKDF-Expand-SHA256(masterKey,
// info="enc", 32) followed by HKDF-Expand-SHA256(masterKey, info="mac", 32).
//
// Contract only: returns ErrNotImplemented today.
func StretchMasterKey(masterKey MasterKey) ([]byte, error) {
	_ = masterKey
	return nil, ErrNotImplemented
}
