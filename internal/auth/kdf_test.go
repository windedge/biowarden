package auth

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// Official PBKDF2-SHA256 test vectors.
//
// Source: bitwarden-clients, libs/legacy-crypto/src/services/
// web-crypto-function.service.spec.ts:48-51 (the 256-bit sha256 block) and the
// testPbkdf2 helper at :200-245. The Node variant at
// libs/legacy-crypto/src/node/node-crypto-function.service.spec.ts:58 carries
// the same vectors. The inputs are (password, salt=email, iterations=5000,
// keyLen=32); the expected values are the base64 of the derived 32 bytes.
func TestDeriveMasterKeyPBKDF2OfficialVectors(t *testing.T) {
	cases := []struct {
		name     string
		password string
		email    string
		wantB64  string
	}{
		{"regular", "password", "user@example.com", "pj9prw/OHPleXI6bRdmlaD+saJS4awrMiQsQiDjeu2I="},
		{"utf8", "pǻssword", "üser@example.com", "yqvoFXgMRmHR3QPYr5pyR4uVuoHkltv9aHUP63p8n7I="},
		{"unicode", "😀password🙏", "user@example.com", "ZdeOata6xoRpB4DLp8zHhXz5kLmkWtX5pd+TdRH8w8w="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := KdfParams{Type: KdfPBKDF2SHA256, Iterations: 5000, Salt: tc.email}
			key, err := DeriveMasterKey(params, tc.password)
			if err != nil {
				t.Fatalf("DeriveMasterKey: %v", err)
			}
			if got := base64.StdEncoding.EncodeToString(key); got != tc.wantB64 {
				t.Errorf("master key = %s, want %s", got, tc.wantB64)
			}
		})
	}
}

// Argon2id Bitwarden-chain vector.
//
// The official repository keeps its Argon2 implementation inside the wasm SDK
// and has no hardcoded vector, so this one was generated with the official
// chain (salt = sha256(email), memory MiB -> KiB) and cross-checked against an
// independent implementation, OpenSSL 3.6.5:
//
//	openssl kdf -keylen 32 \
//	  -kdfopt pass:password \
//	  -kdfopt hexsalt:b4c9a289323b21a01c3e940f150eb9b8c542587f1abfd8f0e1cc1ffc5e475514 \
//	  -kdfopt iter:3 -kdfopt memcost:65536 -kdfopt lanes:4 ARGON2ID
//
// hexsalt is sha256("user@example.com"); memcost is 64 MiB in KiB. Both
// implementations produce vi0zqrXOU9IuKFtB6Er3uuwUj+FfCUTYS7802bR8xco=.
func TestDeriveMasterKeyArgon2idVector(t *testing.T) {
	params := KdfParams{
		Type:        KdfArgon2id,
		Iterations:  3,
		Memory:      64, // MiB
		Parallelism: 4,
		Salt:        "user@example.com",
	}
	key, err := DeriveMasterKey(params, "password")
	if err != nil {
		t.Fatalf("DeriveMasterKey: %v", err)
	}
	const want = "vi0zqrXOU9IuKFtB6Er3uuwUj+FfCUTYS7802bR8xco="
	if got := base64.StdEncoding.EncodeToString(key); got != want {
		t.Errorf("master key = %s, want %s", got, want)
	}
}

func TestDeriveMasterKeyRejectsBadParams(t *testing.T) {
	cases := []struct {
		name   string
		params KdfParams
	}{
		{"missing salt", KdfParams{Type: KdfPBKDF2SHA256, Iterations: 1}},
		{"zero iterations", KdfParams{Type: KdfPBKDF2SHA256, Salt: "s"}},
		{"unknown type", KdfParams{Type: KdfType(9), Iterations: 1, Salt: "s"}},
		{"argon2 without memory", KdfParams{Type: KdfArgon2id, Iterations: 1, Parallelism: 1, Salt: "s"}},
		{"argon2 without parallelism", KdfParams{Type: KdfArgon2id, Iterations: 1, Memory: 16, Salt: "s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DeriveMasterKey(tc.params, "password"); err == nil {
				t.Fatal("DeriveMasterKey() = nil error, want failure")
			}
		})
	}
}

