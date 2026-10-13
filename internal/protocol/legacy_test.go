package protocol

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"testing"
)

// The AES/HMAC vectors below come from WardenBio's ProtocolTests.swift, which
// in turn took them from the pure-Go reference implementation. They pin the
// byte-level AES-256-CBC + PKCS7 and HMAC-SHA256 semantics.
var (
	refKey     = []byte("0123456789abcdef0123456789abcdef")
	refIV      = []byte("0123456789abcdef")
	refVectors = []struct {
		plaintext  string
		ciphertext string // IV || AES-CBC(PKCS7) ciphertext
	}{
		{"hello", "MDEyMzQ1Njc4OWFiY2RlZlEHcOOCQ6s7MWKZEgsFwyA="},
		{"exactly16bytes!!", "MDEyMzQ1Njc4OWFiY2RlZuy0Di5DhlajKVsXtoRUAdpFV/UQ8BWycLtCLCwFWlf5"},
		{"the quick brown fox jumps over the lazy dog 中文字符", "MDEyMzQ1Njc4OWFiY2RlZjQdpkXuARTmPHHCFjG3RkKirvXgbn0FvQibZYycVU0iRlOUx9E/waaHMd64gNItZ4YACBLTNa57im1M7AXQw48="},
	}
)

func TestAESVectorsMatchReference(t *testing.T) {
	for _, v := range refVectors {
		full, err := base64.StdEncoding.DecodeString(v.ciphertext)
		if err != nil {
			t.Fatalf("decode vector for %q: %v", v.plaintext, err)
		}
		iv, body := full[:16], full[16:]

		got, err := decryptAESCBC(refKey, iv, body)
		if err != nil {
			t.Fatalf("decrypt %q: %v", v.plaintext, err)
		}
		if string(got) != v.plaintext {
			t.Fatalf("decrypt %q: got %q", v.plaintext, got)
		}

		enc, err := encryptAESCBC(refKey, refIV, []byte(v.plaintext))
		if err != nil {
			t.Fatalf("encrypt %q: %v", v.plaintext, err)
		}
		if !bytes.Equal(enc, body) {
			t.Fatalf("encrypt %q: got %s, want %s", v.plaintext,
				base64.StdEncoding.EncodeToString(enc), base64.StdEncoding.EncodeToString(body))
		}
	}
}

func TestHMACVector(t *testing.T) {
	got := base64.StdEncoding.EncodeToString(hmacSHA256(refKey, []byte("hello")))
	want := "22i0e2PCOX6seFAlscnCEDrgbCiZgjSng7PrZFPRrXA="
	if got != want {
		t.Fatalf("hmacSHA256(hello) = %s, want %s", got, want)
	}
}

func TestEncStringJSONShape(t *testing.T) {
	enc := EncString{IV: "AAAA", Mac: "MMMM", Data: "BBBB", EncryptionType: 2}
	raw, err := json.Marshal(enc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for k, want := range map[string]any{
		"iv": "AAAA", "data": "BBBB", "mac": "MMMM",
		"encryptionType": float64(2), "encryptedString": "2.AAAA|BBBB|MMMM",
	} {
		if m[k] != want {
			t.Fatalf("field %q = %v, want %v", k, m[k], want)
		}
	}

	// type 0 has no mac and its encryptedString stops after data.
	legacy := EncString{IV: "AAAA", Data: "BBBB", EncryptionType: 0}
	raw, _ = json.Marshal(legacy)
	m = nil
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["mac"]; ok {
		t.Fatalf("type 0 kept an empty mac field: %s", raw)
	}
	if m["encryptedString"] != "0.AAAA|BBBB" {
		t.Fatalf("type 0 encryptedString = %v", m["encryptedString"])
	}

	// Round-trip: an inbound EncString (no encryptedString on the wire) must
	// still marshal it again, so the trap cannot be reintroduced by decoding.
	var decoded EncString
	if err := json.Unmarshal([]byte(`{"iv":"a","mac":"m","data":"d","encryptionType":2}`), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.EncryptedString() != "2.a|d|m" {
		t.Fatalf("recomputed encryptedString = %q", decoded.EncryptedString())
	}
}

func TestSessionCrossDecrypt(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, TransportKeyLength)
	host, err := NewSessionWithKey(key)
	if err != nil {
		t.Fatalf("host session: %v", err)
	}
	extension, err := NewSessionWithKey(key)
	if err != nil {
		t.Fatalf("extension session: %v", err)
	}

	plaintext := []byte(`{"command":"getBiometricsStatusForUser","messageId":1}`)
	enc, err := extension.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc.EncryptionType != EncTypeAesCbc256HmacSha256 || enc.Mac == "" {
		t.Fatalf("encrypt produced type %d mac %q", enc.EncryptionType, enc.Mac)
	}
	got, err := host.Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip changed payload: got %q", got)
	}
}

