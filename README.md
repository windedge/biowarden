# biowarden

English · [简体中文](./README.zh-CN.md)

> Biometric unlock for the Bitwarden browser extension on Windows - one small exe instead of the
> Bitwarden Desktop app. Sign in inside biowarden, and it gates the account key with Windows Hello.

biowarden is a Windows port of [WardenBio](https://github.com/xVanTuring/WardenBio). It ships its
own native messaging host, embedded in the same executable and launched by the browser on demand.
As far as the browser extension is concerned, it *is* the desktop client: when the extension asks
for a biometric unlock, biowarden shows a Windows Hello prompt, then reads that account's
encryption key from the Windows Credential Manager and hands it back over the channel the
extension set up.

You no longer need to copy a script out of the extension console. On a fresh machine biowarden
logs into your Bitwarden account itself, derives the account key, stores it behind a Windows Hello
consent, and the extension's "Unlock with biometrics" is ready. The old extraction script is still
there under the account page's **Advanced** disclosure as a troubleshooting and compatibility path.

The protocol follows [quexten/bw-bio-handler](https://github.com/quexten/bw-bio-handler) and
[quexten/goldwarden](https://github.com/quexten/goldwarden) (a subset of the Bitwarden
Desktop <-> browser IPC protocol). This is not an official Bitwarden project - please don't report
unlock problems to the Bitwarden team.

## How it works

```
browser extension -stdio(native messaging)-> biowarden host -> Windows Hello -> Credential Manager
```

- One executable, two modes. When stdin is a pipe, biowarden runs as the native messaging host
  (started by the browser on demand); started any other way it opens the GUI - a tray icon plus a
  window with a left sidebar of four pages: **Account** / **Browsers** / **Settings** / **Log**.
- The login link is real. The login window types the email and master password into biowarden,
  which talks to the Bitwarden identity and API endpoints while impersonating the official desktop
  client (`client_id=desktop`, `deviceType=6`, `deviceName=windows`). It runs prelogin to read the
  KDF parameters (PBKDF2-SHA256 or Argon2id), requests a token, derives the master key, decrypts
  the wrapped 64-byte userKey, and only then writes it to the Credential Manager. Windows Hello
  gates that write, so a refused prompt leaves the store untouched.
- Two channels to the extension:
  - the legacy command protocol (today's main path): `setupEncryption` hands the host a 64-byte
    transport key wrapped in RSA-OAEP-SHA1; everything after that is AES-256-CBC + HMAC, and every
    `message` must carry both `encryptedString` (`2.<iv>|<data>|<mac>`) and iv/data/mac
  - the newer desktop IPC, used once the extension's remote `BiometricsSDKIPC` flag enables it:
    a `Noise_NN_P256_AESGCM_SHA256` handshake with CBOR frames and AES-256-GCM payloads, then JSON
    RPC (`DiscoverRequest`, `GetBiometricsStatus`, `UnlockBiometrics`, `AuthenticateBiometrics`)
    under the `forwarded-bitwarden-ipc-message` envelope
  - the extension opens both connections, so two host processes per browser is normal
- The key is held in the Windows Credential Manager as `Biowarden/<userId>`, one entry per account,
  plus a `Biowarden/Index` metadata entry. The GUI and the host share this store, so a key saved by
  a login in the GUI is the one the host reads at unlock time.
- Logs go to `%LOCALAPPDATA%\biowarden\host.log` (rotated at 5 MB, secret fields always redacted);
  set `BIOWARDEN_LOG=debug` for protocol-level detail.
- The interface language follows the system's language (English, Simplified Chinese, Traditional
  Chinese, Japanese, German, French or Spanish); the
  Settings page's **Language** row overrides it.

**Security model: Windows Hello gates *access* to the key, it does not encrypt it.** The account
key sits in the Windows Credential Manager as raw bytes. Passing the Hello check only means
biowarden will read that entry - nothing about the stored key is derived from your biometrics or
PIN. Any other process running as you can in principle read the same credential; that is the same
threat model you already accept with the official desktop client.

The master password you type in the login window lives in memory only. It is kept for the length
of the run so signing in (or signing back in) can re-derive the key after a password or KDF
change, and it is never written to disk or to a log. The access token is memory-only too. To keep
you signed in across restarts, biowarden persists the refresh token, your email and your server
address as a separate `Biowarden/Session/<userId>` entry, encrypted by Windows DPAPI under your
user account; that entry holds no master password and no account key. The two-factor code is
dropped as soon as one attempt settles. Because only the refresh token is kept, re-deriving the
key still requires typing your master password again. A clean **Log out** deletes the stored key
and the stored session after a Windows Hello consent.

## Installation

1. Download `biowarden Setup <version> amd64.exe` from this repository's Releases page and run it
   (per-user install), or download the portable `biowarden.exe` and put it somewhere stable - the
   native messaging manifest records its absolute path, so **re-install the manifest if you move it**.
2. Launch biowarden. On a fresh machine it configures itself; see "Usage" below.
3. Prefer building it yourself? See "Build from source".

Requires Windows 10 1709 or later with Windows Hello configured.

## Usage

1. **First launch registers your browsers.** When no key is stored and no browser is registered to
   biowarden yet, the first launch probes for installed browsers (through their App Paths entries)
   and writes its manifest for each one it finds. This happens once per installation, marked under
   `HKCU\Software\biowarden`. A browser whose registration may belong to another client (the
   official desktop app) is **skipped, not overwritten**, and the summary on the Browsers page
   names it; use the per-row confirmation there if you want to take it over. Later launches do
   nothing and show no login window.
2. **Sign in.** The first launch opens the login window. Enter your email and master password. The
   server address is under **Advanced**; it defaults to the official Bitwarden cloud and accepts a
   self-hosted address. Two-factor is supported for **TOTP (authenticator app), email code, and
   YubiKey**. **WebAuthn and Duo are not supported** yet - if your account requires them, cancel
   and use the extraction script in step 5's fallback instead.
3. **Confirm with Windows Hello.** After a successful sign-in, biowarden asks for Windows Hello
   consent and, once you approve, writes the account key to the Credential Manager. Refusing leaves
   nothing stored.
4. **Enable biometrics in the extension.** In the Bitwarden extension, Settings -> Security, enable
   "Unlock with biometrics" (this grants the `nativeMessaging` permission and reloads the
   extension). **Restart the browser completely after installing or repairing the manifest** -
   browsers read the `NativeMessagingHosts` registry keys only at startup.
5. **Lock and unlock.** Lock the extension, click unlock: Windows Hello appears (centered on the
   screen) and passing it unlocks the vault.

### Troubleshooting and compatibility: the extraction script

The manual path is kept for the cases the login flow cannot cover: an account protected by
WebAuthn or Duo, a self-hosted server whose endpoints biowarden cannot reach, or a broken login
that you want to diagnose.

On the **Account** page, expand **Advanced**. **Copy extraction script** copies a snippet that runs
in the extension's background console (Chrome: `chrome://extensions` -> the Bitwarden service
worker; Firefox: `about:debugging`) while the extension is unlocked. It prints `{userId, keyB64}`.
Bitwarden 2026.8.x keeps the key in session storage (wrapped as `{__json__}`) or, under a "never
lock" policy, in local `<userId>_user_auto` - the script handles both, and says why it found
nothing otherwise (the extension is locked, or the "never lock" policy is off, or the layout
changed). Paste both values back on the same page, save, and approve the Windows Hello prompt.

## Known limitations

- **The official Bitwarden desktop client conflicts with biowarden.** It unconditionally rewrites
  the `NativeMessagingHosts` registry keys for every browser each time it starts, taking the
  registration back. The **Browsers** page shows a red "overwritten by another program" state and
  offers one-click recovery, but the real fix is not to run the official desktop app alongside.
- **A manifest change needs a full browser restart.** Browsers read the registry values only at
  startup, so a fresh install or a repair takes effect on the next browser launch.
- **Chromium reads its own registry key.** A Chrome install on the Browsers page writes both the
  `Google\Chrome` and the `Chromium` key; biowarden versions before 2026.1 only wrote the former.
- **Only the zero-configuration first launch auto-registers.** Adding a browser you install later
  is a manual step on the Browsers page; later launches never register anything on their own.
- **WebAuthn and Duo second factors are not supported.** biowarden has no embedded web vault and
  does not register the `bitwarden://` protocol, so neither connector's callback can reach it.
  Accounts that require them need the extraction script fallback.
- The extraction script depends on the extension's internal storage layout
  (`chrome.storage.session`'s `user_<id>_crypto_userKey`). A major Bitwarden extension update can
  change it; the script reports the reason instead of failing silently.
- v2 has registration points for Chrome, Chromium, Edge and Firefox. Edge and Firefox still await
  real-browser acceptance testing. Brave, Vivaldi, Safari and other browsers are not supported.
- Host processes stay alive per connection (two per browser), and the extension polling once a
  second while locked is normal.
- An unlock request blocks that connection while the Hello prompt is up, exactly as the official
  desktop client does.

## Build from source

Requirements: Go 1.27+, and NSIS for the installer (on Windows `mygo build` downloads NSIS 3.13
into `%LOCALAPPDATA%\mygo` when `makensis` is missing).

```powershell
pwsh scripts/build.ps1
```

Output under `build/windows-amd64/`: `biowarden.exe`, `biowarden Setup 0.1.0 amd64.exe` and
`icon.png`. The script builds into a staging directory, stops any running biowarden (GUI or host),
and swaps the new build in - a running exe can be renamed but not overwritten. Plain
`go tool mygo build` works too when nothing holds the executable.

The version lives in two places that must be kept in sync when releasing: `mygo.json`'s `version`
(the build links it into the binary and names the installer after it) and `internal/ui/ui.go`'s
`appVersion` (shown in the sidebar footer and on the Settings page). Both are `0.1.0` today.

```sh
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... && go test ./...
```

More build detail is in [docs/build.md](./docs/build.md).

## Uninstall

- **Browsers** page: click **Uninstall** for every configured browser (removes the manifest
  and the registry value)
- **Account** page: delete each stored account key (Hello consent required), or use **Log out**
- Quit biowarden (tray menu -> Exit) and delete the program and its installer

## Disclaimer

Not an official Bitwarden project, and not affiliated with Bitwarden or 8bit Solutions LLC in any
way. Unlock problems belong here, not in Bitwarden's issue tracker. The key you store is your
vault's encryption key: never keep a plaintext copy anywhere. Use at your own risk.

## Repository layout

```
cmd/biowarden/          dual-mode entry (mode detection, dispatch)
internal/auth/          Bitwarden login link (prelogin/KDF/token/userKey, TOTP/email/YubiKey 2FA)
internal/protocol/      frames, legacy command protocol, SDK IPC (Noise/CBOR/RPC)
internal/host/          serve loop: dispatch, unlock flow, logging
internal/hoststate/     per-browser connection history (which browser pulled the host in)
internal/{hello,secrets,registry,autostart}/   platform seams
internal/ui/            GUI (login window, first-run setup, real account backend, pages)
```

Reference implementations: WardenBio (macOS/Swift), bw-bio-handler, goldwarden.

## Credits

biowarden stands on the work of others:

- [goldwarden](https://github.com/quexten/goldwarden) - MIT, Copyright (c) 2023 Bernd Schoolmann.
  The login, KDF and encryption implementation is referenced and partly ported from it.
- [WardenBio](https://github.com/xVanTuring/WardenBio) - the macOS/Swift implementation biowarden
  is a Windows port of.
- [bw-bio-handler](https://github.com/quexten/bw-bio-handler) - the legacy command protocol.

Dependencies and their licenses:

- [github.com/danieljoos/wincred](https://github.com/danieljoos/wincred) v1.2.3 - MIT
- [github.com/egoist/mygo](https://github.com/egoist/mygo) v0.4.8 - MIT
- [golang.org/x/sys](https://cs.opensource.google/go/x/sys) v0.48.0 - BSD-3-Clause
- [github.com/ebitengine/purego](https://github.com/ebitengine/purego) v0.11.1 - Apache-2.0
- [github.com/go-text/typesetting](https://github.com/go-text/typesetting) v0.3.5 - Unlicense OR
  BSD-3-Clause
- [golang.org/x/image](https://cs.opensource.google/go/x/image) v0.46.0 - BSD-3-Clause

See [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md) for full license texts and copyright lines.
