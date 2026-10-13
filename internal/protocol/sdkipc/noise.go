package sdkipc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// NoiseCipherSuite is the only suite the extension negotiates.
const NoiseCipherSuite = "Noise_NN_P256_AESGCM_SHA256"

var (
	ErrNoisePublicKey = errors.New("sdkipc: invalid P-256 public key")
	ErrNoiseHandshake = errors.New("sdkipc: noise handshake failed")
	ErrNoiseReplay    = errors.New("sdkipc: noise nonce is not strictly increasing")
)

// Responder holds the post-handshake symmetric state of the responder side.
type Responder struct {
	receiveKey []byte // i2r
	sendKey    []byte // r2i
}

// ReceiveKey exposes i2r where tests need to build the peer transport.
func (r *Responder) ReceiveKey() []byte { return r.receiveKey }

// SendKey exposes r2i where tests need to build the peer transport.
func (r *Responder) SendKey() []byte { return r.sendKey }

// Respond consumes a HandshakeStart noise_frame and returns the HandshakeFinish
// noise_frame. A nil ephemeral generates a fresh key; callers pass a fixed one
// to reproduce the snow cross-check vector.
func Respond(startFrame []byte, ephemeral *ecdh.PrivateKey) ([]byte, *Responder, error) {
	// InitializeSymmetric: h0 and ck are both the protocol name right-padded
	// with zeros to 32 bytes, and h is that value MixHash'ed once for the empty
	// prologue. Starting from sha256(padded) without the second step (or using
	// the unpadded name) shifts every later AEAD AAD and the handshake fails.
	padded := make([]byte, 32)
	copy(padded, NoiseCipherSuite)
	h := sha256.Sum256(padded)
	ck := padded

	// <- e: msg1 carries the initiator's 65-byte uncompressed ephemeral key.
	if len(startFrame) < 65 || startFrame[0] != 0x04 {
		return nil, nil, ErrNoisePublicKey
	}
	initiatorPub, err := ecdh.P256().NewPublicKey(startFrame[:65])
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrNoisePublicKey, err)
	}
	h = mixHash(h, startFrame[:65])
	// msg1's payload has no key yet, so it travels in the clear, but
	// EncryptAndHash still mixes it into h. For NN it is always empty, which is
	// still one more MixHash: sha256(h). Skipping it breaks the msg2 tag.
	h = mixHash(h, startFrame[65:])

	if ephemeral == nil {
		ephemeral, err = ecdh.P256().GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: generate ephemeral: %v", ErrNoiseHandshake, err)
		}
	}
	responderPub := ephemeral.PublicKey().Bytes()
	h = mixHash(h, responderPub)

	// -> ee: MixKey updates the chaining key as well as the next AEAD key. Using
	// the pre-MixKey ck for Split yields wrong transport keys.
	dh, err := ephemeral.ECDH(initiatorPub)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: ecdh: %v", ErrNoiseHandshake, err)
	}
	ck, tempKey := hkdf2(ck, dh)

	// EncryptAndHash(empty): nonce 0 and the current handshake hash as AAD.
	sealed, err := aeadSeal(tempKey, 0, h[:], nil)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrNoiseHandshake, err)
	}
	h = mixHash(h, sealed)

	// Split uses the post-MixKey ck with an empty input; split[0] is i2r.
	split := hkdf(ck, nil, 2)
	finish := make([]byte, 0, len(responderPub)+len(sealed))
	finish = append(finish, responderPub...)
	finish = append(finish, sealed...)
	return finish, &Responder{receiveKey: split[0], sendKey: split[1]}, nil
}

// Transport seals and opens post-handshake frames. The nonce is implicit in the
// Noise spec but the SDK IPC wire format carries it explicitly next to each
// payload.
type Transport struct {
	sendKey      []byte
	receiveKey   []byte
	sendNonce    uint64
	receiveNonce uint64
}

// NewTransport builds the responder-side transport from a completed handshake.
func NewTransport(r *Responder) *Transport {
	return &Transport{sendKey: r.sendKey, receiveKey: r.receiveKey}
}

// Seal encrypts a plaintext and returns it with the nonce the caller must put
// on the wire. snow starts transport nonces at 1.
func (t *Transport) Seal(plaintext []byte) ([]byte, uint64, error) {
	t.sendNonce++
	sealed, err := aeadSeal(t.sendKey, t.sendNonce, nil, plaintext)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrNoiseHandshake, err)
	}
	return sealed, t.sendNonce, nil
}

// Open decrypts a frame. The peer may skip nonces, but a nonce at or below the
// last accepted one is rejected as a replay, matching the reference.
func (t *Transport) Open(sealed []byte, nonce uint64) ([]byte, error) {
	if nonce <= t.receiveNonce {
		return nil, fmt.Errorf("%w: got %d, last accepted %d", ErrNoiseReplay, nonce, t.receiveNonce)
	}
	plaintext, err := aeadOpen(t.receiveKey, nonce, nil, sealed)
	if err != nil {
		return nil, err
	}
	t.receiveNonce = nonce
	return plaintext, nil
}

func mixHash(h [32]byte, data []byte) [32]byte {
	hash := sha256.New()
	hash.Write(h[:])
	hash.Write(data)
	var out [32]byte
	copy(out[:], hash.Sum(nil))
	return out
}

// hkdf is the Noise HKDF: HMAC-SHA256 with the chaining key as salt and an
// empty info field (i.e. RFC 5869 with info clear).
func hkdf(chainingKey, input []byte, outputCount int) [][]byte {
	temp := hmacSHA256(chainingKey, input)
	outputs := make([][]byte, 0, outputCount)
	var prev []byte
	for i := 1; i <= outputCount; i++ {
		mac := hmac.New(sha256.New, temp)
		mac.Write(prev)
		mac.Write([]byte{byte(i)})
		out := mac.Sum(nil)
		outputs = append(outputs, out)
		prev = out
	}
	return outputs
}

func hkdf2(chainingKey, input []byte) ([]byte, []byte) {
	out := hkdf(chainingKey, input, 2)
	return out[0], out[1]
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// nonce is 4 zero bytes followed by the counter as a big-endian u64.
func nonceFor(counter uint64) []byte {
	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce[4:], counter)
	return nonce
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func aeadSeal(key []byte, counter uint64, aad, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nil, nonceFor(counter), plaintext, aad), nil
}

func aeadOpen(key []byte, counter uint64, aad, sealed []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.Overhead() {
		return nil, fmt.Errorf("%w: ciphertext shorter than the AEAD tag", ErrNoiseHandshake)
	}
	return gcm.Open(nil, nonceFor(counter), sealed, aad)
}
