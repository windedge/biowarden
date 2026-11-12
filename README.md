# Biowarden

[![Latest release](https://img.shields.io/github/v/release/windedge/biowarden)](https://github.com/windedge/biowarden/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](./LICENSE)

English · [简体中文](./README.zh-CN.md)

Biometric unlock for the Bitwarden browser extension on Windows - one small exe instead of the
Bitwarden Desktop app. Sign in inside Biowarden, and Windows Hello gates the account key. Dark and
light appearances, seven interface languages, no account key ever written in plain text.
Not an official Bitwarden project; please don't report unlock problems to the Bitwarden team.

**Windows Hello gates *access* to the key, it does not encrypt it.** The key sits in the Windows
Credential Manager as raw bytes; passing the prompt only means Biowarden may read it. That is the
same model you already accept with the official desktop client.

![The Biowarden account page](./docs/images/screenshot-light.png)

## Install

1. Download `biowarden-setup-<version>-amd64.exe` from
   [Releases](https://github.com/windedge/biowarden/releases) and run it (per-user install). The
   installer is the only thing published there, next to a `SHA256SUMS.txt`.
2. Check the download against it:

   ```powershell
   (Get-FileHash .\biowarden-setup-<version>-amd64.exe).Hash   # compare with SHA256SUMS.txt
   ```

   On a shell with coreutils, `sha256sum -c SHA256SUMS.txt` does the same.

3. Launch Biowarden. On a fresh machine it registers your browsers and opens the login window.

Nothing is code-signed, so first run shows SmartScreen's "Windows protected your PC": click
**More info** -> **Run anyway**. The hash check above is your guard against a tampered download.
Requires Windows 10 1709 or later with Windows Hello set up.

The supported extension browsers are Chrome, Edge and Firefox; Brave and Vivaldi are not
supported yet - see [Known limitations](./docs/troubleshooting.md#known-limitations).

## Use

1. **Sign in** - email and master password in the login window. The server address is under
   **Advanced**; it defaults to the official Bitwarden cloud and accepts any self-hosted address
   (Vaultwarden included). TOTP, email and YubiKey second factors are supported; WebAuthn and Duo
   are not.
2. **Confirm with Windows Hello** - the prompt is what writes the key. Refusing stores nothing.
3. **Enable biometrics in the extension** - Settings -> Security -> "Unlock with biometrics", then
   restart the browser completely; browsers read the registration only at startup.

Lock and unlock in the extension as usual. **Log out** on the Account page deletes the stored key
and the session.

Something not working? [Troubleshooting](./docs/troubleshooting.md) starts with the three reasons
an unlock prompt never appears, before it gets to the extraction-script fallback.

## FAQ

### Is Biowarden an official Bitwarden product?

No. Biowarden is an independent project with no affiliation to Bitwarden Inc. The name
`com.8bit.bitwarden` is the appId hard-coded in the official extension, and Biowarden only takes
over that registry entry so the extension can find its host - see [The connection name](#the-connection-name).

### Does it upload my passwords?

No. Network requests go only to your own Bitwarden server (or Vaultwarden); `internal/auth` is the
only package that makes HTTP calls. There is no telemetry or analytics, and the source is open to
audit.

### Why do I have to type my master password?

Biowarden is the keychain for your password manager: it derives the userKey from your master
password and stores that key in the Windows Credential Manager. The master password itself is never
written to disk (derived, then dropped); the access token is memory-only too. Only the refresh
token is persisted, as the DPAPI-encrypted `Biowarden/Session/<userId>` entry. See
[Security model](./docs/how-it-works.md#security-model).

### Does Windows Hello encrypt my key?

No. Hello only gates access to reading the `Biowarden/<userId>` credential entry; the key is
stored as raw bytes (wrapped by Credential Manager's own DPAPI, readable by your OS user). Hello
is a gatekeeper, not a vault. See [Security model](./docs/how-it-works.md#security-model).

### Why not use the official desktop client?

The official client is a full Electron app - hundreds of MB, a resident background process,
auto-updates. Biowarden is a native messaging host started on demand: tens of MB of memory,
millisecond startup, no resident UI process (the GUI is a separate process opened only when
needed). It suits people who use only the browser extension.

### How do I verify the installer was not tampered with?

Every release ships a `SHA256SUMS.txt`; check the download with `Get-FileHash -Algorithm SHA256`
in PowerShell (or `sha256sum -c`). The full source is on GitHub and CI builds it on every push, running
`go build`, `go vet`, `go test` and `mygo generate` on windows-latest. Nothing is code-signed, so
SmartScreen will warn - the hash check is your guard.

## Docs

| | |
|---|---|
| [How it works](./docs/how-it-works.md) | protocol, key storage, security model, repository layout |
| [Troubleshooting](./docs/troubleshooting.md) | unlock prompt missing, extraction script, limitations, uninstall |
| [Building](./docs/build.md) | from source, packaging, releasing |
| [Security policy](./SECURITY.md) | how to report a vulnerability, what is in scope |
| [Third-party notices](./THIRD_PARTY_NOTICES.md) | licenses and the origin of ported work |
| [Contributing](./CONTRIBUTING.md) | how to report, build and send a change |
| [Working on this repository](./docs/agents-brief.md) | where each concern lives, and the checks a change must keep green |

## The connection name

The host registers under `com.8bit.bitwarden`, because that name is hard-coded in the extension.
Taking that registration over is the whole cost of not forking or patching it. An existing
registration is shown as "possibly the official client" and only replaced after you confirm.

## Disclaimer

Not affiliated with Bitwarden or 8bit Solutions LLC in any way. Bitwarden is a trademark of
Bitwarden Inc.; the connection name `com.8bit.bitwarden` appears here only because the browser
extension requires that exact name. The key you store is your vault's encryption key: never keep a
plaintext copy anywhere. Use at your own risk. Released under the [MIT license](./LICENSE).
