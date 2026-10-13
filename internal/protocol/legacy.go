package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// TransportKeyLength is the size of the session key handed to the extension
// during setupEncryption: 32 bytes AES-256-CBC plus 32 bytes HMAC-SHA256. A
// 32-byte key (AES only) would decrypt but never pass the extension's MAC check.
const TransportKeyLength = 64

const (
	encKeyLength = 32
	macKeyLength = 32

	// EncTypeAesCbc256HmacSha256 is Bitwarden's EncString type 2.
	EncTypeAesCbc256HmacSha256 = 2
)

// BiometricsStatus values, as the extension's libs/key-management enum.
const (
	BiometricsAvailable           = 0
	BiometricsUnlockNeeded        = 1
	BiometricsHardwareUnavailable = 2
	BiometricsDesktopDisconnected = 6
	BiometricsNotEnabledLocally   = 7
	BiometricsNotEnabledInDesktop = 8
)

var (
	ErrTransportKeyLength = errors.New("protocol: transport key must be 64 bytes")
	ErrNotRSAPublicKey    = errors.New("protocol: setupEncryption publicKey is not an RSA key")
	ErrMissingPublicKey   = errors.New("protocol: setupEncryption request is missing publicKey")
	ErrNotSetup           = errors.New("protocol: not a setupEncryption request")
	ErrEnvelope           = errors.New("protocol: malformed native messaging envelope")
	ErrMACMismatch        = errors.New("protocol: HMAC verification failed")
	ErrMissingMAC         = errors.New("protocol: type 2 message is missing its mac")
	ErrBadIV              = errors.New("protocol: iv must be 16 bytes")
	ErrBlockSize          = errors.New("protocol: data is not a multiple of the AES block size")
	ErrPadding            = errors.New("protocol: invalid PKCS7 padding")
)

// EncString is the wire form of one encrypted string. Marshalling always emits
// encryptedString next to iv/data/mac: the extension feeds that string straight
// to the SDK decryptor, and a reply without it leaves the extension spinning.
type EncString struct {
	IV             string
	Mac            string
	Data           string
	EncryptionType int
}

// EncryptedString renders the `<type>.<iv>|<data>|<mac>` form the extension
// decoder consumes. The mac part is omitted for type 0 (no MAC).
func (e EncString) EncryptedString() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d.%s|%s", e.EncryptionType, e.IV, e.Data)
	if e.Mac != "" {
		b.WriteByte('|')
		b.WriteString(e.Mac)
	}
	return b.String()
}

func (e EncString) MarshalJSON() ([]byte, error) {
	wire := struct {
		IV              string `json:"iv"`
		Mac             string `json:"mac,omitempty"`
		Data            string `json:"data"`
		EncryptionType  int    `json:"encryptionType"`
		EncryptedString string `json:"encryptedString"`
	}{e.IV, e.Mac, e.Data, e.EncryptionType, e.EncryptedString()}
	return json.Marshal(wire)
}

