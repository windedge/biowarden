package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

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
func DecodeEncString(s string) (EncString, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return EncString{}, errors.New("auth: decode encrypted string: empty input")
	}
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return EncString{}, errors.New("auth: decode encrypted string: missing type prefix")
	}
	typeValue, err := strconv.Atoi(s[:dot])
	if err != nil {
		return EncString{}, fmt.Errorf("auth: decode encrypted string: bad type prefix %q", s[:dot])
	}
	encType := EncStringType(typeValue)
	parts := strings.Split(s[dot+1:], "|")

	var iv, ct, mac []byte
	switch encType {
	case EncTypeAesCbc256:
		if len(parts) != 2 {
			return EncString{}, fmt.Errorf("auth: decode encrypted string: type %d wants iv|ct, got %d parts", encType, len(parts))
		}
		if iv, err = decodeBase64(parts[0]); err != nil {
			return EncString{}, err
		}
		if ct, err = decodeBase64(parts[1]); err != nil {
			return EncString{}, err
		}
	case EncTypeAesCbc128HmacSha256, EncTypeAesCbc256HmacSha256:
		if len(parts) != 3 {
			return EncString{}, fmt.Errorf("auth: decode encrypted string: type %d wants iv|ct|mac, got %d parts", encType, len(parts))
		}
		if iv, err = decodeBase64(parts[0]); err != nil {
			return EncString{}, err
		}
		if ct, err = decodeBase64(parts[1]); err != nil {
			return EncString{}, err
		}
		if mac, err = decodeBase64(parts[2]); err != nil {
			return EncString{}, err
		}
	default:
		return EncString{}, fmt.Errorf("auth: decode encrypted string: unsupported type %d", encType)
	}
	return EncString{Type: encType, IV: iv, CT: ct, MAC: mac}, nil
}

// decodeBase64 accepts the padded standard base64 the server emits and the
// unpadded form some self-hosted servers use.
func decodeBase64(s string) ([]byte, error) {
	out, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		return out, nil
	}
	out, rawErr := base64.RawStdEncoding.DecodeString(s)
	if rawErr != nil {
		return nil, fmt.Errorf("auth: decode encrypted string: bad base64: %w", err)
	}
	return out, nil
}

// UnmarshalText lets an EncString field in a JSON response decode from its
// wire form.
func (e *EncString) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "" || s == "null" {
		*e = EncString{}
		return nil
	}
	parsed, err := DecodeEncString(s)
	if err != nil {
		return err
	}
	*e = parsed
	return nil
}

// isZero reports whether the EncString carries no ciphertext, i.e. it came
// from a missing or null response field.
func (e EncString) isZero() bool {
	return len(e.IV) == 0 && len(e.CT) == 0 && len(e.MAC) == 0
}

// UnlockUserKey decrypts the token response's Key, which is the account's
// 64-byte userKey wrapped with the stretched master key.
//
// The path is: StretchMasterKey(masterKey) -> 64 bytes split into a 32-byte
// enc key and a 32-byte mac key -> verify EncString type 2's HMAC over
// iv||ct -> AES-256-CBC decrypt -> the userKey (64 bytes split into
// enc||mac when it is later used as a symmetric key).
func UnlockUserKey(masterKey MasterKey, encryptedKey EncString) ([]byte, error) {
	if encryptedKey.Type != EncTypeAesCbc256HmacSha256 {
		return nil, fmt.Errorf("auth: unlock user key: unsupported encrypted key type %d", encryptedKey.Type)
	}
	if len(encryptedKey.MAC) == 0 {
		return nil, errors.New("auth: unlock user key: type 2 encrypted key has no MAC")
	}
	stretched, err := StretchMasterKey(masterKey)
	if err != nil {
		return nil, err
	}
	encKey := stretched[:masterKeyLen]
	macKey := stretched[masterKeyLen:]

	mac := hmac.New(sha256.New, macKey)
	mac.Write(encryptedKey.IV)
	mac.Write(encryptedKey.CT)
	if !hmac.Equal(mac.Sum(nil), encryptedKey.MAC) {
		return nil, errors.New("auth: unlock user key: MAC mismatch")
	}

	plaintext, err := decryptAESCBC(encKey, encryptedKey.IV, encryptedKey.CT)
	if err != nil {
		return nil, fmt.Errorf("auth: unlock user key: %w", err)
	}
	if len(plaintext) != userKeyLen {
		return nil, fmt.Errorf("auth: unlock user key: decrypted key is %d bytes, want %d", len(plaintext), userKeyLen)
	}
	return plaintext, nil
}

// decryptAESCBC decrypts ciphertext with AES-CBC and strips PKCS#7 padding.
func decryptAESCBC(key, iv, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("IV is %d bytes, want %d", len(iv), aes.BlockSize)
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not a positive multiple of the AES block size", len(ciphertext))
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	return unpadPKCS7(plaintext)
}

// unpadPKCS7 removes PKCS#7 padding after validating it.
func unpadPKCS7(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty padded data")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(data) {
		return nil, errors.New("invalid PKCS#7 padding")
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, errors.New("invalid PKCS#7 padding")
		}
	}
	return data[:len(data)-padding], nil
}
