# Building biowarden

`go.mod` pins the MyGo CLI as a [tool](https://go.dev/doc/modules/managing-dependencies#tools)
(v0.4.8), so `go tool mygo build` runs the version the project is developed
against. One command builds the app:

```sh
go tool mygo build
```

On Windows PowerShell the wrapper adds an NSIS check and lists the output:

```powershell
pwsh scripts/build.ps1
pwsh scripts/build.ps1 -platform windows/amd64,windows/arm64
```

The CLI defaults to `CGO_ENABLED=0`, which the rules of the repo require.

## Output

`mygo build` writes `build/<os>-<arch>/` (`out` in `mygo.json`). For the
machine's own target, `windows/amd64`:

| File | |
|---|---|
| `biowarden.exe` | the app: tray and native messaging host in one binary |
| `biowarden Setup 0.1.0 amd64.exe` | the per-user installer |
| `icon.png` | the icon copied from `resources/` into the app's files |

`build/` is in `.gitignore`; nothing there is committed.

## The installer needs NSIS

`mygo build` makes the installer with [NSIS](https://nsis.sourceforge.io).
On Windows it downloads the official NSIS 3.13 into `%LOCALAPPDATA%\mygo`
when `makensis` is missing, so a build there still produces one. On macOS
and Linux, install NSIS (`brew install makensis`, `apt install nsis`);
without it the installer step is skipped and only the executable is built.

`scripts/build.ps1` warns when `makensis` is missing and says which of the
two applies.

## Single sources

- `mygo.json` is the only place the name, identifier and version live. The
  build links them into the binary (read back through `mygo.App.Name`,
  `mygo.App.Version` and `mygo.App.IsPackaged`), and the executable and
  installer are named after them.
- `resources/icon.png` is the only icon: `icon` in `mygo.json` points at it,
  and the build derives the Windows icon resource from it.

`resources/icon.png` is generated, not hand-drawn. `go run ./scripts/icongen`
draws the blue ring the tray icon uses, with the standard library alone;
regenerate it instead of editing it.
