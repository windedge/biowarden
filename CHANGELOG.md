# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.6] - 2026-11-05

### Added

- Escape closes the login and the bulk-confirm dialogs through the same path
  as their cancel buttons, and the login dialog opens with the email field
  focused (the code field at the two-factor step).
- A show/hide toggle on the login dialog's master password field.
- Empty state on the Browsers page when no supported browser is installed, a
  fixed page header with a scrolling body on the Account and Settings pages,
  and a divider between stored accounts.
- The sidebar's chosen item wears the accent's soft wash with a rounded bar at
  its leading edge, and its icon takes the same readable ink as the label.

### Changed

- mygo v0.7.4: no API removed and no `Theme` or `Tester` field changed from
  v0.7.1, so the pages keep their shapes.
- The README now answers the two questions a reader has before installing -
  how Hello treats the key, and why the host answers to the official connection
  name - and then says how to install and use it. The protocol walkthrough,
  the extraction-script fallback, the known limitations and the uninstall steps
  moved to `docs/how-it-works.md` and `docs/troubleshooting.md`; the credits
  line points at `THIRD_PARTY_NOTICES.md`, which already carried them.
- The Settings page's tray and log-level rows say what they actually reach:
  the tray choice lasts the session, the level affects this page only.
- The login flow asks for a code instead of calling back: `Start`, `Continue`
  and `SendEmail` return a result, and the solver callbacks are gone along
  with the adapter that pre-filled their answers.
- One assembly point builds the credential store, the Hello verifier and the
  registry once per process, so the desktop runs a single verifier rather than
  two, each with its own thread.
- The stored login session, its refresh retry and the language marker moved out
  of the UI package into `internal/session` and the registry seam, leaving the
  GUI with no direct call into a platform API.

### Fixed

- The desktop and the host no longer fight over one log file: the GUI appends
  to `gui.log`, the Log page shows both, and a rotation that cannot rename
  falls back to copying, so neither file can grow without bound.
- The scrolling pages leave the scrollbar its room, so it no longer sits on
  the buttons and the text.
- The registry is no longer re-read on every animation frame; the Browsers
  page's tick reads at most every two seconds, always on the first frame, and
  not at all while another page is on screen.
- Switching to dark no longer left the sidebar's chosen row in the light
  accent wash.

## [Unreleased]

## [0.1.5] - 2026-10-29

### Added

- A dark appearance. The pages and the two dialogs take their sizes and colors
  from one set of design tokens instead of repeating them inline, and the
  tokens carry two palettes: the app follows the desktop's light or dark mode
  and repaints as it changes. The layout and screenshot tests now run in both
  appearances, so text that no longer fits in the dark palette is caught in
  the tests.
- An Appearance row on the Settings page: follow the system, light, or dark.
  The choice is remembered per user and reaches the theme before the first
  window, so a forced appearance never shows a frame of the system's own first.
- The status line above the pages and in the login window now carries a
  severity (info, warning, danger) taken from the kind behind it rather than
  from its words, and draws it as a soft wash under the row, a mark at the
  leading edge, and the severity's ink on the text.
- Buttons come in five levels instead of two: primary (the one accent-filled
  action per page or dialog), secondary, plain, danger (the only red fill the
  app draws) and link. Destructive actions such as removing a key or
  unregistering a browser are red now instead of looking like any other
  button.
- The session check now says which of the three ways it can fail applies:
  network trouble, a rate-limited server, or a server that did not answer
  properly, each with its own wording in all seven languages, plus a
  **Check again** button and a bounded silent retry (30 s / 2 min / 5 min) so
  a flaky start no longer sticks until the next relaunch.

### Changed

- Upgraded mygo to v0.7.1 (no public API removed or renamed; the module now
  requires Go 1.27.3, and the generated catalog imports mygo's own Japanese
  strings). The installer asset changed with it: it is now
  `biowarden-setup-<version>-amd64.exe` instead of the old
  `biowarden Setup <version> amd64.exe`, so the name no longer needs rewriting
  on upload.
- Releases now publish the installer and a `SHA256SUMS.txt` only. The portable
  `biowarden.exe` is no longer uploaded; it stays a local build artifact for
  development and testing.
- Pre-publication documentation pass: the READMEs disclose why the host
  registers under the official `com.8bit.bitwarden` connection name and drop
  the "impersonating" wording; THIRD_PARTY_NOTICES.md resolves its unverified
  license entries (bw-bio-handler is MIT, WardenBio declares none) and lists
  x/crypto correctly; a SECURITY.md spells out the threat model; the stale
  version and dependency facts are fixed; AGENTS.md stops pointing at files
  that do not exist; CHANGELOG gains compare links; CI runs the acceptance
  command on windows-latest and the bug template asks for the log up front.

### Fixed

- The session check no longer starves under its own timeout: it allows 12 s
  instead of 5 s and retries the transient failures, so a slow first hop
  (server config plus token exchange through a CDN) no longer reads as
  "cannot verify". A 403 page from a gateway no longer deletes the stored
  login session, and 429 is told apart from a dead server. Failures are
  logged (attempts, classification, elapsed) instead of vanishing.
- The status badges on the Browsers page drew a tone's own color on the tone's
  own wash, which sat under the 4.5:1 a label needs, so "installed" read as a
  barely visible blue. A registered manifest is green now (the accent blue
  means "the action here") and the ink is derived from the wash until the pair
  clears 4.5:1 in both appearances.
- The Appearance row on the Settings page spread its three-option segment
  across the whole page with the options squeezed into a corner; it now takes
  its content width, the way the language list above it already did.

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

[unreleased]: https://github.com/windedge/biowarden/compare/v0.1.6...HEAD
[0.1.6]: https://github.com/windedge/biowarden/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/windedge/biowarden/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/windedge/biowarden/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/windedge/biowarden/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/windedge/biowarden/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/windedge/biowarden/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/windedge/biowarden/compare/6f255fc8cb6d8e4a15cafa279f5b49c965999442...v0.1.0