func TestSessionRejectsTampering(t *testing.T) {
	session, err := NewSession()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	enc, err := session.Encrypt([]byte("hello"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	t.Run("ciphertext", func(t *testing.T) {
		body, _ := base64.StdEncoding.DecodeString(enc.Data)
		body[0] ^= 0x01
		bad := enc
		bad.Data = base64.StdEncoding.EncodeToString(body)
		if _, err := session.Decrypt(bad); !errors.Is(err, ErrMACMismatch) {
			t.Fatalf("got %v, want ErrMACMismatch", err)
		}
	})
	t.Run("iv", func(t *testing.T) {
		iv, _ := base64.StdEncoding.DecodeString(enc.IV)
		iv[0] ^= 0x01
		bad := enc
		bad.IV = base64.StdEncoding.EncodeToString(iv)
		if _, err := session.Decrypt(bad); !errors.Is(err, ErrMACMismatch) {
			t.Fatalf("got %v, want ErrMACMismatch", err)
		}
	})
	t.Run("mac", func(t *testing.T) {
		mac, _ := base64.StdEncoding.DecodeString(enc.Mac)
		mac[0] ^= 0x01
		bad := enc
		bad.Mac = base64.StdEncoding.EncodeToString(mac)
		if _, err := session.Decrypt(bad); !errors.Is(err, ErrMACMismatch) {
			t.Fatalf("got %v, want ErrMACMismatch", err)
		}
	})
	t.Run("missing mac", func(t *testing.T) {
		bad := enc
		bad.Mac = ""
		if _, err := session.Decrypt(bad); !errors.Is(err, ErrMissingMAC) {
			t.Fatalf("got %v, want ErrMissingMAC", err)
		}
	})
}

func TestPKCS7HandlesEmptyPlaintext(t *testing.T) {
	ct, err := encryptAESCBC(refKey, refIV, nil)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(ct) != 16 {
		t.Fatalf("empty plaintext padded to %d bytes, want 16", len(ct))
	}
	got, err := decryptAESCBC(refKey, refIV, ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d bytes, want empty", len(got))
	}
}

// The SPKI key and PEM private key are the pair the Swift suite uses; the OAEP
// ciphertext was produced by the Go reference implementation.
const refSPKI = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAtvQNbQJRkklL0MGk0oXdQkBOUvXttdraR8D37FO3i1He8qN5IBpBrfIpXflrCJblo3/3OfciZ0bPeNJ8QpS66pkcw57nYbI6+AgiXXFND3iWfMFHVZ2G8GCGfssBKqoIUCupgj+9XJQhCXSN4ti+ZdETLj4WYdcA2kaVArrXDtZk69JqfFZt0S5aier3Vd68zfO4XSugoySsb7KqGh7NA4r5D1HUwl+TMWmA2PGx5fC3m+fbxZPpdPdRSN+sJsUDiblu63PG/mmT29K4DplZdvR+4g5tXDFXkgx+xaLCaNRlhWI/M1AlnPEpMuzK8UCpksOEYEKtRKZV2y1/J456CQIDAQAB"

const refPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAtvQNbQJRkklL0MGk0oXdQkBOUvXttdraR8D37FO3i1He8qN5
IBpBrfIpXflrCJblo3/3OfciZ0bPeNJ8QpS66pkcw57nYbI6+AgiXXFND3iWfMFH
VZ2G8GCGfssBKqoIUCupgj+9XJQhCXSN4ti+ZdETLj4WYdcA2kaVArrXDtZk69Jq
fFZt0S5aier3Vd68zfO4XSugoySsb7KqGh7NA4r5D1HUwl+TMWmA2PGx5fC3m+fb
xZPpdPdRSN+sJsUDiblu63PG/mmT29K4DplZdvR+4g5tXDFXkgx+xaLCaNRlhWI/
M1AlnPEpMuzK8UCpksOEYEKtRKZV2y1/J456CQIDAQABAoIBABdQZ2uWSY3SQFKU
cqwPOgBo0HJa+5Vak8lkClP34SJkZvxVn6hbIDaJ7wKWN7ELBa19r6QX3a76g5La
g9I6QM2HgHZfSA4Xl9MmujZdK1xG2iqAuNXsspbsPgdnWDk3HMMWpU2/UKK9D660
RyZ6x1yTNZYFfHWwX9/w8ak85Q3zRgTt20ZVDhqBhN/GLB7nT/0+x2qF4QQAS1sV
IiCDV5Gtt7nZvWUmAGYBZgdSj7ATpw1BwhyMwRjYQj4/UK8qNTsT8DELpOhhKUP1
lCj81kDNXUMYW/Lji3BfRNfWmRdhSLI/9LSSkMcQ6+h8qNP7/ZZ88xhtk6x0fa0+
Rys+EHcCgYEA4z5oufUTRvwfMZeFU+BfoCOBYQkjHyPXM1CvbNdZh7DLa/DTXBI3
PjG2zqcpKfsG7zAHj1oVVuf8Gp56c/rmCogSSGD9QQrHe9UkvlNRQJafYzQCJagy
kMWw1fnBlQe9a9fqSH9DeJhms4QlWbX83EmVNy33XoXtYHbPF53ZmH8CgYEAzhrZ
NwNtn6Ze5H1FEp7Rrfm1ISO2+4f/pit1SxijhOW0w6YmlMztwJtI0ac2f7azTuo8
AGP9DuxFaCesUlGzR5+C+LcB0A4VsqcAH1NeEXBKn22wGsnhvOpI1OsaBpadMBnu
Ig+1pAhGQdXvuevNUz2rYRgYgx1vjjTPzv3j6XcCgYBovluHW2+HSK8YLL9H6iQp
rDP2hj+qGtsWzBoQETMzWEprdpX12m+cO04H8wUGVK7dkUXqzZsIc+XAX0wSKZPf
Rtkmg444bL+GVLibAcxx+Pt+vno+4UhtcrIP7w9LokWtb9iNkhuHerfcBw0wTLJA
16nvxUBAUXsY+6p5OEkUAQKBgQCoxBNZmNULBZBuSKVmueW4L+DBYh1TmekciDlj
ZKmBCRcbndG5xpRoQr897U4TBeeNlv/K0hVFQvMSdmoEfiHvZA462qABXYcm4tiW
zg0Y+nogUhEB3o2Zw74fmuOUwX+1A4abHYH+70eYYTYLZ6qZnsXWm4R22IOgTl5b
gEo/wQKBgFfL8768ej5/jyjAibpqeu5R/+Yxh5LUdTGhAGsvWZYgWNR2L6Cpl9wh
JfgEmUGj2JK9gKI5ttvM07SMkRtQZjAKsfM8pNL/v/iupkSKOuRxJMjakyBkhPRo
PdYrEMKCmZjXAKJUUgbLh9X84g78oaAhxZauXgwR0SRyLpGzTzym
-----END RSA PRIVATE KEY-----`

const refOAEPCT = "YkjM5YHOor2V7PtvcwB6L82yghBDtkVFwtSORptTJyO+0YSa7jb9jHe0D2K1sbVU10IAn+Ql+aa/1Z4hghb1GmWtVsqMW7Z+7nqJ8vA54y1oc55zd95UMcgJqQdoXWQKcSxT42Q3J8Tn2GKq+sK7tV+C8TGkOEb+xZU/w6LPXyaAtl7ZAHy6W6UVqbIhEafYjSu9x6yG/Yi/t2yhNy+OV+SaMBFJBZumiZ4K72MHPjU0cQeAjjB7GgCu1bpuOkUMCjcJ3W4E09h5cgkU3GoeVYQI5MPqCkJdp2lbLx2ICeiMTccHeg6n8RVyfECxuyKzkhqVqmPAzS9ayA8kaNRy8g=="

func TestParseSPKIPublicKey(t *testing.T) {
	pub, err := parseSPKIPublicKey(refSPKI)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if pub.N.BitLen() != 2048 {
		t.Fatalf("RSA size = %d bits, want 2048", pub.N.BitLen())
	}
}

func TestOAEPDecryptReferenceCiphertext(t *testing.T) {
	block, _ := pem.Decode([]byte(refPEM))
	if block == nil {
		t.Fatal("PEM did not decode")
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse PKCS1: %v", err)
	}
	ct, err := base64.StdEncoding.DecodeString(refOAEPCT)
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	plain, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, priv, ct, nil)
	if err != nil {
		t.Fatalf("OAEP decrypt: %v", err)
	}
	if string(plain) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("plaintext = %q", plain)
	}
}

// TestSetupEncryptionRoundTrip walks the whole handshake: no external bytes,
// just the two sides of the protocol.
func TestSetupEncryptionRoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	spki, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal SPKI: %v", err)
	}
	appID := "chrome-extension://nngceckbapebfimnlniiiahkandclblb"
	raw := mustJSON(t, IncomingMessage{
		AppID:   appID,
		Message: mustJSON(t, SetupRequest{Command: "setupEncryption", UserID: "user-1", PublicKey: base64.StdEncoding.EncodeToString(spki)}),
	})

	host, err := NewSession()
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	req, err := host.Read(raw)
	if err != nil {
		t.Fatalf("read setup: %v", err)
	}
	if req.Kind != RequestSetup || req.AppID != appID || req.Setup.UserID != "user-1" {
		t.Fatalf("decoded request = %+v", req)
	}
	if host.HandshakeComplete() {
		t.Fatal("handshake marked complete before setupEncryption")
	}

	reply, err := host.SetupEncryption(req)
	if err != nil {
		t.Fatalf("setupEncryption: %v", err)
	}
	if reply.Command != "setupEncryption" || reply.AppID != appID {
		t.Fatalf("reply envelope = %+v", reply)
	}
	if reply.MessageID == nil || *reply.MessageID != -1 {
		t.Fatalf("reply messageId = %v, want -1", reply.MessageID)
	}
	if !host.HandshakeComplete() {
		t.Fatal("handshake not marked complete after setupEncryption")
	}

	secret, err := base64.StdEncoding.DecodeString(reply.SharedSecret)
	if err != nil {
		t.Fatalf("decode sharedSecret: %v", err)
	}
	key, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, priv, secret, nil)
	if err != nil {
		t.Fatalf("unwrap transport key: %v", err)
	}
	if len(key) != TransportKeyLength {
		t.Fatalf("transport key is %d bytes, want %d", len(key), TransportKeyLength)
	}

	// The extension side now owns the same key and must decrypt an encrypted reply.
	extension, err := NewSessionWithKey(key)
	if err != nil {
		t.Fatalf("extension session: %v", err)
	}
	out, err := host.Reply(req, StatusResponse{
		Command: "setupEncryption", MessageID: req.MessageID(), Response: BiometricsAvailable, Timestamp: 1,
	})
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	if out.AppID != appID {
		t.Fatalf("encrypted reply appId = %q, want %q", out.AppID, appID)
	}
	if out.Message == nil || out.Message.EncryptedString() == "" {
		t.Fatalf("encrypted reply missing message: %+v", out)
	}
	plaintext, err := extension.Decrypt(*out.Message)
	if err != nil {
		t.Fatalf("extension decrypt: %v", err)
	}
	var status StatusResponse
	if err := json.Unmarshal(plaintext, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.MessageID != -1 || status.Command != "setupEncryption" {
		t.Fatalf("decoded status = %+v", status)
	}
}

func TestReadDecryptsPayloadRequest(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, TransportKeyLength)
	host, _ := NewSessionWithKey(key)
	extension, _ := NewSessionWithKey(key)

	messageID := 42
	payload := PayloadRequest{Command: "unlockWithBiometricsForUser", UserID: "user-1", MessageID: &messageID, Timestamp: 1700000000000}
	enc, err := extension.Encrypt(mustJSON(t, payload))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	raw := mustJSON(t, IncomingMessage{AppID: "ext-id", Message: mustJSON(t, enc)})

	req, err := host.Read(raw)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if req.Kind != RequestPayload || req.AppID != "ext-id" {
		t.Fatalf("decoded request = %+v", req)
	}
	if req.Payload.Command != "unlockWithBiometricsForUser" || req.Payload.UserID != "user-1" {
		t.Fatalf("payload = %+v", req.Payload)
	}
	if req.MessageID() != 42 {
		t.Fatalf("messageId = %d, want 42", req.MessageID())
	}
}

func TestRequestMessageIDDefaultsToMinusOne(t *testing.T) {
	req := Request{Kind: RequestPayload, Payload: &PayloadRequest{Command: "x"}}
	if req.MessageID() != -1 {
		t.Fatalf("messageId = %d, want -1", req.MessageID())
	}
}

func TestReplyEnvelopeShape(t *testing.T) {
	key := bytes.Repeat([]byte{0x22}, TransportKeyLength)
	host, _ := NewSessionWithKey(key)
	messageID := 7
	req := Request{Kind: RequestPayload, AppID: "ext-id", Payload: &PayloadRequest{Command: "getBiometricsStatus", MessageID: &messageID}}

	out, err := host.Reply(req, StatusResponse{Command: "getBiometricsStatus", MessageID: req.MessageID(), Response: BiometricsAvailable, Timestamp: 9})
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["appId"] != "ext-id" {
		t.Fatalf("appId = %v, want ext-id", m["appId"])
	}
	if _, ok := m["sharedSecret"]; ok {
		t.Fatalf("encrypted reply carried sharedSecret: %s", raw)
	}
	message, ok := m["message"].(map[string]any)
	if !ok {
		t.Fatalf("message missing: %s", raw)
	}
	for _, field := range []string{"iv", "data", "mac", "encryptionType", "encryptedString"} {
		if _, ok := message[field]; !ok {
			t.Fatalf("message missing %q: %s", field, raw)
		}
	}
	if message["encryptionType"] != float64(2) {
		t.Fatalf("encryptionType = %v, want 2", message["encryptionType"])
	}
	encString := message["encryptedString"].(string)
	wantPrefix := "2." + message["iv"].(string) + "|" + message["data"].(string) + "|" + message["mac"].(string)
	if encString != wantPrefix {
		t.Fatalf("encryptedString = %q, want %q", encString, wantPrefix)
	}

	// The decoded payload echoes the request's messageId so the extension can
	// match its callback.
	plaintext, err := host.Decrypt(*out.Message)
	if err != nil {
		t.Fatalf("decrypt reply: %v", err)
	}
	var status StatusResponse
	if err := json.Unmarshal(plaintext, &status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.MessageID != 7 {
		t.Fatalf("payload messageId = %d, want 7", status.MessageID)
	}
}

func TestOutgoingMessageOmitsNilFields(t *testing.T) {
	raw, err := json.Marshal(Connected("host-id"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["command"] != "connected" || m["appId"] != "host-id" {
		t.Fatalf("connected = %s", raw)
	}
	for _, absent := range []string{"message", "sharedSecret", "messageId"} {
		if _, ok := m[absent]; ok {
			t.Fatalf("connected carried %q: %s", absent, raw)
		}
	}
}

func TestInvalidSessionKeyLength(t *testing.T) {
	if _, err := NewSessionWithKey(make([]byte, 32)); !errors.Is(err, ErrTransportKeyLength) {
		t.Fatalf("got %v, want ErrTransportKeyLength", err)
	}
	if _, err := NewSessionWithKey(make([]byte, 64)); err != nil {
		t.Fatalf("64-byte key rejected: %v", err)
	}
}

func TestReadRejectsMalformedEnvelope(t *testing.T) {
	host, _ := NewSession()
	for _, raw := range []string{`not json`, `{"appId":"x"}`, `{"appId":"x","message":{}}`} {
		if _, err := host.Read([]byte(raw)); err == nil {
			t.Fatalf("Read(%q) succeeded, want error", raw)
		}
	}
}

func TestBadEncStringErrors(t *testing.T) {
	host, _ := NewSession()
	cases := []struct {
		name string
		enc  EncString
	}{
		{"bad iv base64", EncString{IV: "!!!", Data: "AAAA", EncryptionType: 2}},
		{"bad data base64", EncString{IV: "AAAA", Data: "!!!", EncryptionType: 2}},
		{"bad iv length", EncString{IV: base64.StdEncoding.EncodeToString([]byte("short")), Data: "AAAA", EncryptionType: 0}},
		{"ciphertext not block multiple", EncString{IV: base64.StdEncoding.EncodeToString(make([]byte, 16)), Data: base64.StdEncoding.EncodeToString(make([]byte, 15)), EncryptionType: 0}},
		{"type 2 without mac", EncString{IV: base64.StdEncoding.EncodeToString(make([]byte, 16)), Data: base64.StdEncoding.EncodeToString(make([]byte, 16)), EncryptionType: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := host.Decrypt(tc.enc); err == nil {
				t.Fatal("decrypt succeeded, want error")
			}
		})
	}
}

// TestSetupEncryptionPlaintextShape keeps the request probe in sync with the
// extension's payload: it must not be mistaken for an encrypted message.
func TestSetupEncryptionPlaintextShape(t *testing.T) {
	host, _ := NewSession()
	raw := `{"appId":"ext","message":{"command":"setupEncryption","publicKey":"AAAA"}}`
	req, err := host.Read([]byte(raw))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if req.Kind != RequestSetup || req.Setup.PublicKey != "AAAA" {
		t.Fatalf("request = %+v", req)
	}
}

func TestSetupEncryptionRejectsBadRequests(t *testing.T) {
	host, _ := NewSession()
	if _, err := host.SetupEncryption(Request{}); !errors.Is(err, ErrNotSetup) {
		t.Fatalf("got %v, want ErrNotSetup", err)
	}
	missing := Request{Kind: RequestSetup, Setup: &SetupRequest{Command: "setupEncryption"}}
	if _, err := host.SetupEncryption(missing); !errors.Is(err, ErrMissingPublicKey) {
		t.Fatalf("got %v, want ErrMissingPublicKey", err)
	}
	bad := Request{Kind: RequestSetup, Setup: &SetupRequest{Command: "setupEncryption", PublicKey: "not base64!"}}
	if _, err := host.SetupEncryption(bad); err == nil {
		t.Fatal("malformed public key accepted")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
