#!/usr/bin/env pwsh
# Builds biowarden with the MyGo CLI pinned as a tool in go.mod.
#
#   pwsh scripts/build.ps1                                       # this machine (windows/amd64)
#   pwsh scripts/build.ps1 -platform windows/arm64
#   pwsh scripts/build.ps1 -platform windows/amd64,windows/arm64 -debug
#
# Everything after the named parameters goes to `go tool mygo build`.
#
# The build also produces the portable `biowarden.exe`. Keep it: the tests and
# the on-device acceptance runs use it. It is not a release asset though -
# a release carries the installer and SHA256SUMS.txt only (see docs/build.md).
[CmdletBinding()]
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$MygoArgs
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'go is not on PATH; install Go 1.27 or later.'
}

# The installer needs NSIS. Windows builds download NSIS 3.13 into
# %LOCALAPPDATA%\mygo when makensis is missing; on macOS and Linux the
# installer step is skipped instead.
if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) {
    $onWindows = $PSVersionTable.PSVersion.Major -lt 6 -or $IsWindows
    Write-Warning 'NSIS (makensis) is not on PATH.'
    if ($onWindows) {
        Write-Host '  mygo build downloads NSIS 3.13 into %LOCALAPPDATA%\mygo on first use, so the installer is still built.'
    } else {
        Write-Host '  Install it for an installer (brew install makensis, apt install nsis); mygo build skips the installer otherwise.'
    }
}

$staging = 'build/.staging'
$output = 'build'

# Build into a staging directory first: Chromium keeps respawning the native
# messaging host, which holds the running biowarden.exe open, so writing straight
# into build/ fails at mygo's final rename of the directory.
& go tool mygo build -o $staging @MygoArgs
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

# A running executable cannot be overwritten, but it can be renamed. Stop the
# running copies (GUI and host), move the current exe aside, then copy the fresh
# files in. Chromium reconnects with the new build.
$exes = @(Get-ChildItem -Path $output -Recurse -Filter 'biowarden.exe' -File -ErrorAction SilentlyContinue)
if ($exes.Count -gt 0) {
    $running = @(Get-Process -Name 'biowarden' -ErrorAction SilentlyContinue)
    if ($running.Count -gt 0) {
        $running | Stop-Process -Force -ErrorAction SilentlyContinue
        Write-Host 'Stopped the running biowarden (GUI/host); Chromium will use the new build on its next connection.'
    }
}

$staged = @(Get-ChildItem -Path $staging -Directory -ErrorAction SilentlyContinue)
foreach ($platform in $staged) {
    $dest = Join-Path $output $platform.Name
    New-Item -ItemType Directory -Path $dest -Force | Out-Null

    $exe = Join-Path $dest 'biowarden.exe'
    if (Test-Path $exe) {
        $old = Join-Path $dest 'biowarden.exe.old'
        Remove-Item -Path $old -Force -ErrorAction SilentlyContinue
        try {
            Rename-Item -Path $exe -NewName 'biowarden.exe.old' -ErrorAction Stop
        } catch {
            # Rename can fail when nothing holds the file open; the copy below
            # overwrites it in place in that case.
        }
    }

    Copy-Item -Path (Join-Path $platform.FullName '*') -Destination $dest -Recurse -Force
}

Remove-Item -Path $staging -Recurse -Force -ErrorAction SilentlyContinue

# A process that respawned after the swap can keep the .old file open; leaving it
# behind is harmless, so only say so instead of failing the build.
Get-ChildItem -Path $output -Recurse -Filter '*.old' -File -ErrorAction SilentlyContinue | ForEach-Object {
    Remove-Item -Path $_.FullName -Force -ErrorAction SilentlyContinue
    if (Test-Path $_.FullName) {
        Write-Warning ("Could not remove {0} - a biowarden process still holds it; the next build will clean it up." -f $_.FullName)
    }
}

Write-Host ''
Write-Host 'Build output under build/:'
Get-ChildItem -Path 'build' -Directory -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -notlike '.*' } | ForEach-Object {
    $arch = $_
    Get-ChildItem -Path $arch.FullName -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -notlike '*.old' } | ForEach-Object {
        Write-Host ('  {0}/{1}' -f $arch.Name, $_.Name)
    }
}
