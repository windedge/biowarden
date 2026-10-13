package sdkipc

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/json"
	"errors"
	"testing"
)

// establish performs the real handshake through Session.Handle and returns the
// peer transport (keys swapped relative to the responder).
func establish(t *testing.T, s *Session) *Transport {
	t.Helper()
	initiator, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("initiator key: %v", err)
	}
	start := MapValue(Pair{Key: TextValue("HandshakeStart"), Value: MapValue(
		Pair{Key: TextValue("ciphersuite"), Value: TextValue(NoiseCipherSuite)},
		Pair{Key: TextValue("noise_frame"), Value: bytesToArray(initiator.PublicKey().Bytes())},
	)})
	out, err := s.Handle(Envelope{Payload: EncodeCBOR(start), Endpoint: EndpointDesktopRenderer})
	if err != nil {
		t.Fatalf("Handle HandshakeStart: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("handshake returned %d envelopes, want 1", len(out))
	}
	if out[0].Topic != "" {
		t.Fatalf("handshake reply has topic %q, want null", out[0].Topic)
	}
	finish, err := DecodeCBOR(out[0].Payload)
	if err != nil {
		t.Fatalf("decode HandshakeFinish: %v", err)
	}
	nf, ok := finish.Lookup("HandshakeFinish")
	if !ok || nf.Kind != KindArray || len(nf.Array) != 81 {
		t.Fatalf("HandshakeFinish is not an 81-byte integer array: %+v", nf)
	}
	if s.transport == nil {
		t.Fatal("handshake did not install a transport")
	}
	return &Transport{sendKey: s.transport.receiveKey, receiveKey: s.transport.sendKey}
}

// rpcCall seals a request, feeds it through the session, decrypts the reply and
// returns both the reply envelope and the parsed JSON body.
func rpcCall(t *testing.T, s *Session, peer *Transport, request map[string]any) (Envelope, map[string]any) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	sealed, nonce, err := peer.Seal(body)
	if err != nil {
		t.Fatalf("seal request: %v", err)
	}
	frame := MapValue(Pair{Key: TextValue("TransportFrame"), Value: MapValue(
		Pair{Key: TextValue("payload"), Value: BytesValue(sealed)},
		Pair{Key: TextValue("nonce"), Value: UintValue(nonce)},
	)})
	out, err := s.Handle(Envelope{Payload: EncodeCBOR(frame), Endpoint: EndpointDesktopRenderer})
	if err != nil {
		t.Fatalf("Handle TransportFrame: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("RPC returned %d envelopes, want 1", len(out))
	}
	env := out[0]
	reply, err := DecodeCBOR(env.Payload)
	if err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	tf, ok := reply.Lookup("TransportFrame")
	if !ok {
		t.Fatalf("reply is not a TransportFrame: %x", env.Payload)
	}
	payloadValue, ok := tf.Lookup("payload")
	if !ok {
		t.Fatal("reply TransportFrame has no payload")
	}
	sealedReply, ok := payloadValue.AsBytes()
	if !ok {
		t.Fatal("reply payload is not a byte string")
	}
	nonceValue, ok := tf.Lookup("nonce")
	if !ok {
		t.Fatal("reply TransportFrame has no nonce")
	}
	replyNonce, ok := nonceValue.AsUint()
	if !ok {
		t.Fatal("reply nonce is not an integer")
	}
	plain, err := peer.Open(sealedReply, replyNonce)
	if err != nil {
		t.Fatalf("decrypt reply: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(plain, &resp); err != nil {
		t.Fatalf("reply body is not JSON: %v", err)
	}
	return env, resp
}

func resultOf(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result is not an object: %#v", resp["result"])
	}
	return result
}

func TestSessionDiscover(t *testing.T) {
	s := &Session{}
	peer := establish(t, s)
	env, resp := rpcCall(t, s, peer, map[string]any{
		"request_id":   "req-1",
		"request_type": "DiscoverRequest",
		"request":      nil,
	})

	if env.Topic != TopicRpcResponse {
		t.Fatalf("reply topic = %q, want %q", env.Topic, TopicRpcResponse)
	}
	if env.Endpoint != EndpointDesktopRenderer {
		t.Fatalf("reply endpoint = %q, want DesktopRenderer", env.Endpoint)
	}
	if resp["request_id"] != "req-1" || resp["request_type"] != "DiscoverRequest" {
		t.Fatalf("reply does not echo the request: %#v", resp)
	}
	ok, _ := resultOf(t, resp)["Ok"].(map[string]any)
	if ok["version"] != DesktopVersion {
		t.Fatalf("version = %v, want %s", ok["version"], DesktopVersion)
	}
}

