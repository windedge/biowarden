package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestDecodeEncString(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, e EncString)
	}{
		{
			name:  "type 2",
			input: "2.aXY=|Y3Q=|bWFj",
			check: func(t *testing.T, e EncString) {
				if e.Type != EncTypeAesCbc256HmacSha256 {
					t.Errorf("Type = %d, want 2", e.Type)
				}
				if string(e.IV) != "iv" || string(e.CT) != "ct" || string(e.MAC) != "mac" {
					t.Errorf("decoded = %q %q %q", e.IV, e.CT, e.MAC)
				}
			},
		},
		{
			name:  "type 0 has no mac",
			input: "0.aXY=|Y3Q=",
			check: func(t *testing.T, e EncString) {
				if e.Type != EncTypeAesCbc256 {
					t.Errorf("Type = %d, want 0", e.Type)
				}
				if e.MAC != nil {
					t.Errorf("MAC = %x, want nil", e.MAC)
				}
			},
		},
		{
			// Structural sample carried verbatim from the official spec:
			// libs/legacy-crypto/src/models/enc-string.spec.ts:80
			name:  "official sample",
			input: "2.QmFzZTY0UGFydA==|QmFzZTY0UGFydA==|QmFzZTY0UGFydA==",
			check: func(t *testing.T, e EncString) {
				if string(e.IV) != "Base64Part" || string(e.CT) != "Base64Part" || string(e.MAC) != "Base64Part" {
					t.Errorf("decoded = %q %q %q", e.IV, e.CT, e.MAC)
				}
			},
		},
		{name: "empty", input: "", wantErr: true},
		{name: "null", input: "null", wantErr: true},
		{name: "no type prefix", input: "aXY=|Y3Q=|bWFj", wantErr: true},
		{name: "bad type prefix", input: "x.aXY=|Y3Q=|bWFj", wantErr: true},
		{name: "type 2 missing mac", input: "2.aXY=|Y3Q=", wantErr: true},
		{name: "type 0 extra part", input: "0.aXY=|Y3Q=|bWFj", wantErr: true},
		{name: "unsupported rsa type", input: "3.Y3Q=", wantErr: true},
		{name: "bad base64", input: "2.!!!!|Y3Q=|bWFj", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, err := DecodeEncString(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("DecodeEncString(%q) = nil error, want failure", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeEncString(%q): %v", tc.input, err)
			}
			tc.check(t, e)
		})
	}
}

// UnlockUserKey vector. masterKey is the official PBKDF2 vector output; the
// encrypted key is a type-2 EncString wrapping the 64-byte userKey 0x00..0x3f.
// The ciphertext and MAC were produced independently with OpenSSL 3.6.5 from
// the stretched master key that StretchMasterKey computes (see
// TestStretchMasterKeyVector for the enc/mac split):
//
//	openssl enc -aes-256-cbc -K ae139f1b...463da4 -iv 10101010101010101010101010101010
//	openssl dgst -sha256 -mac HMAC -macopt hexkey:4872bd07...03328b -binary
//
// Both the AES-CBC ciphertext and the HMAC-SHA256 tag matched, so this vector
// checks the whole unlock path against an implementation other than
// golang.org/x/crypto.
func TestUnlockUserKeyVector(t *testing.T) {
	masterKey := MasterKey(mustB64(t, "pj9prw/OHPleXI6bRdmlaD+saJS4awrMiQsQiDjeu2I="))
	const encString = "2.EBAQEBAQEBAQEBAQEBAQEA==|lpnnGSmUGNu7ZCJ5Uolx7d9CVUBvEbcJaGOmPYHCx+il7Q6R8TAI/afRlpqIGZtpqu/pArTL27HDSsLvaof54O6vbgQ0GJV4LLaTRQwxr1M=|ap0f2828naJvmZGRUCidFoReX3tUbRxcPCn1bNmi1O4="
	encrypted, err := DecodeEncString(encString)
	if err != nil {
		t.Fatalf("DecodeEncString: %v", err)
	}
	userKey, err := UnlockUserKey(masterKey, encrypted)
	if err != nil {
		t.Fatalf("UnlockUserKey: %v", err)
	}
	want := make([]byte, 64)
	for i := range want {
		want[i] = byte(i)
	}
	if !bytes.Equal(userKey, want) {
		t.Errorf("userKey = %x, want %x", userKey, want)
	}
}

func TestUnlockUserKeyRejectsTamperedMAC(t *testing.T) {
	masterKey := MasterKey(mustB64(t, "pj9prw/OHPleXI6bRdmlaD+saJS4awrMiQsQiDjeu2I="))
	encrypted, err := DecodeEncString("2.EBAQEBAQEBAQEBAQEBAQEA==|lpnnGSmUGNu7ZCJ5Uolx7d9CVUBvEbcJaGOmPYHCx+il7Q6R8TAI/afRlpqIGZtpqu/pArTL27HDSsLvaof54O6vbgQ0GJV4LLaTRQwxr1M=|ap0f2828naJvmZGRUCidFoReX3tUbRxcPCn1bNmi1O4=")
	if err != nil {
		t.Fatalf("DecodeEncString: %v", err)
	}
	encrypted.MAC[0] ^= 0xff
	if _, err := UnlockUserKey(masterKey, encrypted); err == nil {
		t.Fatal("UnlockUserKey() = nil error for a tampered MAC")
	}
}

func TestUnlockUserKeyRejectsWrongType(t *testing.T) {
	if _, err := UnlockUserKey(MasterKey(make([]byte, 32)), EncString{Type: EncTypeAesCbc256}); err == nil {
		t.Fatal("UnlockUserKey() = nil error for a type-0 key")
	}
}

func TestEncStringUnmarshalTextNull(t *testing.T) {
	var e EncString
	if err := e.UnmarshalText([]byte("null")); err != nil {
		t.Fatalf("UnmarshalText(null): %v", err)
	}
	if !e.isZero() {
		t.Errorf("EncString after null = %+v, want zero", e)
	}
}

func TestTokenResponseDecodesWrappedKey(t *testing.T) {
	body := []byte(`{"access_token":"a","Key":"2.EBAQEBAQEBAQEBAQEBAQEA==|lpnnGSmUGNu7ZCJ5Uolx7d9CVUBvEbcJaGOmPYHCx+il7Q6R8TAI/afRlpqIGZtpqu/pArTL27HDSsLvaof54O6vbgQ0GJV4LLaTRQwxr1M=|ap0f2828naJvmZGRUCidFoReX3tUbRxcPCn1bNmi1O4=","PrivateKey":null}`)
	var resp TokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal TokenResponse: %v", err)
	}
	if resp.Key.Type != EncTypeAesCbc256HmacSha256 {
		t.Fatalf("Key.Type = %d, want 2", resp.Key.Type)
	}
	if !resp.PrivateKey.isZero() {
		t.Errorf("PrivateKey = %+v, want zero for JSON null", resp.PrivateKey)
	}
	if resp.AccessToken != "a" {
		t.Errorf("AccessToken = %q", resp.AccessToken)
	}
}

func TestDecodeEncStringUnpaddedBase64(t *testing.T) {
	// "iv" and "ct" are two-char unpadded base64; some self-hosted servers
	// omit padding. The decoder must accept both forms.
	if _, err := DecodeEncString("2.aXY=|Y3Q=|bWFj"); err != nil {
		t.Fatalf("DecodeEncString unpadded: %v", err)
	}
	raw := base64.RawStdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})
	if _, err := DecodeEncString("2." + raw + "|" + raw + "|" + raw); err != nil {
		t.Fatalf("DecodeEncString raw: %v", err)
	}
}