// MasterPasswordHash vector: PBKDF2-HMAC-SHA256(masterKey, password, 1, 32)
// base64-encoded. masterKey is the output of the official PBKDF2 vector above;
// the expected hash was cross-checked with OpenSSL 3.6.5:
//
//	openssl kdf -keylen 32 -kdfopt digest:SHA256 \
//	  -kdfopt hexpass:a63f69af0fce1cf95e5c8e9b45d9a5683fac6894b86b0acc890b108838debb62 \
//	  -kdfopt salt:password -kdfopt iter:1 PBKDF2
//
// The official client computes the same value at
// libs/legacy-crypto/src/services/legacy-compat-key.service.ts:119-132.
func TestMasterPasswordHashVector(t *testing.T) {
	masterKey := mustB64(t, "pj9prw/OHPleXI6bRdmlaD+saJS4awrMiQsQiDjeu2I=")
	hash, err := MasterPasswordHash(MasterKey(masterKey), "password")
	if err != nil {
		t.Fatalf("MasterPasswordHash: %v", err)
	}
	const want = "9LdZMwDKVbVzf7dJ6SbYXN3QuxnDAI0/x+5GgpFKqZ0="
	if hash != want {
		t.Errorf("MasterPasswordHash() = %s, want %s", hash, want)
	}
}

func TestMasterPasswordHashRejectsWrongKeySize(t *testing.T) {
	if _, err := MasterPasswordHash(MasterKey([]byte("short")), "password"); err == nil {
		t.Fatal("MasterPasswordHash() = nil error, want failure")
	}
}

// StretchMasterKey vector: HKDF-Expand-SHA256(masterKey, "enc", 32) ||
// HKDF-Expand-SHA256(masterKey, "mac", 32). masterKey is again the official
// PBKDF2 vector. Cross-checked with OpenSSL 3.6.5:
//
//	openssl kdf -keylen 32 -kdfopt digest:SHA256 -kdfopt mode:EXPAND_ONLY \
//	  -kdfopt hexkey:a63f69af0fce1cf95e5c8e9b45d9a5683fac6894b86b0acc890b108838debb62 \
//	  -kdfopt hexinfo:656e63 HKDF   # info="enc"
//
// The official split is at
// libs/legacy-crypto/src/services/default-key-generation.service.ts:68-96.
func TestStretchMasterKeyVector(t *testing.T) {
	masterKey := MasterKey(mustB64(t, "pj9prw/OHPleXI6bRdmlaD+saJS4awrMiQsQiDjeu2I="))
	stretched, err := StretchMasterKey(masterKey)
	if err != nil {
		t.Fatalf("StretchMasterKey: %v", err)
	}
	if len(stretched) != stretchedMasterKeyLen {
		t.Fatalf("stretched key is %d bytes, want %d", len(stretched), stretchedMasterKeyLen)
	}
	encWant := mustHex(t, "ae139f1b8961eb7a382920e6818de56f99fd57e7f603cfa447a2d72401463da4")
	macWant := mustHex(t, "4872bd07dffe8a298a9af206719a0e5ca956e4dee71f2b0667eef190de03328b")
	if !bytes.Equal(stretched[:32], encWant) {
		t.Errorf("enc key = %x, want %x", stretched[:32], encWant)
	}
	if !bytes.Equal(stretched[32:], macWant) {
		t.Errorf("mac key = %x, want %x", stretched[32:], macWant)
	}
}

func TestStretchMasterKeyRejectsWrongSize(t *testing.T) {
	if _, err := StretchMasterKey(MasterKey([]byte("short"))); err == nil {
		t.Fatal("StretchMasterKey() = nil error, want failure")
	}
}

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	out, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("bad base64 %q: %v", s, err)
	}
	return out
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var b byte
		for j := 0; j < 2; j++ {
			c := s[i*2+j]
			switch {
			case c >= '0' && c <= '9':
				b = b<<4 | (c - '0')
			case c >= 'a' && c <= 'f':
				b = b<<4 | (c - 'a' + 10)
			default:
				t.Fatalf("bad hex %q", s)
			}
		}
		out[i] = b
	}
	return out
}
