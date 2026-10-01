# Builds the Windows release: desktop app, CLI and installer.
#
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1
#
# The version comes from internal/version/version.go.
# Needs: Go, Node.js (npm), NSIS (makensis). Output: dist\
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$src = Get-Content (Join-Path $root "internal\version\version.go") -Raw
if ($src -notmatch 'Version = "(\d+\.\d+\.\d+)"') { throw "no version in internal/version/version.go" }
$Version = $Matches[1]
Write-Host "DAWGit $Version"
$desktop = Join-Path $root "desktop"
$dist = Join-Path $root "dist"
Remove-Item -Recurse -Force $dist -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $dist | Out-Null

function Step($name) { Write-Host "`n== $name" -ForegroundColor Cyan }
function Check($what) { if ($LASTEXITCODE -ne 0) { throw "$what failed" } }

$makensis = (Get-Command makensis -ErrorAction SilentlyContinue).Source
if (-not $makensis) {
  foreach ($p in "${env:ProgramFiles(x86)}\NSIS\makensis.exe", "$env:ProgramFiles\NSIS\makensis.exe") {
    if (Test-Path $p) { $makensis = $p }
  }
}
if (-not $makensis) { throw "NSIS not found (winget install NSIS.NSIS)" }

Step "frontend"
Push-Location (Join-Path $desktop "frontend")
if (-not (Test-Path node_modules)) { npm install; Check "npm install" }
npm run build; Check "frontend build"
Pop-Location

Step "Windows resources (icon, manifest, version info)"
Push-Location $desktop
# info.json with this version filled in (file properties of DAWGit.exe).
$info = Get-Content build/windows/info.json -Raw | ConvertFrom-Json
$info.fixed.file_version = "$Version.0"
$info.fixed.product_version = "$Version.0"
$info.info."0409".ProductVersion = $Version
$info.info."0409".FileVersion = $Version
$infoFile = Join-Path $env:TEMP "dawgit-info.json"
[IO.File]::WriteAllText($infoFile, ($info | ConvertTo-Json -Depth 5), (New-Object Text.UTF8Encoding $false))
wails3 generate syso -arch amd64 -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest `
  -info $infoFile -out cmd/dawgit-desktop/wails_windows_amd64.syso; Check "syso"
Remove-Item $infoFile

Step "desktop app"
$ldflags = "-w -s -H windowsgui"
go build -tags production -trimpath -buildvcs=false -ldflags="$ldflags" -o "$dist\DAWGit.exe" ./cmd/dawgit-desktop; Check "desktop build"
Remove-Item cmd/dawgit-desktop/wails_windows_amd64.syso
Pop-Location

Step "command line tool"
Push-Location $root
# In bin\: Windows file names ignore case, so dawgit.exe and DAWGit.exe
# cannot share a folder.
go build -trimpath -buildvcs=false -ldflags="-w -s" -o "$dist\bin\dawgit.exe" ./cmd/dawgit; Check "cli build"
Pop-Location

Step "installer"
Push-Location (Join-Path $desktop "build\windows")
& $makensis /V2 "/DVERSION=$Version" "/DDIST=$dist" installer.nsi; Check "makensis"
Pop-Location

Get-ChildItem $dist -Recurse -File | Format-Table Name, @{n = "MB"; e = { [math]::Round($_.Length / 1MB, 1) } }
