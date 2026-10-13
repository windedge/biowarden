package sdkipc

import (
	"bytes"
	"crypto/ecdh"
	"encoding/hex"
	"errors"
	"testing"
)

// Cross-checked against snow 0.10: initiator ephemeral 0x11*32, responder
// ephemeral 0x22*32. The finish frame and split keys are the exact values the
// Rust side accepts.
const (
	snowStartHex = "040217e617f0b6443928278f96999e69a23a4f2c152bdf6d6cdf66e5b80282d4" +
		"ed194a7debcb97712d2dda3ca85aa8765a56f45fc758599652f2897c65306e5794"
	snowFinishHex = "04d65a93977caa3d1b081852ff57a79e465f1660577304baead505dd3a48589cf350185e895372df6221ea3a137557e4" +
		"73fddb6755f05bd507c3c533fce9c91285548b16c5c22cf3c39251a21d25adcca4"
	snowReceiveKeyHex = "e12fac68aecf624b9b3bb6a4dd96a63e39fff509042d4f17c7857a8fc1d30bd0"
	snowSendKeyHex    = "4a15f70304b93f88acc684be7054cdddfe9603da073403247104fcfcf1edd312"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	return b
}

func snowResponder(t *testing.T) *Responder {
	t.Helper()
	priv, err := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{0x22}, 32))
	if err != nil {
		t.Fatalf("fixed responder key: %v", err)
	}
	_, responder, err := Respond(mustHex(t, snowStartHex), priv)
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	return responder
}

func TestNoiseResponderMatchesSnowVector(t *testing.T) {
	priv, err := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{0x22}, 32))
	if err != nil {
		t.Fatalf("fixed responder key: %v", err)
	}
	finish, responder, err := Respond(mustHex(t, snowStartHex), priv)
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if len(finish) != 81 {
		t.Fatalf("finish is %d bytes, want 65-byte public key + 16-byte tag", len(finish))
	}
	if got := hex.EncodeToString(finish); got != snowFinishHex {
		t.Fatalf("finish =\n %s\nwant\n %s", got, snowFinishHex)
	}
	if got := hex.EncodeToString(responder.receiveKey); got != snowReceiveKeyHex {
		t.Fatalf("receiveKey = %s, want %s", got, snowReceiveKeyHex)
	}
	if got := hex.EncodeToString(responder.sendKey); got != snowSendKeyHex {
		t.Fatalf("sendKey = %s, want %s", got, snowSendKeyHex)
	}
}

// Noise's HKDF is RFC 5869 with an empty info field; case 3 pins the exact
// output so a change to the HMAC chain is caught.
func TestNoiseHKDFMatchesRFC5869Case3(t *testing.T) {
	outputs := hkdf(nil, bytes.Repeat([]byte{0x0b}, 22), 2)
	joined := append(append([]byte{}, outputs[0]...), outputs[1]...)
	got := hex.EncodeToString(joined[:42])
	const want = "8da4e775a563c18f715f802a063c5a31b8a11f5c5ee1879ec3454e5f3c738d2d9d201395faa4b61a96c8"
	if got != want {
		t.Fatalf("HKDF = %s, want %s", got, want)
	}
}

func TestRespondRejectsShortPublicKey(t *testing.T) {
	if _, _, err := Respond([]byte{0x04, 0x01}, nil); !errors.Is(err, ErrNoisePublicKey) {
		t.Fatalf("got %v, want ErrNoisePublicKey", err)
	}
	if _, _, err := Respond(bytes.Repeat([]byte{0x02}, 65), nil); !errors.Is(err, ErrNoisePublicKey) {
		t.Fatalf("uncompressed prefix missing: got %v, want ErrNoisePublicKey", err)
	}
}

func TestTransportRoundTrip(t *testing.T) {
	responder := snowResponder(t)
	server := NewTransport(responder)
	peer := &Transport{sendKey: responder.receiveKey, receiveKey: responder.sendKey}

	plaintext := []byte("hello noise")
	sealed, nonce, err := server.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if nonce != 1 {
		t.Fatalf("first transport nonce = %d, want 1", nonce)
	}
	got, err := peer.Open(sealed, nonce)
	if err != nil {
		t.Fatalf("peer Open: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("peer read %q, want %q", got, plaintext)
	}

	reply, replyNonce, err := peer.Seal([]byte("reply"))
	if err != nil {
		t.Fatalf("peer Seal: %v", err)
	}
	if replyNonce != 1 {
		t.Fatalf("peer first nonce = %d, want 1", replyNonce)
	}
	back, err := server.Open(reply, replyNonce)
	if err != nil {
		t.Fatalf("server Open: %v", err)
	}
	if string(back) != "reply" {
		t.Fatalf("server read %q, want reply", back)
	}
}

func TestTransportTamperFails(t *testing.T) {
	responder := snowResponder(t)
	server := NewTransport(responder)
	peer := &Transport{sendKey: responder.receiveKey, receiveKey: responder.sendKey}

	sealed, nonce, err := server.Seal([]byte("hello"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	sealed[0] ^= 0xff
	if _, err := peer.Open(sealed, nonce); err == nil {
		t.Fatal("Open accepted a tampered ciphertext")
	}
}

func TestTransportRejectsReplay(t *testing.T) {
	responder := snowResponder(t)
	server := NewTransport(responder)
	peer := &Transport{sendKey: responder.receiveKey, receiveKey: responder.sendKey}

	sealed, nonce, err := server.Seal([]byte("hello"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := peer.Open(sealed, nonce); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if _, err := peer.Open(sealed, nonce); !errors.Is(err, ErrNoiseReplay) {
		t.Fatalf("replayed Open = %v, want ErrNoiseReplay", err)
	}
	// Going backwards is rejected for the same reason, even with a fresh nonce.
	if _, err := peer.Open(sealed, 0); !errors.Is(err, ErrNoiseReplay) {
		t.Fatalf("rollback Open = %v, want ErrNoiseReplay", err)
	}
}

func TestTransportAcceptsSkippedNonce(t *testing.T) {
	responder := snowResponder(t)
	server := NewTransport(responder)
	peer := &Transport{sendKey: responder.receiveKey, receiveKey: responder.sendKey}

	if _, _, err := server.Seal([]byte("one")); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	sealed, nonce, err := server.Seal([]byte("two"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	// Drop the first frame: the second (nonce 2) must still open.
	got, err := peer.Open(sealed, nonce)
	if err != nil {
		t.Fatalf("Open skipped nonce: %v", err)
	}
	if string(got) != "two" {
		t.Fatalf("got %q, want two", got)
	}
}
