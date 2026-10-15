# biowarden

English · [简体中文](./README.zh-CN.md)

> Biometric unlock for the Bitwarden browser extension on Windows - one small exe instead of the
> Bitwarden Desktop app. It stands in for the desktop client and gates your account key with
> Windows Hello.

biowarden is a Windows port of [WardenBio](https://github.com/xVanTuring/WardenBio). It ships its
own native messaging host, embedded in the same executable and launched by the browser on demand.
As far as the browser extension is concerned, it *is* the desktop client: when the extension asks
for a biometric unlock, biowarden shows a Windows Hello prompt, then reads that account's
encryption key from the Windows Credential Manager and hands it back over the channel the
extension set up.

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
  window with four pages: **浏览器** / **密钥** / **设置** / **日志** (Browsers / Keys / Settings / Log).
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
  plus a `Biowarden/Index` metadata entry.
- Logs go to `%LOCALAPPDATA%\biowarden\host.log` (rotated at 5 MB, secret fields always redacted);
  set `BIOWARDEN_LOG=debug` for protocol-level detail.

**Security model: Windows Hello gates *access* to the key, it does not encrypt it.** The account
key sits in the Windows Credential Manager as raw bytes. Passing the Hello check only means
biowarden will read that entry - nothing about the stored key is derived from your biometrics or
PIN. Any other process running as you can in principle read the same credential; that is the same
threat model you already accept with the official desktop client.

## Installation

1. Download `biowarden Setup <version> amd64.exe` from this repository's Releases page and run it
   (per-user install), or download the portable `biowarden.exe` and put it somewhere stable - the
   native messaging manifest records its absolute path, so **re-install the manifest if you move it**.
2. Launch biowarden and follow "Usage" below.
3. Prefer building it yourself? See "Build from source".

Requires Windows 10 1709 or later with Windows Hello configured.

## Usage

1. Run `biowarden.exe` and click **安装** (Install) next to your browser on the **浏览器**
   (Browsers) page. Chrome installs write both the `Google\Chrome` and the `Chromium` registry key;
   biowarden versions before 2026.1 only wrote the former.
2. With the extension unlocked, click **复制提取脚本** (copy extraction script) on the **密钥**
   (Keys) page and paste the script into the extension's background console (Chrome:
   `chrome://extensions` -> the Bitwarden service worker; Firefox: `about:debugging`). It prints
   `{userId, keyB64}`. Bitwarden 2026.8.x keeps the key in session storage (wrapped as `{__json__}`)
   or, under a "never lock" policy, in local `<userId>_user_auto` - the script handles both, and
   says why it found nothing otherwise. Paste both values back on the **密钥** page, save, and
   approve the Windows Hello prompt.
3. In the Bitwarden extension, Settings -> Security -> enable "Unlock with biometrics" (this grants
   the `nativeMessaging` permission and reloads the extension). **Restart the browser completely
   after installing or repairing the manifest** - browsers read the `NativeMessagingHosts`
   registry keys only at startup.
4. Lock the extension, click unlock: Windows Hello appears (centered on the screen) and passing it
   unlocks the vault.

## Known limitations

- **The official Bitwarden desktop client conflicts with biowarden.** It unconditionally rewrites
  the `NativeMessagingHosts` registry keys for every browser each time it starts, taking the
  registration back. The **浏览器** page shows a red "overwritten by another program" state and
  offers one-click recovery, but the real fix is not to run the official desktop app alongside.
- The extraction script depends on the extension's internal storage layout
  (`chrome.storage.session`'s `user_<id>_crypto_userKey`). A major Bitwarden extension update can
  change it; the script reports the reason instead of failing silently.
- v1 has registration points for Chrome, Chromium, Edge and Firefox. Edge and Firefox still await
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

```sh
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... && go test ./...
```

## Uninstall

- **浏览器** page: click **卸载** (Uninstall) for every configured browser (removes the manifest
  and the registry value)
- **密钥** page: delete each stored account key
- Quit biowarden (tray menu -> Exit) and delete the program and its installer

## Disclaimer

Not an official Bitwarden project, and not affiliated with Bitwarden or 8bit Solutions LLC in any
way. Unlock problems belong here, not in Bitwarden's issue tracker. The key you store is your
vault's encryption key: never keep a plaintext copy anywhere. Use at your own risk.

## Repository layout

```
cmd/biowarden/          dual-mode entry (mode detection, dispatch)
internal/protocol/      frames, legacy command protocol
internal/protocol/sdkipc/   desktop IPC (Noise/CBOR/JSON RPC)
internal/host/          serve loop: dispatch, unlock flow, logging
internal/{hostlog,hello,secrets,registry,autostart}/   platform seams
internal/ui/            GUI (tray + browsers / keys / settings / log pages)
```

Reference implementations: WardenBio (macOS/Swift), bw-bio-handler, goldwarden.
