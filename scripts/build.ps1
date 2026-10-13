#!/usr/bin/env pwsh
# Builds biowarden with the MyGo CLI pinned as a tool in go.mod.
#
#   pwsh scripts/build.ps1                                       # this machine (windows/amd64)
#   pwsh scripts/build.ps1 -platform windows/arm64
#   pwsh scripts/build.ps1 -platform windows/amd64,windows/arm64 -debug
#
# Everything after the named parameters goes to `go tool mygo build`.
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

& go tool mygo build @MygoArgs
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

Write-Host ''
Write-Host 'Build output under build/:'
Get-ChildItem -Path 'build' -Directory -ErrorAction SilentlyContinue | ForEach-Object {
    $arch = $_
    Get-ChildItem -Path $arch.FullName -File | ForEach-Object {
        Write-Host ('  {0}/{1}' -f $arch.Name, $_.Name)
    }
}
