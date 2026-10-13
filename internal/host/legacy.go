package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/windedge/biowarden/internal/protocol"
)

// messageValidWindow is the tolerated clock skew for an encrypted request,
// matching the official desktop client's 10 seconds.
const messageValidWindow = 10 * time.Second

func (h *host) dispatchLegacy(raw []byte) error {
	req, err := h.legacy.Read(raw)
	if err != nil {
		return h.legacyReadError(raw, err)
	}
	switch req.Kind {
	case protocol.RequestSetup:
		return h.handleSetup(req)
	case protocol.RequestPayload:
		return h.handlePayload(req)
	default:
		return nil
	}
}

// legacyReadError turns a channel failure into the one reply that can recover
// it: a stale session key only heals when the extension re-runs setupEncryption,
// so a MAC failure asks for that instead of dying silently.
func (h *host) legacyReadError(raw []byte, err error) error {
	if errors.Is(err, protocol.ErrMACMismatch) || errors.Is(err, protocol.ErrMissingMAC) {
		h.log.Error("legacy decrypt failed, asking for re-handshake: " + err.Error())
		return h.writeLegacy(protocol.InvalidateEncryption(protocol.Request{AppID: rawAppID(raw)}))
	}
	return err
}

// rawAppID recovers the appId from a frame that failed to decrypt, so the
// invalidateEncryption reply can still be addressed to the sender.
func rawAppID(raw []byte) string {
	var env protocol.IncomingMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		return ""
	}
	return env.AppID
}

func (h *host) handleSetup(req protocol.Request) error {
	reply, err := h.legacy.SetupEncryption(req)
	if err != nil {
		return err
	}
	h.log.Info("setupEncryption done userId=" + req.Setup.UserID)
	return h.writeLegacy(reply)
}

func (h *host) handlePayload(req protocol.Request) error {
	p := req.Payload
	h.log.Info(fmt.Sprintf("legacy command=%s userId=%s messageId=%d", p.Command, p.UserID, req.MessageID()))
	if !timestampFresh(p.Timestamp) {
		h.log.Info("legacy message timestamp outside window, ignored")
		return nil
	}
	switch p.Command {
	case "unlockWithBiometricsForUser":
		return h.replyUnlock(req)
	case "authenticateWithBiometrics":
		return h.replyAuthenticate(req)
	case "getBiometricsStatus":
		return h.replyStatus(req, h.statusOverall())
	case "getBiometricsStatusForUser":
		return h.replyStatus(req, h.statusForUser(p.UserID))
	default:
		h.log.Info("unknown legacy command: " + p.Command)
		return nil
	}
}

func (h *host) replyUnlock(req protocol.Request) error {
	resp := protocol.UnlockResponse{
		Command:   "unlockWithBiometricsForUser",
		MessageID: req.MessageID(),
		Timestamp: nowMillis(),
	}
	if req.Payload.UserID == "" {
		h.log.Error("unlockWithBiometricsForUser is missing userId")
		return h.replyLegacy(req, resp)
	}
	if key, ok := h.unlockKey(req.Payload.UserID); ok {
		resp.Response = true
		resp.UserKeyB64 = key
	}
	return h.replyLegacy(req, resp)
}

func (h *host) replyAuthenticate(req protocol.Request) error {
	passed := h.verifyConsent(authReason)
	return h.replyLegacy(req, protocol.StatusResponse{
		Command:   "authenticateWithBiometrics",
		MessageID: req.MessageID(),
		Response:  boolStatus(passed),
		Timestamp: nowMillis(),
	})
}

func (h *host) replyStatus(req protocol.Request, status int) error {
	return h.replyLegacy(req, protocol.StatusResponse{
		Command:   req.Payload.Command,
		MessageID: req.MessageID(),
		Response:  status,
		Timestamp: nowMillis(),
	})
}

func (h *host) statusOverall() int {
	if h.available() {
		return protocol.BiometricsAvailable
	}
	return protocol.BiometricsHardwareUnavailable
}

func (h *host) statusForUser(userID string) int {
	if !h.available() {
		return protocol.BiometricsHardwareUnavailable
	}
	if userID == "" || !h.keyExists(userID) {
		return protocol.BiometricsNotEnabledInDesktop
	}
	return protocol.BiometricsAvailable
}

func (h *host) replyLegacy(req protocol.Request, payload any) error {
	reply, err := h.legacy.Reply(req, payload)
	if err != nil {
		return err
	}
	return h.writeLegacy(reply)
}

func (h *host) writeLegacy(message protocol.OutgoingMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	h.log.Debug("outgoing legacy: " + string(data))
	return h.writeFrame(data)
}

// timestampFresh accepts a missing timestamp (the field is optional) and
// rejects one outside the replay window.
func timestampFresh(ts int64) bool {
	if ts == 0 {
		return true
	}
	delta := time.Since(time.UnixMilli(ts))
	if delta < 0 {
		delta = -delta
	}
	return delta <= messageValidWindow
}

func nowMillis() int64 { return time.Now().UnixMilli() }

func boolStatus(b bool) int {
	if b {
		return 1
	}
	return 0
}
