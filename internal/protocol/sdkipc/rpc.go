package sdkipc

import (
	"encoding/json"
	"errors"
	"fmt"
)

// DesktopVersion is what DiscoverRequest reports back. It mirrors the official
// desktop client's version string because the extension may compare formats.
const (
	DesktopVersion          = "2026.9.0"
	TopicRpcRequest         = "RpcRequestMessage"
	TopicRpcResponse        = "RpcResponseMessage"
	EndpointDesktopMain     = "DesktopMain"
	EndpointDesktopRenderer = "DesktopRenderer"
)

var (
	// ErrNotReady means a transport frame arrived before the Noise handshake.
	ErrNotReady = errors.New("sdkipc: noise session is not established")
	// ErrUnknownFrame wraps malformed or unrecognized CBOR/RPC payloads.
	ErrUnknownFrame = errors.New("sdkipc: unrecognized frame")
)

// Business seams. The package does not import the secrets or hello packages;
// the host wires these in.
type (
	UnlockFunc       func(userID string) (userKey string, ok bool)
	AuthenticateFunc func() bool
	AvailabilityFunc func() bool
	KeyExistsFunc    func(userID string) bool
)

// Session drives the IPC channel for a single native messaging connection.
type Session struct {
	Unlock       UnlockFunc
	Authenticate AuthenticateFunc
	Availability AvailabilityFunc
	KeyExists    KeyExistsFunc

	transport *Transport
}

// BiometricsStatus values reported by GetBiometricsStatus.
type BiometricsStatus string

const (
	BiometricsAvailable           BiometricsStatus = "Available"
	BiometricsUnlockNeeded        BiometricsStatus = "UnlockNeeded"
	BiometricsHardwareUnavailable BiometricsStatus = "HardwareUnavailable"
	BiometricsNotEnabled          BiometricsStatus = "NotEnabled"
)

// Envelope is one browser IPC message in the native messaging JSON layer. An
// empty Topic renders as JSON null. Endpoint is echoed as originalSource on
// outgoing frames.
type Envelope struct {
	Payload  []byte
	Topic    string
	Endpoint string
}

// Handle consumes one envelope and returns the envelopes to send back (usually
// zero or one).
func (s *Session) Handle(env Envelope) ([]Envelope, error) {
	frame, err := DecodeCBOR(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnknownFrame, err)
	}

	if start, ok := frame.Lookup("HandshakeStart"); ok {
		return s.handleHandshakeStart(start, env)
	}
	if tf, ok := frame.Lookup("TransportFrame"); ok {
		return s.handleTransportFrame(tf, env)
	}
	if isCryptoInvalidated(frame) {
		s.transport = nil
		return nil, nil
	}
	return nil, fmt.Errorf("%w: not HandshakeStart, TransportFrame or CryptoInvalidated", ErrUnknownFrame)
}

func (s *Session) handleHandshakeStart(start Value, env Envelope) ([]Envelope, error) {
	suite, _ := start.Lookup("ciphersuite")
	name, _ := suite.AsText()
	if name != NoiseCipherSuite {
		return nil, fmt.Errorf("%w: unsupported ciphersuite %q", ErrUnknownFrame, name)
	}
	noiseFrame, ok := start.Lookup("noise_frame")
	if !ok {
		return nil, fmt.Errorf("%w: HandshakeStart is missing noise_frame", ErrUnknownFrame)
	}
	raw, err := cborByteString(noiseFrame)
	if err != nil {
		return nil, fmt.Errorf("%w: noise_frame: %v", ErrUnknownFrame, err)
	}
	finish, responder, err := Respond(raw, nil)
	if err != nil {
		return nil, err
	}
	s.transport = NewTransport(responder)

	// The reference encodes the finish frame as an integer array (Vec<u8>),
	// which the extension accepts, rather than a byte string.
	payload := MapValue(Pair{Key: TextValue("HandshakeFinish"), Value: bytesToArray(finish)})
	return []Envelope{{Payload: EncodeCBOR(payload), Endpoint: env.Endpoint}}, nil
}

