// Package auth implements the Bitwarden account login link: the user types
// their email and master password directly in biowarden, the package talks to
// the Bitwarden identity and API endpoints while impersonating the official
// desktop client, and it ends up holding the account's 64-byte userKey.
//
// Domain boundary. auth knows only about HTTP, the Bitwarden protocol and key
// derivation. It must not import internal/ui or any other presentation code;
// the GUI drives it through the LoginFlow interface. It may depend on the
// internal/hello and internal/secrets seams, but only through their
// interfaces: auth produces a userKey and never stores it, never asks for a
// biometric consent itself. The caller wires the result of an unlock into
// hello.Verifier (Windows Hello consent) and then secrets.Store.Put (the
// Windows Credential Manager).
//
// This package deliberately keeps the whole secret in memory. It emits no
// logs of its own and every type that carries key material redacts itself
// under %v so a stray log line cannot leak a password or a key.
//
// Current scope. The core password chain is real: server URL derivation, the
// HTTP/header layer, prelogin (both variants), the KDF (PBKDF2-SHA256 and
// Argon2id), EncString type 2 decoding and userKey unlocking, the token
// request, refresh and profile. The second-factor model is real too: the
// challenge names the interaction each offered method needs, Continue
// answers code methods (TOTP, email, YubiKey) with a TwoFactorSubmission, and
// SendEmail posts the email method's preparation request. What is not built
// yet is the browser path itself (Duo, WebAuthn) and the GUI; see
// docs/research/2fa-browser-path.md.
package auth
