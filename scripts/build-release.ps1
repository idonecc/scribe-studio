# SPDX-License-Identifier: GPL-3.0-or-later
#
# Windows release build. Mirrors build-release.sh:
#   1. Inject ldflags so backend/scribe/version.go gets the proper
#      version / commit / date / core-rev / build mode.
#   2. wails build -platform windows/amd64
#   3. Copy resources/bin/windows-amd64/* next to the exe so the runtime
#      BinaryPath resolver finds bundled tools without falling through
#      to PATH (end users won't have ffmpeg / whisper-cli installed).
#
# Usage:
#   .\scripts\build-release.ps1                          # uses git describe
#   .\scripts\build-release.ps1 -Version v0.4.0          # explicit tag

[CmdletBinding()]
param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

if (-not $Version) {
    $Version = (& git describe --tags --always --dirty 2>$null)
    if (-not $Version) { $Version = "dev" }
}
$commit = (& git rev-parse --short HEAD 2>$null)
if (-not $commit) { $commit = "unknown" }
$dateUtc = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$coreRev = (& git -C (Join-Path $repoRoot "backend/core") rev-parse --short HEAD 2>$null)
if (-not $coreRev) { $coreRev = "unknown" }

$pkg = "github.com/autogame-17/scribe-studio/backend/scribe"

# Single-quoted -X args mirror the bash script; PowerShell needs us to
# build the string ourselves because -ldflags "{0}" gets reparsed.
$ldflags = @(
    "-X '$pkg.BuildVersion=$Version'",
    "-X '$pkg.BuildCommit=$commit'",
    "-X '$pkg.BuildDate=$dateUtc'",
    "-X '$pkg.BuildCoreRev=$coreRev'",
    "-X '$pkg.BuildMode=release'"
) -join " "

Write-Host "==> Building Scribe $Version"
Write-Host "    commit=$commit date=$dateUtc core=$coreRev"

& wails build -platform windows/amd64 -ldflags $ldflags
if ($LASTEXITCODE -ne 0) { throw "wails build failed" }

$exe = Join-Path $repoRoot "build/bin/scribe-studio.exe"
if (-not (Test-Path $exe)) {
    Get-ChildItem (Join-Path $repoRoot "build/bin")
    throw "expected $exe to exist after build"
}
Write-Host "==> Built $exe"

# Bundle the static binaries next to the exe. Layout has to mirror
# resources/bin/<arch>/<tool> so runtime/binpath.go's lookup
# (exeDir/resources/bin/<arch>/<name>) finds them.
$srcArchDir = Join-Path $repoRoot "resources/bin/windows-amd64"
if (Test-Path $srcArchDir) {
    $dstArchDir = Join-Path $repoRoot "build/bin/resources/bin/windows-amd64"
    New-Item -ItemType Directory -Force -Path $dstArchDir | Out-Null
    Copy-Item -Recurse -Force (Join-Path $srcArchDir "*") $dstArchDir
    Write-Host "==> bundled binaries from $srcArchDir to $dstArchDir"
} else {
    Write-Warning "resources/bin/windows-amd64 not found; run fetch-bins.ps1 -Release first"
}
