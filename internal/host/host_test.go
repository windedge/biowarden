package host

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/hostlog"
	"github.com/windedge/biowarden/internal/protocol"
	"github.com/windedge/biowarden/internal/protocol/sdkipc"
	"github.com/windedge/biowarden/internal/secrets"
)

const testAppID = "chrome-extension://nngceckbapebfimnlniiiahkandclblb"

// --- seams ------------------------------------------------------------------

type fakeVerifier struct {
	mu        sync.Mutex
	available bool
	availErr  error
	result    hello.Result
	err       error
	calls     int
	reason    string
}

func (f *fakeVerifier) Available() (bool, error) { return f.available, f.availErr }

func (f *fakeVerifier) Verify(reason string) (hello.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.reason = reason
	return f.result, f.err
}

func (f *fakeVerifier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeStore struct {
	mu   sync.Mutex
	keys map[string]secrets.Key
	gets int
}

func newFakeStore(keys map[string]secrets.Key) *fakeStore { return &fakeStore{keys: keys} }

func (s *fakeStore) List() ([]secrets.Account, error) { return nil, nil }

func (s *fakeStore) Get(userID string) (secrets.Key, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets++
	key, ok := s.keys[userID]
	if !ok {
		return nil, secrets.ErrNotFound
	}
	return append(secrets.Key(nil), key...), nil
}

func (s *fakeStore) Put(secrets.Account, secrets.Key) error { return nil }
func (s *fakeStore) Remove(string) error                    { return nil }

func (s *fakeStore) getCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gets
}

func newTestLogger(t *testing.T, level hostlog.Level) (*hostlog.Logger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "host.log")
	logger, err := hostlog.New(path, level)
	if err != nil {
		t.Fatalf("hostlog.New: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return logger, path
}

// --- pipe harness -----------------------------------------------------------

func startHost(t *testing.T, v hello.Verifier, store secrets.Store, log *hostlog.Logger) (*io.PipeWriter, *io.PipeReader, <-chan error) {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := Run(stdinR, stdoutW, WithVerifier(v), WithStore(store), WithLogger(log))
		_ = stdoutW.Close()
		done <- err
	}()
	return stdinW, stdoutR, done
}

func closeAndWait(t *testing.T, stdinW *io.PipeWriter, done <-chan error) {
	t.Helper()
	_ = stdinW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after stdin closed")
	}
}

func writeFrame(t *testing.T, w io.Writer, payload []byte) {
	t.Helper()
	if err := protocol.WriteFrame(w, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
}

func readFrame(t *testing.T, r io.Reader) []byte {
	t.Helper()
	payload, err := protocol.ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	return payload
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func readOutgoing(t *testing.T, r io.Reader) protocol.OutgoingMessage {
	t.Helper()
	var msg protocol.OutgoingMessage
	if err := json.Unmarshal(readFrame(t, r), &msg); err != nil {
		t.Fatalf("outgoing message is not JSON: %v", err)
	}
	return msg
}

func readGreeting(t *testing.T, r io.Reader) {
	t.Helper()
	if msg := readOutgoing(t, r); msg.Command != "connected" {
		t.Fatalf("first reply = %q, want connected", msg.Command)
	}
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return priv
}

func intPtr(v int) *int { return &v }

// --- legacy channel helpers -------------------------------------------------

// legacyHandshake runs setupEncryption for real and rebuilds the extension's
// session from the RSA-wrapped transport key.
func legacyHandshake(t *testing.T, priv *rsa.PrivateKey, stdinW *io.PipeWriter, stdoutR *io.PipeReader) *protocol.Session {
	t.Helper()
	spki, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal SPKI: %v", err)
	}
	setup := protocol.SetupRequest{
		Command:   "setupEncryption",
		UserID:    "user-a",
		PublicKey: base64.StdEncoding.EncodeToString(spki),
	}
	writeFrame(t, stdinW, mustJSON(t, protocol.IncomingMessage{AppID: testAppID, Message: mustJSON(t, setup)}))
	reply := readOutgoing(t, stdoutR)
	if reply.Command != "setupEncryption" {
		t.Fatalf("setup reply = %q, want setupEncryption", reply.Command)
	}
	secret, err := base64.StdEncoding.DecodeString(reply.SharedSecret)
	if err != nil {
		t.Fatalf("sharedSecret is not base64: %v", err)
	}
	key, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, priv, secret, nil)
	if err != nil {
		t.Fatalf("OAEP decrypt: %v", err)
	}
	if len(key) != protocol.TransportKeyLength {
		t.Fatalf("transport key = %d bytes, want %d", len(key), protocol.TransportKeyLength)
	}
	session, err := protocol.NewSessionWithKey(key)
	if err != nil {
		t.Fatalf("extension session: %v", err)
	}
	return session
}

