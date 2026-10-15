package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

// Key sizes fixed by the Bitwarden protocol.
const (
	// masterKeyLen is the master key size: 256 bits.
	masterKeyLen = 32
	// stretchedMasterKeyLen is the expanded master key: enc (32) || mac (32).
	stretchedMasterKeyLen = 64
	// userKeyLen is the account userKey size: enc (32) || mac (32).
	userKeyLen = 64
	// passwordHashLen is the master password hash size.
	passwordHashLen = 32
)

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
// The salt is params.Salt, which Prelogin fills either with the server's Salt
// or, on the legacy prelogin variant, with the email trimmed and lowercased.
// Argon2id converts Memory from MiB to KiB and hashes the salt with SHA-256,
// matching the official SDK's derive_kdf_material; PBKDF2 uses the salt bytes
// directly.
func DeriveMasterKey(params KdfParams, password string) (MasterKey, error) {
	if params.Salt == "" {
		return nil, errors.New("auth: derive master key: missing KDF salt")
	}
	if params.Iterations == 0 {
		return nil, errors.New("auth: derive master key: zero iterations")
	}
	switch params.Type {
	case KdfPBKDF2SHA256:
		key := pbkdf2.Key([]byte(password), []byte(params.Salt), int(params.Iterations), masterKeyLen, sha256.New)
		return MasterKey(key), nil
	case KdfArgon2id:
		if params.Memory == 0 || params.Parallelism == 0 {
			return nil, errors.New("auth: derive master key: argon2id requires memory and parallelism")
		}
		salt := sha256.Sum256([]byte(params.Salt))
		key := argon2.IDKey([]byte(password), salt[:], params.Iterations, params.Memory*1024, params.Parallelism, masterKeyLen)
		return MasterKey(key), nil
	default:
		return nil, fmt.Errorf("auth: derive master key: unsupported KDF type %d", params.Type)
	}
}

// MasterPasswordHash produces the value sent as the "password" form field of
// the token request. It is not the password and not the master key: it is
// PBKDF2-HMAC-SHA256(masterKey, password, iterations=1, 32) base64-encoded.
// Note the argument order: the master key is the PBKDF2 password and the
// plaintext master password is the salt.
func MasterPasswordHash(masterKey MasterKey, password string) (string, error) {
	if len(masterKey) != masterKeyLen {
		return "", fmt.Errorf("auth: master password hash: master key is %d bytes, want %d", len(masterKey), masterKeyLen)
	}
	hash := pbkdf2.Key(masterKey, []byte(password), 1, passwordHashLen, sha256.New)
	return base64.StdEncoding.EncodeToString(hash), nil
}

// StretchMasterKey expands the 32-byte master key into the 64-byte
// enc||mac key that wraps the userKey: HKDF-Expand-SHA256(masterKey,
// info="enc", 32) followed by HKDF-Expand-SHA256(masterKey, info="mac", 32).
func StretchMasterKey(masterKey MasterKey) ([]byte, error) {
	if len(masterKey) != masterKeyLen {
		return nil, fmt.Errorf("auth: stretch master key: key is %d bytes, want %d", len(masterKey), masterKeyLen)
	}
	out := make([]byte, stretchedMasterKeyLen)
	enc := hkdf.Expand(sha256.New, masterKey, []byte("enc"))
	if _, err := io.ReadFull(enc, out[:masterKeyLen]); err != nil {
		return nil, fmt.Errorf("auth: stretch master key enc: %w", err)
	}
	mac := hkdf.Expand(sha256.New, masterKey, []byte("mac"))
	if _, err := io.ReadFull(mac, out[masterKeyLen:]); err != nil {
		return nil, fmt.Errorf("auth: stretch master key mac: %w", err)
	}
	return out, nil
}
