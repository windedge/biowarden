# biowarden

Biometric unlock for the Bitwarden browser extension on Windows (Go + MyGo), porting the macOS
WardenBio implementation. It impersonates the Bitwarden desktop client: gates the account
key with Windows Hello, then returns it over native messaging. Not an official Bitwarden project.

## Architecture

One executable, two modes. stdin is a pipe (`GetFileType(stdin) == FILE_TYPE_PIPE`) means host
mode (native messaging host, launched by the browser on demand); anything else means GUI mode
(MyGo tray + four pages: Browsers / Keys / Settings / Log).

```
cmd/biowarden/          dual-mode entry (mode detection, dispatch)
internal/protocol/      frames, legacy command protocol, SDK IPC (Noise/CBOR/RPC)
internal/host/          serve loop: dispatch, unlock flow, logging
internal/{hello,secrets,registry}/   platform seams (interfaces + build tags)
internal/ui/            GUI
```

Seams: `hello` = Windows Hello consent via pure syscalls; `secrets` = Credential Manager,
`Biowarden/<userId>` per account plus a `Biowarden/Index` metadata entry; `registry` = HKCU-only
manifest registration for Chrome/Edge/Firefox, showing an existing value as "possibly the official
client" and only overwriting after confirmation. v1 implements `windows` only.

## Rules

- No cgo: `CGO_ENABLED=0` must always build.
- Logs redact long secrets.
- Biometrics gate *access* to a key stored in the OS credential store; they encrypt nothing. Never
  word the model stronger than that.
- Read the WardenBio `AGENTS.md` before touching protocol code: the four pitfalls (64-byte transport
  key, `encryptedString` alongside `iv`/`data`/`mac`, `appId`/`messageId` echo,
  `forwarded-bitwarden-ipc-message` envelope) are unforgiving.

## Verify

```bash
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... && go test ./...
```

Acceptance is a real browser with the official Bitwarden extension, not a harness alone.

## Tracking

Locked decisions and open work live on the Wayfinder map issue and its child issues.
Do not commit unless asked.

## Agent skills

### Issue tracker

GitHub Issues of windedge/biowarden via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
