# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-10-18

First release. biowarden lets the Bitwarden browser extension unlock with
Windows Hello: sign in once, and the account key is stored in the Windows
Credential Manager behind a biometrics check.

### Added

- Sign-in flow with the Bitwarden account (email + master password, 2FA via
  TOTP / email code / YubiKey): prelogin, PBKDF2 and Argon2id key derivation,
  token exchange, and user key decryption implemented in pure Go.
- First-run setup: detects installed browsers, registers the native messaging
  manifests automatically (skipping registrations that belong to other
  programs), and opens the sign-in window. Runs once.
- Login window: modal child window, server URL remembered across restarts,
  Enter to submit, height that follows its content.
- Session persistence: the refresh token, email and server URL are stored in a
  dedicated Credential Manager entry (DPAPI-encrypted); restarts restore the
  signed-in state. The master password and access token stay in memory only.
- Account page with three states (signed in / configured / not signed in),
  re-login as the single key-refresh entry, and a manual extraction-script
  fallback for unsupported 2FA methods and diagnostics.
- Browsers page: per-browser manifest status with one-click install, batch
  install-all with a confirmation dialog that lists foreign/hijacked
  registrations before taking them over, browsers that are not installed are
  skipped, and a connection indicator fed by per-browser history.
- Windows Hello consent prompt centered on the work area and kept above other
  windows (topmost owner window plus a foreground lend/return around the
  request).
- Both extension protocol channels: the legacy command protocol and the
  BiometricsSDK IPC channel (Noise + CBOR + JSON RPC).
- GUI in a sidebar layout (Account / Browsers / Settings / Log) with tray
  icon, single instance and autostart; host mode is selected automatically
  when the browser launches the executable.
- NSIS installer and portable exe builds via `scripts/build.ps1`; MIT license
  with third-party notices; bilingual README (English / Simplified Chinese).

### Fixed

- The two-factor challenge from the real identity server (providers encoded
  as a string array) is parsed correctly; a rejected code retries instead of
  dead-ending the sign-in.
- The consent prompt no longer hides behind other windows.
- Local state (stored keys, Hello availability) renders immediately at
  startup; the session check runs in the background with a short timeout and
  no longer blocks the page.
- Deleting an account key also clears the stored login session.
- Registry tests never touch the real user hive (private app hive instead).
