# Builds the Windows release: desktop app, CLI and installer.
#
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1 [-Version 0.1.0]
#
# Needs: Go, Node.js (npm), NSIS (makensis). Output: dist\
param([string]$Version = "0.1.0")
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$desktop = Join-Path $root "desktop"
$dist = Join-Path $root "dist"
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
wails3 generate syso -arch amd64 -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest `
  -info build/windows/info.json -out wails_windows_amd64.syso; Check "syso"

Step "desktop app"
$ldflags = "-w -s -H windowsgui"
go build -tags production -trimpath -buildvcs=false -ldflags="$ldflags" -o "$dist\DAWGit.exe" .; Check "desktop build"
Remove-Item wails_windows_amd64.syso
Pop-Location

Step "command line tool"
Push-Location $root
go build -trimpath -buildvcs=false -ldflags="-w -s" -o "$dist\dawgit.exe" ./cmd/dawgit; Check "cli build"
Pop-Location

Step "installer"
Push-Location (Join-Path $desktop "build\windows")
& $makensis /V2 "/DVERSION=$Version" "/DDIST=$dist" installer.nsi; Check "makensis"
Pop-Location

Get-ChildItem $dist | Format-Table Name, @{n = "MB"; e = { [math]::Round($_.Length / 1MB, 1) } }
