// Package protocol speaks the Bitwarden desktop client's two native messaging
// channels: the legacy command protocol (RSA-OAEP-SHA1 key exchange, then
// AES-256-CBC + HMAC) and the newer desktop IPC (Noise_NN_P256_AESGCM_SHA256,
// CBOR frames, JSON RPC). Stdio frames are a 4-byte native-endian length
// followed by JSON.
//
// The details are unforgiving: read WardenBio\AGENTS.md for the
// four traps (64-byte transport key, encryptedString alongside iv/data/mac,
// appId/messageId echo, forwarded-bitwarden-ipc-message envelope) before
// changing any of it.
package protocol
