# Contributing

Biowarden is a Windows-only Go application with a MyGo GUI. Small, focused changes are easiest to
review; a bug report with a `host.log` excerpt is just as welcome as a patch.

## Reporting a bug

Open an issue - the template asks for the excerpt, versions, browser and whether the official
desktop client is installed, because those four answer most reports on their own. Before that,
[docs/troubleshooting.md](./docs/troubleshooting.md) covers the three reasons an unlock prompt never
appears.

Security problems do **not** go in the issue tracker. See [SECURITY.md](./SECURITY.md).

## Building

Requirements and the packaging flow are in [docs/build.md](./docs/build.md). In short:

```sh
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... && go test ./...
```

`CGO_ENABLED=0` is not a preference - the project must build without cgo, and CI checks it.

## What a change must keep green

| Command | What it guards |
|---|---|
| `CGO_ENABLED=0 go build ./...` | no cgo, no platform-specific breakage |
| `CGO_ENABLED=0 go vet ./...` | the usual static checks |
| `go test ./...` | including the layout tests: every page, in all seven languages, in both appearances |
| `go tool mygo generate` then `git diff --exit-code` | generated catalogs stay in step with the JSON |
| `go tool mygo i18n check` | every locale carries the same keys |

The interface has seven languages, so a new string is not done until it exists in all seven files
under `locales/`. `en.json` carries the `@` descriptions translators read. Please do not let
punctuation drift: the Chinese locales use full-width marks, and the project uses ASCII hyphens
rather than typographic dashes everywhere.

## Style

- Plain, declarative commit subjects (`fix(ui): ...`, `refactor(auth): ...`), a body that explains
  why rather than what.
- Comments explain what the code cannot say. A comment that restates the line above it is noise.
- Tests go beside the code they cover; a behaviour that lives in the protocol or the unlock path
  needs one, because those are the parts a harness cannot prove by hand.
- Never write the real registry from a test. The registry tests load a private app hive.

## Architecture notes

[docs/how-it-works.md](./docs/how-it-works.md) explains the two modes, the two extension channels
and where the key lives - read it before touching `internal/protocol` or the unlock flow.
[AGENTS.md](./AGENTS.md) records the constraints that must not bend: no cgo, redacted logs, the
biometrics model worded precisely, and the four protocol pitfalls.