func sendEncryptedLegacy(t *testing.T, session *protocol.Session, stdinW *io.PipeWriter, payload any) {
	t.Helper()
	enc, err := session.Encrypt(mustJSON(t, payload))
	if err != nil {
		t.Fatalf("encrypt payload: %v", err)
	}
	writeFrame(t, stdinW, mustJSON(t, protocol.IncomingMessage{AppID: testAppID, Message: mustJSON(t, enc)}))
}

func decryptLegacyReply(t *testing.T, session *protocol.Session, stdoutR *io.PipeReader, out any) {
	t.Helper()
	msg := readOutgoing(t, stdoutR)
	if msg.Message == nil {
		t.Fatalf("reply has no encrypted message: %+v", msg)
	}
	plain, err := session.Decrypt(*msg.Message)
	if err != nil {
		t.Fatalf("decrypt reply: %v", err)
	}
	if err := json.Unmarshal(plain, out); err != nil {
		t.Fatalf("reply body is not the expected message: %v (raw %s)", err, plain)
	}
}

// --- legacy tests -----------------------------------------------------------

func TestLegacyUnlockSucceeds(t *testing.T) {
	key := secrets.Key("raw-key-bytes")
	verifier := &fakeVerifier{available: true, result: hello.Verified}
	store := newFakeStore(map[string]secrets.Key{"user-a": key})
	logger, _ := newTestLogger(t, hostlog.LevelInfo)

	stdinW, stdoutR, done := startHost(t, verifier, store, logger)
	defer stdinW.Close()
	readGreeting(t, stdoutR)

	session := legacyHandshake(t, newRSAKey(t), stdinW, stdoutR)
	sendEncryptedLegacy(t, session, stdinW, protocol.PayloadRequest{
		Command:   "unlockWithBiometricsForUser",
		UserID:    "user-a",
		MessageID: intPtr(7),
		Timestamp: time.Now().UnixMilli(),
	})

	var resp protocol.UnlockResponse
	decryptLegacyReply(t, session, stdoutR, &resp)
	if !resp.Response || resp.UserKeyB64 != base64.StdEncoding.EncodeToString(key) {
		t.Fatalf("unlock response = %+v, want response=true userKeyB64=%q", resp, key)
	}
	if resp.MessageID != 7 {
		t.Fatalf("messageId = %d, want the request's 7", resp.MessageID)
	}
	if verifier.callCount() != 1 {
		t.Fatalf("verify calls = %d, want 1", verifier.callCount())
	}
	if store.getCount() != 1 {
		t.Fatalf("store gets = %d, want 1", store.getCount())
	}
	closeAndWait(t, stdinW, done)
}

func TestLegacyUnlockDeniedNeverReadsKey(t *testing.T) {
	verifier := &fakeVerifier{available: true, result: hello.Canceled}
	store := newFakeStore(map[string]secrets.Key{"user-a": []byte("raw-key-bytes")})
	logger, _ := newTestLogger(t, hostlog.LevelInfo)

	stdinW, stdoutR, done := startHost(t, verifier, store, logger)
	defer stdinW.Close()
	readGreeting(t, stdoutR)

	session := legacyHandshake(t, newRSAKey(t), stdinW, stdoutR)
	sendEncryptedLegacy(t, session, stdinW, protocol.PayloadRequest{
		Command:   "unlockWithBiometricsForUser",
		UserID:    "user-a",
		MessageID: intPtr(1),
		Timestamp: time.Now().UnixMilli(),
	})

	var resp protocol.UnlockResponse
	decryptLegacyReply(t, session, stdoutR, &resp)
	if resp.Response || resp.UserKeyB64 != "" {
		t.Fatalf("denied unlock returned a key: %+v", resp)
	}
	if store.getCount() != 0 {
		t.Fatalf("store was read %d times after a refused prompt, want 0", store.getCount())
	}
	closeAndWait(t, stdinW, done)
}

