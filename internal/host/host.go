// Package host is the native messaging host: the browser launches the binary
// with pipes on stdin and stdout and the host serves the extension until they
// close. An unlock reply carries key material, so its logging is redacted.
package host

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/internal/hostlog"
	"github.com/windedge/biowarden/internal/protocol"
	"github.com/windedge/biowarden/internal/protocol/sdkipc"
	"github.com/windedge/biowarden/internal/secrets"
)

// hostAppID identifies this host in the legacy `connected` greeting. It is the
// app id, not the native messaging host name, mirroring the reference.
const hostAppID = "dev.windedge.biowarden"

const (
	unlockReason = "Bitwarden 浏览器扩展请求解锁"
	authReason   = "Bitwarden 浏览器扩展请求身份验证"
)

// Options carries the host's seams so tests can run the real serve loop
// without Windows Hello or the credential store. Its zero value wires the
// production implementations.
type Options struct {
	Verifier hello.Verifier
	Store    secrets.Store
	Log      *hostlog.Logger
}

// Option overrides one seam.
type Option func(*Options)

func WithVerifier(v hello.Verifier) Option { return func(o *Options) { o.Verifier = v } }
func WithStore(s secrets.Store) Option     { return func(o *Options) { o.Store = s } }
func WithLogger(l *hostlog.Logger) Option  { return func(o *Options) { o.Log = l } }

// Run serves frames from stdin, answering on stdout, until the browser closes
// stdin. A clean close returns nil; any other error means the stream is
// unusable.
func Run(stdin io.Reader, stdout io.Writer, opts ...Option) error {
	options := Options{}
	for _, opt := range opts {
		opt(&options)
	}
	if options.Verifier == nil {
		options.Verifier = hello.New()
	}
	if options.Store == nil {
		options.Store = secrets.New()
	}
	if options.Log == nil {
		options.Log = defaultLogger()
	}
	defer options.Log.Close()

	h, err := newHost(options)
	if err != nil {
		return err
	}
	return h.serve(stdin, stdout)
}

func defaultLogger() *hostlog.Logger {
	path, err := hostlog.DefaultPath()
	if err != nil {
		return hostlog.Nop()
	}
	logger, err := hostlog.New(path, hostlog.ParseLevel(os.Getenv("BIOWARDEN_LOG")))
	if err != nil {
		return hostlog.Nop()
	}
	return logger
}

type host struct {
	verifier hello.Verifier
	store    secrets.Store
	log      *hostlog.Logger
	legacy   *protocol.Session
	sdk      *sdkipc.Session
	out      io.Writer
	writeMu  sync.Mutex
}

func newHost(options Options) (*host, error) {
	legacy, err := protocol.NewSession()
	if err != nil {
		return nil, err
	}
	h := &host{verifier: options.Verifier, store: options.Store, log: options.Log, legacy: legacy}
	// The SDK channel calls back into the same consent-before-read path as the
	// legacy channel; only the message shapes differ.
	h.sdk = &sdkipc.Session{
		Unlock:       h.unlockKey,
		Authenticate: func() bool { return h.verifyConsent(authReason) },
		Availability: h.available,
		KeyExists:    h.keyExists,
	}
	return h, nil
}

func (h *host) serve(stdin io.Reader, stdout io.Writer) error {
	h.out = stdout
	h.log.Info(fmt.Sprintf("host started: pid=%d", os.Getpid()))
	if err := h.writeLegacy(protocol.Connected(hostAppID)); err != nil {
		return err
	}
	for {
		raw, err := protocol.ReadFrame(stdin)
		if err != nil {
			if errors.Is(err, io.EOF) {
				h.log.Info("stdin closed, host exiting")
				return nil
			}
			h.log.Error("read frame failed: " + err.Error())
			return err
		}
		h.log.Debug("incoming frame: " + string(raw))
		if err := h.dispatch(raw); err != nil {
			h.log.Error("handle message failed: " + err.Error())
		}
	}
}

// dispatch picks the channel by message shape: the SDK IPC envelope is the only
// one with type=bitwarden-ipc-message, so its parser rejects everything else.
func (h *host) dispatch(raw []byte) error {
	if env, ok := sdkipc.ParseEnvelope(raw); ok {
		return h.dispatchSDK(env)
	}
	return h.dispatchLegacy(raw)
}

func (h *host) dispatchSDK(env sdkipc.Envelope) error {
	h.log.Info("sdk ipc message: endpoint=" + env.Endpoint + " topic=" + env.Topic)
	out, err := h.sdk.Handle(env)
	if err != nil {
		return err
	}
	for _, reply := range out {
		data, err := reply.EncodeJSON()
		if err != nil {
			return err
		}
		h.log.Debug("sdk ipc reply: " + string(data))
		if err := h.writeFrame(data); err != nil {
			return err
		}
	}
	return nil
}

// writeFrame is the only writer for stdout. A frame's length header and body
// must not interleave with another frame, so every future answerer must take
// this lock rather than calling protocol.WriteFrame directly.
func (h *host) writeFrame(data []byte) error {
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	return protocol.WriteFrame(h.out, data)
}

func (h *host) verifyConsent(reason string) bool {
	result, err := h.verifier.Verify(reason)
	if err != nil {
		h.log.Error("hello: verify failed: " + err.Error())
		return false
	}
	if result != hello.Verified {
		h.log.Info("hello: consent not granted (" + result.String() + ")")
		return false
	}
	h.log.Info("hello: consent granted")
	return true
}

func (h *host) available() bool {
	ok, err := h.verifier.Available()
	if err != nil {
		h.log.Error("hello: availability check failed: " + err.Error())
		return false
	}
	return ok
}

// unlockKey is the one path that releases a stored key: consent first, then the
// credential store. A refused prompt must not touch the store, so the two
// channels share this function instead of each reading keys on their own.
func (h *host) unlockKey(userID string) (string, bool) {
	if userID == "" {
		return "", false
	}
	if !h.verifyConsent(unlockReason) {
		return "", false
	}
	// The store holds the raw key bytes; the extension expects base64 text
	// in userKeyB64/user_key, so encode on the way out.
	key, err := h.store.Get(userID)
	if err != nil {
		h.log.Error("secrets: read failed for userId=" + userID + ": " + err.Error())
		return "", false
	}
	h.log.Info("unlock ok userId=" + userID)
	return base64.StdEncoding.EncodeToString(key), true
}

func (h *host) keyExists(userID string) bool {
	if userID == "" {
		return false
	}
	_, err := h.store.Get(userID)
	return err == nil
}
