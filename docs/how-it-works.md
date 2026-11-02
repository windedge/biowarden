# How Biowarden works

```
browser extension -stdio(native messaging)-> Biowarden host -> Windows Hello -> Credential Manager
```

## One executable, two modes

When stdin is a pipe, Biowarden runs as the native messaging host (started by the browser on
demand). Started any other way it opens the GUI: a tray icon plus a window with a left sidebar of
four pages - **Account** / **Browsers** / **Settings** / **Log**.

The Settings page carries the appearance (follow the system, light or dark, remembered in
`HKCU\Software\biowarden\Appearance`), the language, autostart at login, and a Hello availability
check with its own retest button. Launching at login writes a value under the user's `Run` key; the
tray icon choice is per run and says so.

## The login link

The login window types the email and master password into Biowarden, which talks to the Bitwarden
identity and API endpoints while presenting itself as the official desktop client
(`client_id=desktop`, `deviceType=6`, `deviceName=windows`). It runs prelogin to read the KDF
parameters (PBKDF2-SHA256 or Argon2id), requests a token, derives the master key, decrypts the
wrapped 64-byte userKey, and only then writes it to the Credential Manager. Windows Hello gates
that write, so a refused prompt leaves the store untouched.

## Two channels to the extension

- **The legacy command protocol** (today's main path). `setupEncryption` hands the host a 64-byte
  transport key wrapped in RSA-OAEP-SHA1; everything after that is AES-256-CBC + HMAC, and every
  `message` must carry both `encryptedString` (`2.<iv>|<data>|<mac>`) and iv/data/mac.
- **The newer desktop IPC**, used once the extension's remote `BiometricsSDKIPC` flag enables it. A
  `Noise_NN_P256_AESGCM_SHA256` handshake with CBOR frames and AES-256-GCM payloads, then JSON RPC
  (`DiscoverRequest`, `GetBiometricsStatus`, `UnlockBiometrics`, `AuthenticateBiometrics`) under
  the `forwarded-bitwarden-ipc-message` envelope.
- The extension opens both connections, so two host processes per browser is normal.

## Where the key lives

The key is held in the Windows Credential Manager as `Biowarden/<userId>`, one entry per account,
plus a `Biowarden/Index` metadata entry. The GUI and the host share this store, so a key saved by a
login in the GUI is the one the host reads at unlock time.

## Security model

**Windows Hello gates *access* to the key, it does not encrypt it.** The account key sits in the
Windows Credential Manager as raw bytes. Passing the Hello check only means Biowarden will read
that entry; nothing about the stored key is derived from your biometrics or PIN. Any other process
running as you can in principle read the same credential; that is the same threat model you
already accept with the official desktop client.

The master password you type in the login window lives in memory only. It is kept for the length of
the run so signing in (or signing back in) can re-derive the key after a password or KDF change,
and it is never written to disk or to a log. The access token is memory-only too.

To keep you signed in across restarts, Biowarden persists the refresh token, your email and your
server address as a separate `Biowarden/Session/<userId>` entry, encrypted by Windows DPAPI under
your user account; that entry holds no master password and no account key. The two-factor code is
dropped as soon as one attempt settles. Because only the refresh token is kept, re-deriving the key
still requires typing your master password again. A clean **Log out** deletes the stored key and the
stored session after a Windows Hello consent.

## Logs and language

Logs go to `%LOCALAPPDATA%\biowarden\host.log` (rotated at 5 MB, secret fields always redacted);
set `BIOWARDEN_LOG=debug` for protocol-level detail.

The interface language follows the system's language (English, Simplified Chinese, Traditional
Chinese, Japanese, German, French or Spanish); the Settings page's **Language** row overrides it.

## Repository layout

```
cmd/biowarden/          dual-mode entry (mode detection, dispatch)
internal/auth/          Bitwarden login link (prelogin/KDF/token/userKey, TOTP/email/YubiKey 2FA)
internal/protocol/      frames, legacy command protocol, SDK IPC (Noise/CBOR/RPC)
internal/host/          serve loop: dispatch, unlock flow, logging
internal/hostlog/       the redacting log writer both processes append to
internal/hoststate/     per-browser connection history (which browser pulled the host in)
internal/{hello,secrets,registry,autostart}/   platform seams
internal/ui/            GUI (login window, first-run setup, real account backend, pages)
```

Reference implementations: WardenBio (macOS/Swift), bw-bio-handler, goldwarden. See
[THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) for licenses, copyright lines and how much of
each one is used.
