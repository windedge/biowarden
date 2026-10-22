# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.4] - 2026-10-22

### Added

- Five more interface languages: Traditional Chinese, Japanese, German, French
  and Spanish join English and Simplified Chinese, seven in all. The language
  list on the Settings page shows every language under its own name
  (English / 简体中文 / 繁體中文 / 日本語 / Deutsch / français / español), with
  "Follow the system" first.
- The text fields' right-click menu (Undo, Cut, Copy, Paste, Delete, Select
  All) now speaks the chosen language instead of staying Chinese.

### Changed

- Upgraded mygo to v0.6.0: a long drop-down list stays inside the window and
  scrolls there instead of sticking out of it, and the layout follows the
  stylesheet more closely.
- Every page is now checked in all seven languages, so text that would run
  past the window is caught in the tests instead of in front of the user.
- The Settings page no longer repeats the server address. The account card
  already shows it, and the repeated row kept naming the official cloud
  address after a sign-out, which was misleading.

### Fixed

- Long texts on the Settings page ran past the right edge of the window and
  were cut off; the page's margins now follow the layout.
- The log path field and the drop-downs on the Settings page were wider than
  the room the window had, so in a narrow window or in a language with longer
  labels they were clipped; they now shrink to the available width and keep
  their size on a wide window.
- The tray menu's first item lost the app name's last letter: the menu item
  is sized from the width of its text, but Windows draws that text a few
  pixels wider and cuts what does not fit. The label now ends with a space,
  which takes the cut instead of the letter.

## [0.1.3] - 2026-10-21

### Changed

- The unused refresh button on the account page is gone; every action already
  reloads the page state.

### Fixed

- The last server address is remembered across sign-outs and key deletions,
  so a self-hosted address no longer has to be retyped: the sign-in window
  pre-fills it.

## [0.1.2] - 2026-10-20

### Fixed

- Signing out used to leave the account page claiming "Signed in" with no
  stored key and no buttons; the in-memory session is now reset, so the page
  becomes "Not signed in" with the sign-in button.
- When the key is missing but a session is still valid (the key was deleted
  elsewhere), the card now offers the sign-in button instead of a dead state.

## [0.1.1] - 2026-10-20

### Added

- The window, the tray menu and the sign-in errors are available in English
  and Simplified Chinese; the app follows the system language and the
  Settings page can override it.
- A language selector on the Settings page.
- Clicking the tray icon opens the window.

### Changed

- Upgraded mygo to v0.5.5.
- The account page shows the account and the stored key in one card instead
  of two.
- Settings moved to the bottom of the sidebar: the main navigation is now
  Account / Browsers / Log, and the sidebar no longer shows the version.
- The tray icon is the app icon, the same one the window and the taskbar
  show.
- The account action is now "Sign out" instead of "Delete", the single
  action left: it removes the stored key after a Windows Hello confirmation
  and clears the login session.

### Fixed

- A login session that could not be verified (network or server problem) no
  longer reports that the session was cleared; the notice says the key is
  still stored and unlocking works.
- Release builds no longer carry a `+dirty` version suffix: the temporary
  `.syso` file the mygo build writes is now ignored.

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
