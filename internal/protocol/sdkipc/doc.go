// Package sdkipc implements the newer Bitwarden desktop IPC channel that the
// browser extension uses alongside the legacy command protocol: a
// Noise_NN_P256_AESGCM_SHA256 handshake, CBOR frames and JSON RPC carried
// inside the Noise transport.
//
// The reference implementation lives in the WardenBio Swift tree
// (Sources/Protocol/{SdkIpc,NoiseNN,Cbor}.swift). Bitwarden's Rust side is
// sdk-internal 0.2.0-main.950 with the snow 0.10 Noise crate; the subtleties
// that are invisible from a casual read (padded protocol name as both h and ck,
// MixHash of the empty msg1 payload, MixKey updating ck, Split using ck, the
// handshake AEAD AAD, and transport nonces starting at 1) are noted inline.
//
// This package deliberately has no dependency on the secrets or hello seams:
// the business callbacks are injected by the caller via Session.
package sdkipc
