# Building biowarden

`go.mod` pins the MyGo CLI as a [tool](https://go.dev/doc/modules/managing-dependencies#tools)
(v0.7.1, the same version the library is pinned to), so `go tool mygo build` runs
the version the project is developed against. The module requires Go 1.27.3; an
older local toolchain is switched out automatically by `GOTOOLCHAIN=auto`. One command builds the app:

```sh
go tool mygo build
```

On Windows PowerShell the wrapper adds an NSIS check and lists the output:

```powershell
pwsh scripts/build.ps1
pwsh scripts/build.ps1 -platform windows/amd64,windows/arm64
```

The CLI defaults to `CGO_ENABLED=0`, which the rules of the repo require.

## The running host locks the executable

Once the browser extension enables "Unlock with biometrics", Chromium keeps a
native messaging channel open and respawns `biowarden.exe` host processes on
demand. Windows lets a running executable be renamed but not overwritten, so a
plain `go tool mygo build` fails at its final rename of `build/windows-amd64/`
with "The process cannot access the file because it is being used by another
process". Killing the host does not help on its own: the extension reconnects
immediately and a new process takes the lock again.

`scripts/build.ps1` handles this. It builds into `build/.staging`, stops the
running `biowarden` processes (GUI and host), renames the current executable to
`biowarden.exe.old`, copies the staged files into place, then removes the
staging directory and the `.old` file. Stopping the GUI closes any open
biowarden window, and Chromium uses the new build the next time it connects.
On a machine where nothing is running (CI, a fresh checkout) the script skips
the swap and just copies.

Building by hand still works, but either stop the biowarden GUI and quit the
browser (or `Stop-Process -Name biowarden -Force`) first, or pass `-o` to write
somewhere else:

```sh
go tool mygo build -o build/manual
go tool mygo build -platform windows/arm64
```

## Output

`mygo build` writes `build/<os>-<arch>/` (`out` in `mygo.json`). For the
machine's own target, `windows/amd64`:

| File | |
|---|---|
| `biowarden.exe` | the app: tray and native messaging host in one binary |
| `biowarden-setup-0.1.4-amd64.exe` | the per-user installer |
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

## Releasing

A GitHub release carries **the installer and `SHA256SUMS.txt` only**. The
portable `biowarden.exe` stays a local build artifact - it is what the tests
and the on-device acceptance runs use - and is not uploaded.

Upload the installer under the name the build gives it
(`biowarden-setup-0.1.4-amd64.exe`; the name is already a slug, GitHub does not
change it) and write `SHA256SUMS.txt` with one line per published asset, two
spaces between the digest and the file name, using the **uploaded** names:

```
<64 hex digits>  biowarden-setup-0.1.4-amd64.exe
```

Then check the result with `sha256sum -c SHA256SUMS.txt` in a temporary
directory, and delete that directory afterwards.
