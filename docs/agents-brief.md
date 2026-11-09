# Working on this repository

A short brief for an agent picking up a task here. Read this before exploring.

## The check that defines done

```sh
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... && go test ./...
go tool mygo generate && git diff --exit-code      # catalogs must be regenerated and committed
go tool mygo i18n check                            # every locale carries the same keys
```

`CGO_ENABLED=0` is not a preference: the project must build without cgo.

## Where things are

| Looking for | File |
|---|---|
| entry point, mode detection | `cmd/biowarden/` |
| protocol frames, legacy command channel, SDK IPC | `internal/protocol/`, `internal/protocol/sdkipc/` |
| host serve loop, unlock flow | `internal/host/` |
| login, KDF, token, 2FA | `internal/auth/` |
| GUI pages, state, backends | `internal/ui/` (`pages_*`, `state_*`, `backend_*`) |
| design tokens, palettes | `internal/ui/style.go` |
| button levels, badges | `internal/ui/widgets.go` |
| platform seams (Hello, credential store, registry) | `internal/{hello,secrets,registry}/` |
| stored login session, refresh | `internal/session/` |
| profile directory layout | `internal/paths/` |
| strings, seven locales | `locales/*.json` (source: `en.json`) |

Architecture and protocol detail: [how-it-works.md](./how-it-works.md).

## MyGo facts worth not rediscovering

MyGo is the UI framework (`github.com/egoist/mygo`, version in `go.mod`). Its source lives in the
module cache, at `$(go env GOMODCACHE)/github.com/egoist/mygo@<version>/`.

- `c.SetTheme(&t)` sets the frame's theme. `applyPalette(c)` in `internal/ui/style.go` does this
  and **must run at the top of a view, before any `c.Theme()` read**.
- `Theme.SetSource(ThemeSystem|ThemeLight|ThemeDark)` decides what `c.Theme().Dark` reports. Call
  it before the first window exists, or the first frame paints in the wrong appearance.
- `Element.AutoFocus()` takes the field's focus when the view builds. Element options are chainable
  and override what the widget set inside.
- `ui.Scroll(c)` draws an **overlay** scrollbar: it does not reserve layout width, so content needs
  its own right inset (`scrollbarGutter` in `style.go`).
- `Element.AnimateColor(key, target, curve)` keeps one animation per key **per element**, and a key
  reused across rows shares state. A page that reused one key for every row painted the previous
  appearance's colour on the first frame after a theme change.
- `ui.Tester` (`ui/headless.go`) drives the pages in tests: `SetDark`, `SetScale`, `SetLocale`,
  `Frame`, `Click`, `Find`, `Texts`, `Focused`. Layout tests render the real pages; screenshots come
  from the same root.

## Tests you must keep green

- `internal/ui/pages_layout_test.go`: every page, seven languages, both appearances, at the minimum
  window size. It asserts text stays inside the content area, and it runs against a fake backend
  with representative rows.
- `internal/ui/pages_shot_test.go`: renders the pages as pictures (light and dark) into the temp
  directory. The README screenshots come from here.
- Registry tests load a **private app hive**. Tests must never touch the real HKCU.

## Conventions

- Seven locales per string. A new key is not done until it exists in all seven files, and `en.json`
  carries the `@` description translators read.
- ASCII hyphens only: no em dash (U+2014), no en dash (U+2013). The Chinese locales use full-width
  punctuation; the English text uses plain ASCII.
- Commit subjects are plain and declarative (`fix(ui): ...`, `refactor(auth): ...`), with a body
  that explains why.
- Do not commit unless asked.