func (s *Session) handleTransportFrame(tf Value, env Envelope) ([]Envelope, error) {
	if s.transport == nil {
		return nil, ErrNotReady
	}
	payload, ok := tf.Lookup("payload")
	if !ok {
		return nil, fmt.Errorf("%w: TransportFrame is missing payload", ErrUnknownFrame)
	}
	sealed, ok := payload.AsBytes()
	if !ok {
		return nil, fmt.Errorf("%w: TransportFrame payload is not a byte string", ErrUnknownFrame)
	}
	nonceValue, ok := tf.Lookup("nonce")
	if !ok {
		return nil, fmt.Errorf("%w: TransportFrame is missing nonce", ErrUnknownFrame)
	}
	nonce, ok := nonceValue.AsUint()
	if !ok {
		return nil, fmt.Errorf("%w: TransportFrame nonce is not an integer", ErrUnknownFrame)
	}

	plaintext, err := s.transport.Open(sealed, nonce)
	if err != nil {
		// A failed open means the peers disagree; drop the session and tell the
		// other side to reset rather than erroring out the connection.
		s.transport = nil
		return []Envelope{{Payload: EncodeCBOR(TextValue("CryptoInvalidated")), Endpoint: env.Endpoint}}, nil
	}
	return s.handleRPC(plaintext, env.Endpoint)
}

type rpcRequest struct {
	RequestID     *string         `json:"request_id"`
	RequestType   *string         `json:"request_type"`
	Request       json.RawMessage `json:"request"`
	ResponseTopic *string         `json:"response_topic"`
}

func (s *Session) handleRPC(plaintext []byte, endpoint string) ([]Envelope, error) {
	var req rpcRequest
	if err := json.Unmarshal(plaintext, &req); err != nil {
		return nil, fmt.Errorf("%w: RPC payload is not JSON: %v", ErrUnknownFrame, err)
	}
	if req.RequestID == nil || req.RequestType == nil {
		return nil, fmt.Errorf("%w: RPC request is missing request_id or request_type", ErrUnknownFrame)
	}

	// DiscoverRequest and AuthenticateBiometrics send "request": null.
	userID := ""
	if len(req.Request) > 0 && string(req.Request) != "null" {
		var payload struct {
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(req.Request, &payload); err != nil {
			return nil, fmt.Errorf("%w: RPC request payload: %v", ErrUnknownFrame, err)
		}
		userID = payload.UserID
	}

	response := map[string]any{
		"result":       s.dispatch(*req.RequestType, userID),
		"request_id":   *req.RequestID,
		"request_type": *req.RequestType,
	}
	body, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("sdkipc: marshal RPC response: %w", err)
	}

	// Newer requests name the reply topic; older ones (e.g. the 0.2.0-main.950
	// build bundled in the extension) always use RpcResponseMessage.
	topic := TopicRpcResponse
	if req.ResponseTopic != nil && *req.ResponseTopic != "" {
		topic = *req.ResponseTopic
	}

	sealed, nonce, err := s.transport.Seal(body)
	if err != nil {
		return nil, err
	}
	frame := MapValue(Pair{Key: TextValue("TransportFrame"), Value: MapValue(
		Pair{Key: TextValue("payload"), Value: BytesValue(sealed)},
		Pair{Key: TextValue("nonce"), Value: UintValue(nonce)},
	)})
	return []Envelope{{Payload: EncodeCBOR(frame), Topic: topic, Endpoint: endpoint}}, nil
}