func TestLegacyStatusForUser(t *testing.T) {
	verifier := &fakeVerifier{available: true, result: hello.Verified}
	store := newFakeStore(map[string]secrets.Key{"user-a": []byte("raw-key-bytes")})
	logger, _ := newTestLogger(t, hostlog.LevelInfo)

	stdinW, stdoutR, done := startHost(t, verifier, store, logger)
	defer stdinW.Close()
	readGreeting(t, stdoutR)

	session := legacyHandshake(t, newRSAKey(t), stdinW, stdoutR)
	sendEncryptedLegacy(t, session, stdinW, protocol.PayloadRequest{
		Command:   "getBiometricsStatusForUser",
		UserID:    "user-a",
		MessageID: intPtr(2),
		Timestamp: time.Now().UnixMilli(),
	})
	var known protocol.StatusResponse
	decryptLegacyReply(t, session, stdoutR, &known)
	if known.Response != protocol.BiometricsAvailable {
		t.Fatalf("status for a stored user = %d, want %d", known.Response, protocol.BiometricsAvailable)
	}
	// A status query must not prompt, only report enrolment.
	if verifier.callCount() != 0 {
		t.Fatalf("status query called Verify %d times, want 0", verifier.callCount())
	}
	closeAndWait(t, stdinW, done)
}

func TestLegacyUnknownUserStatus(t *testing.T) {
	verifier := &fakeVerifier{available: true}
	store := newFakeStore(nil)
	logger, _ := newTestLogger(t, hostlog.LevelInfo)

	stdinW, stdoutR, done := startHost(t, verifier, store, logger)
	defer stdinW.Close()
	readGreeting(t, stdoutR)

	session := legacyHandshake(t, newRSAKey(t), stdinW, stdoutR)
	sendEncryptedLegacy(t, session, stdinW, protocol.PayloadRequest{
		Command:   "getBiometricsStatusForUser",
		UserID:    "missing",
		MessageID: intPtr(3),
		Timestamp: time.Now().UnixMilli(),
	})
	var resp protocol.StatusResponse
	decryptLegacyReply(t, session, stdoutR, &resp)
	if resp.Response != protocol.BiometricsNotEnabledInDesktop {
		t.Fatalf("status = %d, want %d", resp.Response, protocol.BiometricsNotEnabledInDesktop)
	}
	closeAndWait(t, stdinW, done)
}