func (e *EncString) UnmarshalJSON(data []byte) error {
	var wire struct {
		IV             string `json:"iv"`
		Mac            string `json:"mac"`
		Data           string `json:"data"`
		EncryptionType int    `json:"encryptionType"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	e.IV, e.Mac, e.Data, e.EncryptionType = wire.IV, wire.Mac, wire.Data, wire.EncryptionType
	return nil
}

// IncomingMessage is the top-level frame from the extension.
type IncomingMessage struct {
	AppID   string          `json:"appId"`
	Message json.RawMessage `json:"message"`
}

// OutgoingMessage is the top-level frame to the extension. messageId is set
// only on the setupEncryption reply (-1 marks a new session, as the official
// desktop client does); encrypted replies carry their messageId inside the
// decrypted payload instead.
type OutgoingMessage struct {
	Command      string     `json:"command,omitempty"`
	AppID        string     `json:"appId,omitempty"`
	MessageID    *int       `json:"messageId,omitempty"`
	SharedSecret string     `json:"sharedSecret,omitempty"`
	Message      *EncString `json:"message,omitempty"`
}

// SetupRequest is the plaintext message of a setupEncryption request.
type SetupRequest struct {
	Command   string `json:"command"`
	UserID    string `json:"userId,omitempty"`
	PublicKey string `json:"publicKey"`
}

// PayloadRequest is a decrypted encrypted-request payload.
type PayloadRequest struct {
	Command   string `json:"command"`
	UserID    string `json:"userId,omitempty"`
	MessageID *int   `json:"messageId,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// RequestKind tells setup (plaintext) apart from payload (encrypted) traffic.
type RequestKind int

const (
	RequestSetup RequestKind = iota
	RequestPayload
)

// Request is one decoded extension message.
type Request struct {
	Kind    RequestKind
	AppID   string
	Setup   *SetupRequest
	Payload *PayloadRequest
}

// MessageID is the request's messageId, or -1 when the request carried none;
// replies echo it so the extension can match its callback.
func (r Request) MessageID() int {
	if r.Payload != nil && r.Payload.MessageID != nil {
		return *r.Payload.MessageID
	}
	return -1
}

// StatusResponse is the decrypted reply to a status query
// (getBiometricsStatus / getBiometricsStatusForUser).
type StatusResponse struct {
	Command   string `json:"command"`
	MessageID int    `json:"messageId"`
	Response  int    `json:"response"`
	Timestamp int64  `json:"timestamp"`
}

// UnlockResponse is the decrypted reply to unlockWithBiometricsForUser.
type UnlockResponse struct {
	Command    string `json:"command"`
	MessageID  int    `json:"messageId"`
	Response   bool   `json:"response"`
	UserKeyB64 string `json:"userKeyB64,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

// Session is one host<->extension conversation. It owns the transport key for
// the process lifetime; setupEncryption only delivers that key to the extension.
type Session struct {
	key           []byte
	handshakeDone bool
}

// NewSession builds a session with a fresh 64-byte transport key.
func NewSession() (*Session, error) {
	key := make([]byte, TransportKeyLength)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("protocol: generate transport key: %w", err)
	}
	return NewSessionWithKey(key)
}

// NewSessionWithKey builds a session around a caller-supplied 64-byte key. It
// exists for tests and for callers that manage key material themselves;
// production code should use NewSession.
func NewSessionWithKey(key []byte) (*Session, error) {
	if len(key) != TransportKeyLength {
		return nil, ErrTransportKeyLength
	}
	stored := make([]byte, TransportKeyLength)
	copy(stored, key)
	return &Session{key: stored}, nil
}

// HandshakeComplete reports whether setupEncryption has been answered.
func (s *Session) HandshakeComplete() bool { return s.handshakeDone }

func (s *Session) encKey() []byte { return s.key[:encKeyLength] }
func (s *Session) macKey() []byte { return s.key[encKeyLength : encKeyLength+macKeyLength] }

// Read decodes one frame payload. An encrypted message is MAC-checked and
// decrypted; a plaintext message is returned as a setup request.
func (s *Session) Read(raw []byte) (Request, error) {
	var env IncomingMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		return Request{}, fmt.Errorf("%w: %v", ErrEnvelope, err)
	}
	if len(env.Message) == 0 {
		return Request{}, ErrEnvelope
	}

	var probe struct {
		Command   string `json:"command"`
		PublicKey string `json:"publicKey"`
		IV        string `json:"iv"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(env.Message, &probe); err != nil {
		return Request{}, fmt.Errorf("%w: %v", ErrEnvelope, err)
	}

	if probe.IV != "" || probe.Data != "" {
		var enc EncString
		if err := json.Unmarshal(env.Message, &enc); err != nil {
			return Request{}, fmt.Errorf("%w: %v", ErrEnvelope, err)
		}
		plaintext, err := s.Decrypt(enc)
		if err != nil {
			return Request{}, err
		}
		var payload PayloadRequest
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			return Request{}, fmt.Errorf("protocol: decode payload: %w", err)
		}
		if payload.Command == "" {
			return Request{}, ErrEnvelope
		}
		return Request{Kind: RequestPayload, AppID: env.AppID, Payload: &payload}, nil
	}

	var setup SetupRequest
	if err := json.Unmarshal(env.Message, &setup); err != nil {
		return Request{}, fmt.Errorf("%w: %v", ErrEnvelope, err)
	}
	if setup.Command == "" {
		return Request{}, ErrEnvelope
	}
	return Request{Kind: RequestSetup, AppID: env.AppID, Setup: &setup}, nil
}

// SetupEncryption wraps the session's transport key with RSA-OAEP-SHA1 under
// the extension's SPKI public key and returns the reply. It echoes appId and
// marks the session with messageId -1.
func (s *Session) SetupEncryption(req Request) (OutgoingMessage, error) {
	if req.Setup == nil || req.Setup.Command != "setupEncryption" {
		return OutgoingMessage{}, ErrNotSetup
	}
	if req.Setup.PublicKey == "" {
		return OutgoingMessage{}, ErrMissingPublicKey
	}
	secret, err := encryptOAEPSHA1(req.Setup.PublicKey, s.key)
	if err != nil {
		return OutgoingMessage{}, err
	}
	s.handshakeDone = true
	newSession := -1
	return OutgoingMessage{
		Command:      "setupEncryption",
		AppID:        req.AppID,
		MessageID:    &newSession,
		SharedSecret: base64.StdEncoding.EncodeToString(secret),
	}, nil
}

// Encrypt marshals nothing; it seals plaintext into a type-2 EncString.
func (s *Session) Encrypt(plaintext []byte) (EncString, error) {
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return EncString{}, fmt.Errorf("protocol: generate iv: %w", err)
	}
	ciphertext, err := encryptAESCBC(s.encKey(), iv, plaintext)
	if err != nil {
		return EncString{}, err
	}
	mac := hmacSHA256(s.macKey(), macInput(iv, ciphertext))
	return EncString{
		IV:             base64.StdEncoding.EncodeToString(iv),
		Mac:            base64.StdEncoding.EncodeToString(mac),
		Data:           base64.StdEncoding.EncodeToString(ciphertext),
		EncryptionType: EncTypeAesCbc256HmacSha256,
	}, nil
}