// dispatch returns the externally tagged Rust Result<T, RpcError> body.
func (s *Session) dispatch(requestType, userID string) map[string]any {
	switch requestType {
	case "DiscoverRequest":
		return map[string]any{"Ok": map[string]any{"version": DesktopVersion}}

	case "GetBiometricsStatus":
		return map[string]any{"Ok": string(s.biometricsStatus(userID))}

	case "UnlockBiometrics":
		if userID != "" && s.Unlock != nil {
			if key, ok := s.Unlock(userID); ok {
				return map[string]any{"Ok": map[string]any{"user_key": key}}
			}
		}
		return map[string]any{"Ok": map[string]any{"user_key": nil}}

	case "AuthenticateBiometrics":
		passed := s.Authenticate != nil && s.Authenticate()
		return map[string]any{"Ok": passed}

	default:
		return map[string]any{"Err": "NoHandlerFound"}
	}
}

func (s *Session) biometricsStatus(userID string) BiometricsStatus {
	if s.Availability == nil || !s.Availability() {
		return BiometricsHardwareUnavailable
	}
	if userID != "" && s.KeyExists != nil && s.KeyExists(userID) {
		return BiometricsAvailable
	}
	return BiometricsNotEnabled
}

// cborByteString accepts either the serde byte-string form or the plain
// Vec<u8> integer-array form for a field such as noise_frame.
func cborByteString(v Value) ([]byte, error) {
	if b, ok := v.AsBytes(); ok {
		return b, nil
	}
	if v.Kind == KindArray {
		out := make([]byte, len(v.Array))
		for i, item := range v.Array {
			n, ok := item.AsUint()
			if !ok || n > 0xff {
				return nil, fmt.Errorf("element %d is not a byte", i)
			}
			out[i] = byte(n)
		}
		return out, nil
	}
	return nil, errors.New("expected a byte string or an array of integers")
}

func bytesToArray(b []byte) Value {
	items := make([]Value, len(b))
	for i, by := range b {
		items[i] = UintValue(uint64(by))
	}
	return ArrayValue(items...)
}

func isCryptoInvalidated(v Value) bool {
	if s, ok := v.AsText(); ok && s == "CryptoInvalidated" {
		return true
	}
	_, ok := v.Lookup("CryptoInvalidated")
	return ok
}

// ParseEnvelope decodes a native messaging message, returning false when it is
// not a bitwarden-ipc-message envelope.
func ParseEnvelope(data []byte) (Envelope, bool) {
	var raw struct {
		Type    string `json:"type"`
		Message struct {
			Destination *string `json:"destination"`
			Payload     []int   `json:"payload"`
			Topic       *string `json:"topic"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Envelope{}, false
	}
	if raw.Type != "bitwarden-ipc-message" || raw.Message.Payload == nil {
		return Envelope{}, false
	}
	payload := make([]byte, len(raw.Message.Payload))
	for i, v := range raw.Message.Payload {
		payload[i] = byte(v)
	}
	env := Envelope{Payload: payload, Endpoint: EndpointDesktopMain}
	if raw.Message.Destination != nil {
		env.Endpoint = *raw.Message.Destination
	}
	if raw.Message.Topic != nil {
		env.Topic = *raw.Message.Topic
	}
	return env, true
}

// EncodeJSON renders the envelope for the browser. The wrapper must be a
// forwarded-bitwarden-ipc-message with originalSource: the extension stamps
// plain envelopes as originating from DesktopMain, so a response that does not
// carry its real endpoint is dropped by the originator as coming from the wrong
// source (it surfaces as a send timeout, not an error).
func (e Envelope) EncodeJSON() ([]byte, error) {
	payload := make([]int, len(e.Payload))
	for i, b := range e.Payload {
		payload[i] = int(b)
	}
	var topic *string
	if e.Topic != "" {
		t := e.Topic
		topic = &t
	}
	endpoint := e.Endpoint
	if endpoint == "" {
		endpoint = EndpointDesktopMain
	}
	return json.Marshal(map[string]any{
		"type": "forwarded-bitwarden-ipc-message",
		"message": map[string]any{
			"destination": EndpointDesktopMain,
			"payload":     payload,
			"topic":       topic,
		},
		"originalSource": endpoint,
	})
}