func TestSessionUsesResponseTopicFromRequest(t *testing.T) {
	s := &Session{}
	peer := establish(t, s)
	env, _ := rpcCall(t, s, peer, map[string]any{
		"request_id":     "req-2",
		"request_type":   "DiscoverRequest",
		"request":        nil,
		"response_topic": "CustomResponseTopic",
	})
	if env.Topic != "CustomResponseTopic" {
		t.Fatalf("reply topic = %q, want CustomResponseTopic", env.Topic)
	}
}

func TestSessionUnknownRequestType(t *testing.T) {
	s := &Session{}
	peer := establish(t, s)
	_, resp := rpcCall(t, s, peer, map[string]any{
		"request_id":   "req-3",
		"request_type": "NotARequest",
		"request":      nil,
	})
	result := resultOf(t, resp)
	if result["Err"] != "NoHandlerFound" {
		t.Fatalf("result = %#v, want {\"Err\":\"NoHandlerFound\"}", result)
	}
	if _, hasOk := result["Ok"]; hasOk {
		t.Fatal("an error result must not also carry Ok")
	}
}

func TestSessionUnlockBiometrics(t *testing.T) {
	s := &Session{Unlock: func(userID string) (string, bool) {
		if userID == "user-a" {
			return "dXNlci1rZXk=", true
		}
		return "", false
	}}
	peer := establish(t, s)

	_, resp := rpcCall(t, s, peer, map[string]any{
		"request_id":   "req-4",
		"request_type": "UnlockBiometrics",
		"request":      map[string]any{"user_id": "user-a"},
	})
	ok, _ := resultOf(t, resp)["Ok"].(map[string]any)
	if ok["user_key"] != "dXNlci1rZXk=" {
		t.Fatalf("user_key = %v, want the base64 key", ok["user_key"])
	}

	// A cancelled prompt reports Ok with a null key, not an error.
	_, resp = rpcCall(t, s, peer, map[string]any{
		"request_id":   "req-5",
		"request_type": "UnlockBiometrics",
		"request":      map[string]any{"user_id": "user-b"},
	})
	ok, _ = resultOf(t, resp)["Ok"].(map[string]any)
	if v, present := ok["user_key"]; !present || v != nil {
		t.Fatalf("user_key = %v (present=%v), want an explicit null", v, present)
	}
}

func TestSessionAuthenticateBiometrics(t *testing.T) {
	s := &Session{Authenticate: func() bool { return true }}
	peer := establish(t, s)
	_, resp := rpcCall(t, s, peer, map[string]any{
		"request_id":   "req-6",
		"request_type": "AuthenticateBiometrics",
		"request":      nil,
	})
	if passed, _ := resultOf(t, resp)["Ok"].(bool); !passed {
		t.Fatalf("Ok = %v, want true", resultOf(t, resp)["Ok"])
	}

	s = &Session{}
	peer = establish(t, s)
	_, resp = rpcCall(t, s, peer, map[string]any{
		"request_id":   "req-7",
		"request_type": "AuthenticateBiometrics",
		"request":      nil,
	})
	if passed, _ := resultOf(t, resp)["Ok"].(bool); passed {
		t.Fatal("a nil handler must report false")
	}
}