// Decrypt verifies the HMAC over iv+data before decrypting. A type-0 message
// without a mac is accepted for backward compatibility, as the reference does.
func (s *Session) Decrypt(enc EncString) ([]byte, error) {
	iv, err := base64.StdEncoding.DecodeString(enc.IV)
	if err != nil {
		return nil, fmt.Errorf("protocol: decode iv: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(enc.Data)
	if err != nil {
		return nil, fmt.Errorf("protocol: decode data: %w", err)
	}

	if enc.EncryptionType == EncTypeAesCbc256HmacSha256 || enc.Mac != "" {
		if enc.Mac == "" {
			return nil, ErrMissingMAC
		}
		want, err := base64.StdEncoding.DecodeString(enc.Mac)
		if err != nil {
			return nil, fmt.Errorf("protocol: decode mac: %w", err)
		}
		got := hmacSHA256(s.macKey(), macInput(iv, ciphertext))
		if !hmac.Equal(got, want) {
			return nil, ErrMACMismatch
		}
	}
	return decryptAESCBC(s.encKey(), iv, ciphertext)
}

// Reply seals payload and wraps it in the outgoing envelope, echoing appId.
func (s *Session) Reply(req Request, payload any) (OutgoingMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return OutgoingMessage{}, fmt.Errorf("protocol: marshal payload: %w", err)
	}
	enc, err := s.Encrypt(data)
	if err != nil {
		return OutgoingMessage{}, err
	}
	return OutgoingMessage{AppID: req.AppID, Message: &enc}, nil
}

// Connected is the greeting the host sends before any request arrives.
func Connected(appID string) OutgoingMessage {
	return OutgoingMessage{Command: "connected", AppID: appID}
}

// InvalidateEncryption asks the extension to rebuild the session; the host
// sends it when an incoming message fails MAC verification or decryption.
func InvalidateEncryption(req Request) OutgoingMessage {
	return OutgoingMessage{Command: "invalidateEncryption", AppID: req.AppID}
}

func encryptOAEPSHA1(publicKeyB64 string, message []byte) ([]byte, error) {
	pub, err := parseSPKIPublicKey(publicKeyB64)
	if err != nil {
		return nil, err
	}
	// The extension's WebCrypto handshake is RSA-OAEP with SHA-1; SHA-256 here
	// would fail to unwrap.
	return rsa.EncryptOAEP(sha1.New(), rand.Reader, pub, message, nil)
}

func parseSPKIPublicKey(publicKeyB64 string) (*rsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("protocol: decode public key: %w", err)
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("protocol: parse SPKI public key: %w", err)
	}
	pub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, ErrNotRSAPublicKey
	}
	return pub, nil
}

func macInput(iv, ciphertext []byte) []byte {
	out := make([]byte, 0, len(iv)+len(ciphertext))
	out = append(out, iv...)
	out = append(out, ciphertext...)
	return out
}

func hmacSHA256(macKey, data []byte) []byte {
	mac := hmac.New(sha256.New, macKey)
	mac.Write(data)
	return mac.Sum(nil)
}

func encryptAESCBC(key, iv, plaintext []byte) ([]byte, error) {
	if len(iv) != aes.BlockSize {
		return nil, ErrBadIV
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out, nil
}

func decryptAESCBC(key, iv, ciphertext []byte) ([]byte, error) {
	if len(iv) != aes.BlockSize {
		return nil, ErrBadIV
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, ErrBlockSize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ciphertext)
	return pkcs7Unpad(out, aes.BlockSize)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	n := blockSize - len(data)%blockSize
	padded := make([]byte, len(data)+n)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(n)
	}
	return padded
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, ErrPadding
	}
	n := int(data[len(data)-1])
	if n == 0 || n > blockSize || n > len(data) {
		return nil, ErrPadding
	}
	for _, b := range data[len(data)-n:] {
		if int(b) != n {
			return nil, ErrPadding
		}
	}
	return data[:len(data)-n], nil
}
