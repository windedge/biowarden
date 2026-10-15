package auth

// EncStringType is the numeric prefix of a Bitwarden EncString, for example
// the "2" in "2.iv|ct|mac". The numbers are wire values and must not change.
type EncStringType int

const (
	// EncTypeAesCbc256 is type 0: AES-256-CBC, no MAC. Legacy and insecure.
	EncTypeAesCbc256 EncStringType = 0
	// EncTypeAesCbc128HmacSha256 is type 1: AES-128-CBC + HMAC-SHA256.
	EncTypeAesCbc128HmacSha256 EncStringType = 1
	// EncTypeAesCbc256HmacSha256 is type 2: AES-256-CBC + HMAC-SHA256. The
	// token response Key (masterKey-wrapped userKey) uses exactly this type,
	// with a 16-byte IV, 48-byte ciphertext and 32-byte MAC.
	EncTypeAesCbc256HmacSha256 EncStringType = 2
	// EncTypeRsa2048OaepSha256 is type 3.
	EncTypeRsa2048OaepSha256 EncStringType = 3
	// EncTypeRsa2048OaepSha1 is type 4.
	EncTypeRsa2048OaepSha1 EncStringType = 4
	// EncTypeRsa2048OaepSha256HmacSha256 is type 5.
	EncTypeRsa2048OaepSha256HmacSha256 EncStringType = 5
	// EncTypeRsa2048OaepSha1HmacSha256 is type 6.
	EncTypeRsa2048OaepSha1HmacSha256 EncStringType = 6
)

// EncString is a parsed Bitwarden encrypted string.
type EncString struct {
	// Type is the algorithm selector parsed from the "N." prefix.
	Type EncStringType
	// IV is the base64-decoded initialisation vector.
	IV []byte
	// CT is the base64-decoded ciphertext.
	CT []byte
	// MAC is the base64-decoded HMAC tag, empty for types without one.
	MAC []byte
}

// String redacts the ciphertext so it cannot reach a log through %v.
func (EncString) String() string { return "auth.EncString(<redacted>)" }

// DecodeEncString parses a "type.iv|ct[|mac]" EncString. The fields for a
// given type are fixed: type 2 has iv, ct and mac; type 0 has iv and ct only.
//
// Contract only: the core login ticket implements the parser. It returns
// ErrNotImplemented today.
func DecodeEncString(s string) (EncString, error) {
	_ = s
	return EncString{}, ErrNotImplemented
}

// UnmarshalText lets an EncString field in a JSON response decode from its
// wire form.
func (e *EncString) UnmarshalText(text []byte) error {
	parsed, err := DecodeEncString(string(text))
	if err != nil {
		return err
	}
	*e = parsed
	return nil
}

// UnlockUserKey decrypts the token response's Key, which is the account's
// 64-byte userKey wrapped with the stretched master key.
//
// The path is: StretchMasterKey(masterKey) -> 64 bytes split into a 32-byte
// enc key and a 32-byte mac key -> verify EncString type 2's HMAC over
// iv||ct -> AES-256-CBC decrypt -> the userKey (64 bytes split into
// enc||mac when it is later used as a symmetric key).
//
// Contract only: the core login ticket implements this. It returns
// ErrNotImplemented today.
func UnlockUserKey(masterKey MasterKey, encryptedKey EncString) ([]byte, error) {
	_ = masterKey
	_ = encryptedKey
	return nil, ErrNotImplemented
}