func TestSessionGetBiometricsStatus(t *testing.T) {
	cases := []struct {
		name    string
		session *Session
		userID  string
		want    BiometricsStatus
	}{
		{
			name:    "no availability handler",
			session: &Session{},
			want:    BiometricsHardwareUnavailable,
		},
		{
			name:    "hardware unavailable",
			session: &Session{Availability: func() bool { return false }},
			want:    BiometricsHardwareUnavailable,
		},
		{
			name:    "no account key",
			session: &Session{Availability: func() bool { return true }, KeyExists: func(string) bool { return false }},
			userID:  "user-a",
			want:    BiometricsNotEnabled,
		},
		{
			name:    "enrolled",
			session: &Session{Availability: func() bool { return true }, KeyExists: func(id string) bool { return id == "user-a" }},
			userID:  "user-a",
			want:    BiometricsAvailable,
		},
		{
			name:    "no user id",
			session: &Session{Availability: func() bool { return true }},
			want:    BiometricsNotEnabled,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			peer := establish(t, c.session)
			request := map[string]any{"request_id": "req-8", "request_type": "GetBiometricsStatus"}
			if c.userID != "" {
				request["request"] = map[string]any{"user_id": c.userID}
			} else {
				request["request"] = nil
			}
			_, resp := rpcCall(t, c.session, peer, request)
			got := resultOf(t, resp)["Ok"]
			if got != string(c.want) {
				t.Fatalf("status = %v, want %s", got, c.want)
			}
		})
	}
}

func TestSessionNotReadyBeforeHandshake(t *testing.T) {
	s := &Session{}
	frame := MapValue(Pair{Key: TextValue("TransportFrame"), Value: MapValue(
		Pair{Key: TextValue("payload"), Value: BytesValue([]byte{1, 2, 3})},
		Pair{Key: TextValue("nonce"), Value: UintValue(1)},
	)})
	if _, err := s.Handle(Envelope{Payload: EncodeCBOR(frame)}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("got %v, want ErrNotReady", err)
	}
}

func TestSessionRejectsUnsupportedCiphersuite(t *testing.T) {
	s := &Session{}
	start := MapValue(Pair{Key: TextValue("HandshakeStart"), Value: MapValue(
		Pair{Key: TextValue("ciphersuite"), Value: TextValue("Noise_XX_25519_ChaChaPoly_BLAKE2s")},
		Pair{Key: TextValue("noise_frame"), Value: ArrayValue(UintValue(4))},
	)})
	if _, err := s.Handle(Envelope{Payload: EncodeCBOR(start)}); !errors.Is(err, ErrUnknownFrame) {
		t.Fatalf("got %v, want ErrUnknownFrame", err)
	}
}

func TestSessionAcceptsByteStringNoiseFrame(t *testing.T) {
	s := &Session{}
	initiator, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("initiator key: %v", err)
	}
	start := MapValue(Pair{Key: TextValue("HandshakeStart"), Value: MapValue(
		Pair{Key: TextValue("ciphersuite"), Value: TextValue(NoiseCipherSuite)},
		Pair{Key: TextValue("noise_frame"), Value: BytesValue(initiator.PublicKey().Bytes())},
	)})
	out, err := s.Handle(Envelope{Payload: EncodeCBOR(start), Endpoint: EndpointDesktopRenderer})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || s.transport == nil {
		t.Fatalf("byte-string noise_frame was not accepted: out=%d transport=%v", len(out), s.transport)
	}
}

func TestSessionTamperedFrameYieldsCryptoInvalidated(t *testing.T) {
	s := &Session{}
	establish(t, s)
	frame := MapValue(Pair{Key: TextValue("TransportFrame"), Value: MapValue(
		Pair{Key: TextValue("payload"), Value: BytesValue([]byte("garbage"))},
		Pair{Key: TextValue("nonce"), Value: UintValue(1)},
	)})
	out, err := s.Handle(Envelope{Payload: EncodeCBOR(frame), Endpoint: EndpointDesktopRenderer})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d envelopes, want a CryptoInvalidated notice", len(out))
	}
	notice, err := DecodeCBOR(out[0].Payload)
	if err != nil {
		t.Fatalf("decode notice: %v", err)
	}
	if !isCryptoInvalidated(notice) {
		t.Fatalf("notice = %+v, want CryptoInvalidated", notice)
	}
	if s.transport != nil {
		t.Fatal("a decryption failure must drop the transport")
	}
}

func TestSessionCryptoInvalidatedResets(t *testing.T) {
	s := &Session{}
	establish(t, s)
	out, err := s.Handle(Envelope{Payload: EncodeCBOR(TextValue("CryptoInvalidated"))})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("got %d envelopes, want none", len(out))
	}
	if s.transport != nil {
		t.Fatal("CryptoInvalidated must drop the transport")
	}
}