// A tampered frame must get invalidateEncryption so the extension rebuilds its
// session instead of waiting forever.
func TestLegacyTamperedFrameRequestsRehandshake(t *testing.T) {
	verifier := &fakeVerifier{available: true}
	store := newFakeStore(nil)
	logger, _ := newTestLogger(t, hostlog.LevelInfo)

	stdinW, stdoutR, done := startHost(t, verifier, store, logger)
	defer stdinW.Close()
	readGreeting(t, stdoutR)

	session := legacyHandshake(t, newRSAKey(t), stdinW, stdoutR)
	enc, err := session.Encrypt([]byte(`{"command":"getBiometricsStatus","messageId":1}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	enc.Mac = base64.StdEncoding.EncodeToString([]byte("00000000000000000000000000000000"))
	writeFrame(t, stdinW, mustJSON(t, protocol.IncomingMessage{AppID: testAppID, Message: mustJSON(t, enc)}))

	reply := readOutgoing(t, stdoutR)
	if reply.Command != "invalidateEncryption" {
		t.Fatalf("reply = %+v, want invalidateEncryption", reply)
	}
	closeAndWait(t, stdinW, done)
}

// --- SDK IPC test -----------------------------------------------------------

func TestSDKIPCEndToEnd(t *testing.T) {
	key := secrets.Key("raw-key-bytes")
	verifier := &fakeVerifier{available: true, result: hello.Verified}
	store := newFakeStore(map[string]secrets.Key{"user-a": key})
	logger, _ := newTestLogger(t, hostlog.LevelInfo)

	stdinW, stdoutR, done := startHost(t, verifier, store, logger)
	defer stdinW.Close()
	readGreeting(t, stdoutR)

	peer := sdkHandshake(t, stdinW, stdoutR)

	discover := sdkCall(t, stdinW, stdoutR, peer, map[string]any{
		"request_id":   "req-1",
		"request_type": "DiscoverRequest",
		"request":      nil,
	})
	if discover["request_id"] != "req-1" {
		t.Fatalf("discover did not echo request_id: %#v", discover)
	}
	ok, _ := discover["result"].(map[string]any)["Ok"].(map[string]any)
	if ok["version"] != sdkipc.DesktopVersion {
		t.Fatalf("discover version = %v, want %s", ok["version"], sdkipc.DesktopVersion)
	}

	unlock := sdkCall(t, stdinW, stdoutR, peer, map[string]any{
		"request_id":   "req-2",
		"request_type": "UnlockBiometrics",
		"request":      map[string]any{"user_id": "user-a"},
	})
	unlockOK, _ := unlock["result"].(map[string]any)["Ok"].(map[string]any)
	if unlockOK["user_key"] != base64.StdEncoding.EncodeToString(key) {
		t.Fatalf("user_key = %v, want %q", unlockOK["user_key"], key)
	}
	if verifier.callCount() != 1 {
		t.Fatalf("verify calls = %d, want 1", verifier.callCount())
	}
	closeAndWait(t, stdinW, done)
}

func TestSDKIPCUnlockDeniedNeverReadsKey(t *testing.T) {
	verifier := &fakeVerifier{available: true, result: hello.Canceled}
	store := newFakeStore(map[string]secrets.Key{"user-a": []byte("raw-key-bytes")})
	logger, _ := newTestLogger(t, hostlog.LevelInfo)

	stdinW, stdoutR, done := startHost(t, verifier, store, logger)
	defer stdinW.Close()
	readGreeting(t, stdoutR)

	peer := sdkHandshake(t, stdinW, stdoutR)
	unlock := sdkCall(t, stdinW, stdoutR, peer, map[string]any{
		"request_id":   "req-1",
		"request_type": "UnlockBiometrics",
		"request":      map[string]any{"user_id": "user-a"},
	})
	unlockOK, _ := unlock["result"].(map[string]any)["Ok"].(map[string]any)
	if v, present := unlockOK["user_key"]; !present || v != nil {
		t.Fatalf("denied unlock user_key = %v (present=%v), want null", v, present)
	}
	if store.getCount() != 0 {
		t.Fatalf("store was read %d times after a refused prompt, want 0", store.getCount())
	}
	closeAndWait(t, stdinW, done)
}

// --- lifecycle and redaction ------------------------------------------------

func TestStdinClosedReturnsCleanly(t *testing.T) {
	logger, _ := newTestLogger(t, hostlog.LevelInfo)
	stdinW, stdoutR, done := startHost(t, &fakeVerifier{}, newFakeStore(nil), logger)
	defer stdinW.Close()
	readGreeting(t, stdoutR)

	_ = stdinW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil on stdin close", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after stdin closed")
	}
}

func TestHostLogKeepsNoKeyPlaintext(t *testing.T) {
	key := secrets.Key(strings.Repeat("A", 44))
	verifier := &fakeVerifier{available: true, result: hello.Verified}
	store := newFakeStore(map[string]secrets.Key{"user-a": key})
	logger, path := newTestLogger(t, hostlog.LevelDebug)

	stdinW, stdoutR, done := startHost(t, verifier, store, logger)
	defer stdinW.Close()
	readGreeting(t, stdoutR)

	session := legacyHandshake(t, newRSAKey(t), stdinW, stdoutR)
	sendEncryptedLegacy(t, session, stdinW, protocol.PayloadRequest{
		Command:   "unlockWithBiometricsForUser",
		UserID:    "user-a",
		MessageID: intPtr(1),
		Timestamp: time.Now().UnixMilli(),
	})
	var resp protocol.UnlockResponse
	decryptLegacyReply(t, session, stdoutR, &resp)
	closeAndWait(t, stdinW, done)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(content), string(key)) {
		t.Fatalf("log contains the key plaintext:\n%s", content)
	}
	if !strings.Contains(string(content), "<redacted>") {
		t.Fatalf("log has no redaction marker:\n%s", content)
	}
}

// --- SDK IPC wire helpers ---------------------------------------------------
//
// The sdkipc package only ships the responder, so the test drives the real host
// as the initiator. These helpers re-derive the initiator's transport keys from
// the handshake; they mirror noise.go on the initiator side.

type noisePeer struct {
	sendKey    []byte
	receiveKey []byte
	sendNonce  uint64
	recvNonce  uint64
}

func newNoisePeer(t *testing.T, initiator *ecdh.PrivateKey, finish []byte) *noisePeer {
	t.Helper()
	if len(finish) != 81 {
		t.Fatalf("HandshakeFinish = %d bytes, want 81", len(finish))
	}
	responderPub, err := ecdh.P256().NewPublicKey(finish[:65])
	if err != nil {
		t.Fatalf("responder public key: %v", err)
	}
	padded := make([]byte, 32)
	copy(padded, sdkipc.NoiseCipherSuite)
	h := sha256.Sum256(padded)
	ck := padded
	h = noiseMixHash(h, initiator.PublicKey().Bytes())
	h = noiseMixHash(h, nil)
	h = noiseMixHash(h, finish[:65])
	dh, err := initiator.ECDH(responderPub)
	if err != nil {
		t.Fatalf("ECDH: %v", err)
	}
	ck, temp := noiseHKDF2(ck, dh)
	if _, err := noiseOpen(temp, 0, h[:], finish[65:]); err != nil {
		t.Fatalf("handshake tag: %v", err)
	}
	h = noiseMixHash(h, finish[65:])
	split := noiseHKDF(ck, nil, 2)
	return &noisePeer{sendKey: split[0], receiveKey: split[1]}
}

func (p *noisePeer) seal(t *testing.T, plaintext []byte) ([]byte, uint64) {
	t.Helper()
	p.sendNonce++
	sealed, err := noiseSeal(p.sendKey, p.sendNonce, nil, plaintext)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	return sealed, p.sendNonce
}

func (p *noisePeer) open(t *testing.T, sealed []byte, nonce uint64) []byte {
	t.Helper()
	plain, err := noiseOpen(p.receiveKey, nonce, nil, sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	p.recvNonce = nonce
	return plain
}

func sdkHandshake(t *testing.T, stdinW *io.PipeWriter, stdoutR *io.PipeReader) *noisePeer {
	t.Helper()
	initiator, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("initiator key: %v", err)
	}
	start := sdkipc.MapValue(sdkipc.Pair{Key: sdkipc.TextValue("HandshakeStart"), Value: sdkipc.MapValue(
		sdkipc.Pair{Key: sdkipc.TextValue("ciphersuite"), Value: sdkipc.TextValue(sdkipc.NoiseCipherSuite)},
		sdkipc.Pair{Key: sdkipc.TextValue("noise_frame"), Value: sdkipc.ArrayValue(bytesToValues(initiator.PublicKey().Bytes())...)},
	)})
	sendIPC(t, stdinW, sdkipc.EncodeCBOR(start))
	finishFrame, err := sdkipc.DecodeCBOR(readIPC(t, stdoutR))
	if err != nil {
		t.Fatalf("decode HandshakeFinish: %v", err)
	}
	finish, ok := finishFrame.Lookup("HandshakeFinish")
	if !ok {
		t.Fatalf("reply is missing HandshakeFinish: %+v", finishFrame)
	}
	return newNoisePeer(t, initiator, valuesToBytes(t, finish))
}

func sdkCall(t *testing.T, stdinW *io.PipeWriter, stdoutR *io.PipeReader, peer *noisePeer, request map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	sealed, nonce := peer.seal(t, body)
	frame := sdkipc.MapValue(sdkipc.Pair{Key: sdkipc.TextValue("TransportFrame"), Value: sdkipc.MapValue(
		sdkipc.Pair{Key: sdkipc.TextValue("payload"), Value: sdkipc.BytesValue(sealed)},
		sdkipc.Pair{Key: sdkipc.TextValue("nonce"), Value: sdkipc.UintValue(nonce)},
	)})
	sendIPC(t, stdinW, sdkipc.EncodeCBOR(frame))

	reply, err := sdkipc.DecodeCBOR(readIPC(t, stdoutR))
	if err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	tf, ok := reply.Lookup("TransportFrame")
	if !ok {
		t.Fatalf("reply is not a TransportFrame: %+v", reply)
	}
	sealedValue, _ := tf.Lookup("payload")
	sealedReply, ok := sealedValue.AsBytes()
	if !ok {
		t.Fatal("reply payload is not a byte string")
	}
	nonceValue, _ := tf.Lookup("nonce")
	replyNonce, ok := nonceValue.AsUint()
	if !ok {
		t.Fatal("reply nonce is not an integer")
	}
	var resp map[string]any
	if err := json.Unmarshal(peer.open(t, sealedReply, replyNonce), &resp); err != nil {
		t.Fatalf("reply body is not JSON: %v", err)
	}
	return resp
}

func sendIPC(t *testing.T, w io.Writer, payload []byte) {
	t.Helper()
	envelope := map[string]any{
		"type": "bitwarden-ipc-message",
		"message": map[string]any{
			"destination": sdkipc.EndpointDesktopRenderer,
			"payload":     intSlice(payload),
			"topic":       nil,
		},
	}
	writeFrame(t, w, mustJSON(t, envelope))
}

func readIPC(t *testing.T, r io.Reader) []byte {
	t.Helper()
	var envelope struct {
		Message struct {
			Payload []int `json:"payload"`
		} `json:"message"`
	}
	if err := json.Unmarshal(readFrame(t, r), &envelope); err != nil {
		t.Fatalf("ipc envelope: %v", err)
	}
	return byteSlice(envelope.Message.Payload)
}

func intSlice(data []byte) []int {
	out := make([]int, len(data))
	for i, b := range data {
		out[i] = int(b)
	}
	return out
}

func byteSlice(values []int) []byte {
	out := make([]byte, len(values))
	for i, v := range values {
		out[i] = byte(v)
	}
	return out
}

func bytesToValues(data []byte) []sdkipc.Value {
	out := make([]sdkipc.Value, len(data))
	for i, b := range data {
		out[i] = sdkipc.UintValue(uint64(b))
	}
	return out
}

func valuesToBytes(t *testing.T, v sdkipc.Value) []byte {
	t.Helper()
	if b, ok := v.AsBytes(); ok {
		return b
	}
	out := make([]byte, len(v.Array))
	for i, item := range v.Array {
		n, ok := item.AsUint()
		if !ok || n > 0xff {
			t.Fatalf("HandshakeFinish[%d] is not a byte", i)
		}
		out[i] = byte(n)
	}
	return out
}

func noiseMixHash(h [32]byte, data []byte) [32]byte {
	mac := sha256.New()
	mac.Write(h[:])
	mac.Write(data)
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

func noiseHKDF(chainingKey, input []byte, count int) [][]byte {
	temp := noiseHMAC(chainingKey, input)
	out := make([][]byte, 0, count)
	var prev []byte
	for i := 1; i <= count; i++ {
		mac := hmac.New(sha256.New, temp)
		mac.Write(prev)
		mac.Write([]byte{byte(i)})
		next := mac.Sum(nil)
		out = append(out, next)
		prev = next
	}
	return out
}

func noiseHKDF2(chainingKey, input []byte) ([]byte, []byte) {
	out := noiseHKDF(chainingKey, input, 2)
	return out[0], out[1]
}

func noiseHMAC(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func noiseNonce(counter uint64) []byte {
	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce[4:], counter)
	return nonce
}

func noiseGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func noiseSeal(key []byte, counter uint64, aad, plaintext []byte) ([]byte, error) {
	gcm, err := noiseGCM(key)
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nil, noiseNonce(counter), plaintext, aad), nil
}

func noiseOpen(key []byte, counter uint64, aad, sealed []byte) ([]byte, error) {
	gcm, err := noiseGCM(key)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, noiseNonce(counter), sealed, aad)
}
