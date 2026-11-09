# biowarden

Biometric unlock for the Bitwarden browser extension on Windows (Go + MyGo), porting the macOS
WardenBio implementation. It impersonates the Bitwarden desktop client: gates the account
key with Windows Hello, then returns it over native messaging. Not an official Bitwarden project.

## Architecture

One executable, two modes. stdin is a pipe (`GetFileType(stdin) == FILE_TYPE_PIPE`) means host
mode (native messaging host, launched by the browser on demand); anything else means GUI mode
(MyGo tray + four pages: Account / Browsers / Settings / Log).

```
cmd/biowarden/          dual-mode entry (mode detection, dispatch)
internal/auth/          Bitwarden login link (prelogin/KDF/token/userKey, TOTP/email/YubiKey 2FA)
internal/protocol/      frames, legacy command protocol, SDK IPC (Noise/CBOR/RPC)
internal/host/          serve loop: dispatch, unlock flow, logging
internal/hoststate/     per-browser connection history (which browser pulled the host in)
internal/{hello,secrets,registry}/   platform seams (interfaces + build tags)
internal/ui/            GUI: pages, login window, first-run setup, account backends
```

Data flow: `internal/ui` logs the user in through `internal/auth` (prelogin/KDF/token, then 2FA),
and the resulting userKey is written to `internal/secrets` only after a `internal/hello` consent,
so a refused prompt leaves nothing stored. The host and the GUI are separate processes sharing the
same Credential Manager store, so a key saved by a GUI login is the one the host reads at unlock.

Seams: `hello` = Windows Hello consent via pure syscalls; `secrets` = Credential Manager,
`Biowarden/<userId>` per account plus a `Biowarden/Index` metadata entry, and
`Biowarden/Session/<userId>` for the persisted login (refresh token, email, server, never the
password or userKey); `registry` = HKCU-only
manifest registration for Chrome/Edge/Firefox, showing an existing value as "possibly the official
client" and only overwriting after confirmation, plus the `FirstRunCompleted` marker that keeps
the zero-configuration first-run setup to one automatic registration. v1 implements `windows` only.

## Rules

- No cgo: `CGO_ENABLED=0` must always build.
- Tests must never write the real registry: inject a private root key (the registry tests load
  an app hive) instead of touching HKCU/HKLM.
- Logs redact long secrets.
- Biometrics gate *access* to a key stored in the OS credential store; they encrypt nothing. Never
  word the model stronger than that.
- Read "Four things the protocol will not forgive" in `docs/how-it-works.md` before touching
  protocol code: the four pitfalls (64-byte transport key, `encryptedString` alongside
  `iv`/`data`/`mac`, `appId`/`messageId` echo, `forwarded-bitwarden-ipc-message` envelope) are
  unforgiving, and each one shows up as a spinner rather than an error.

## Verify

```bash
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... && go test ./...
go tool mygo generate && git diff --exit-code
go tool mygo i18n check
```

Acceptance is a real browser with the official Bitwarden extension, not a harness alone.

Before exploring, read [`docs/agents-brief.md`](docs/agents-brief.md): where each concern lives, the
MyGo calls worth not rediscovering, and the tests a change must keep green.

## Tracking

Locked decisions and open work live on the GitHub issue tracker: a map issue holding the notes and
its child issues. Do not commit unless asked.

## Agent skills

### Issue tracker

Open work lives in GitHub Issues of windedge/biowarden, driven with the `gh` CLI. No triage
configuration of its own is kept in this repository.

### Domain docs

`CONTEXT.md` and `docs/adr/` do not exist yet; protocol invariants currently live in
`docs/how-it-works.md`. When a decision is worth keeping, record it as an ADR under `docs/adr/`
(create the directory when the first one lands).