func TestSessionRejectsUnrecognizedFrame(t *testing.T) {
	s := &Session{}
	if _, err := s.Handle(Envelope{Payload: EncodeCBOR(MapValue(
		Pair{Key: TextValue("SomethingElse"), Value: NullValue()}))}); !errors.Is(err, ErrUnknownFrame) {
		t.Fatalf("got %v, want ErrUnknownFrame", err)
	}
}

func TestEnvelopeEncodeForwardedShape(t *testing.T) {
	env := Envelope{Payload: []byte{1, 2, 3}, Topic: TopicRpcResponse, Endpoint: EndpointDesktopRenderer}
	data, err := env.EncodeJSON()
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var raw struct {
		Type           string `json:"type"`
		OriginalSource string `json:"originalSource"`
		Message        struct {
			Destination string  `json:"destination"`
			Payload     []int   `json:"payload"`
			Topic       *string `json:"topic"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("reply is not JSON: %v", err)
	}
	if raw.Type != "forwarded-bitwarden-ipc-message" {
		t.Fatalf("type = %q, want forwarded-bitwarden-ipc-message", raw.Type)
	}
	if raw.OriginalSource != EndpointDesktopRenderer {
		t.Fatalf("originalSource = %q, want DesktopRenderer", raw.OriginalSource)
	}
	if raw.Message.Destination != EndpointDesktopMain {
		t.Fatalf("destination = %q, want DesktopMain", raw.Message.Destination)
	}
	if !bytes.Equal([]byte{1, 2, 3}, intsToBytes(raw.Message.Payload)) {
		t.Fatalf("payload = %v, want [1 2 3]", raw.Message.Payload)
	}
	if raw.Message.Topic == nil || *raw.Message.Topic != TopicRpcResponse {
		t.Fatalf("topic = %v, want %s", raw.Message.Topic, TopicRpcResponse)
	}
}

func TestEnvelopeEncodeNullTopic(t *testing.T) {
	data, err := (Envelope{Payload: []byte{1}, Endpoint: EndpointDesktopMain}).EncodeJSON()
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var raw struct {
		Message struct {
			Topic *string `json:"topic"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("reply is not JSON: %v", err)
	}
	if raw.Message.Topic != nil {
		t.Fatalf("topic = %q, want null", *raw.Message.Topic)
	}
}

func TestParseEnvelopeKeepsDestination(t *testing.T) {
	data := []byte(`{"type":"bitwarden-ipc-message","message":{"destination":"DesktopRenderer","payload":[161,110],"topic":null}}`)
	env, ok := ParseEnvelope(data)
	if !ok {
		t.Fatal("ParseEnvelope rejected a valid IPC message")
	}
	if env.Endpoint != EndpointDesktopRenderer {
		t.Fatalf("endpoint = %q, want DesktopRenderer", env.Endpoint)
	}
	if !bytes.Equal(env.Payload, []byte{161, 110}) {
		t.Fatalf("payload = %v, want [161 110]", env.Payload)
	}
	if env.Topic != "" {
		t.Fatalf("topic = %q, want empty (null)", env.Topic)
	}
}

func TestParseEnvelopeDefaultsDestination(t *testing.T) {
	data := []byte(`{"type":"bitwarden-ipc-message","message":{"payload":[1],"topic":"T"}}`)
	env, ok := ParseEnvelope(data)
	if !ok {
		t.Fatal("ParseEnvelope rejected a valid IPC message")
	}
	if env.Endpoint != EndpointDesktopMain {
		t.Fatalf("endpoint = %q, want DesktopMain", env.Endpoint)
	}
	if env.Topic != "T" {
		t.Fatalf("topic = %q, want T", env.Topic)
	}
}

func TestParseEnvelopeRejectsNonIpc(t *testing.T) {
	for _, data := range []string{
		`{"command":"connected"}`,
		`{"type":"bitwarden-ipc-message"}`,
		`{"type":"something-else","message":{"payload":[1]}}`,
		`not json`,
	} {
		if _, ok := ParseEnvelope([]byte(data)); ok {
			t.Errorf("ParseEnvelope(%q) accepted a non-IPC message", data)
		}
	}
}

func intsToBytes(values []int) []byte {
	out := make([]byte, len(values))
	for i, v := range values {
		out[i] = byte(v)
	}
	return out
}
