# Build, commit, push and publish a GitHub release with the exe.
param(
  [string]$Version = "",
  [string]$Message = "update"
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$env:Path += ";$env:USERPROFILE\go\bin"
$env:GOPROXY = "https://proxy.golang.org,direct"

# Determine version
if (-not $Version) {
  $tag = (git tag --list "v*" | Sort-Object { [version]($_ -replace 'v','') } | Select-Object -Last 1)
  if ($tag) {
    $v = [version]($tag -replace 'v','')
    $Version = "v$($v.Major).$($v.Minor).$($v.Build + 1)"
  } else {
    $Version = "v1.0.0"
  }
}
Write-Host "Version: $Version"

# Build frontend + wails
Write-Host "Building frontend..."
Push-Location "$root\hermes_wails\frontend"
npm run build
Pop-Location

Write-Host "Building wails app..."
Push-Location "$root\hermes_wails"
wails build -platform windows/amd64 -f
Pop-Location

$exe = "$root\hermes_wails\build\bin\hermes.exe"
if (-not (Test-Path $exe)) { Write-Error "exe not found"; exit 1 }

# Copy to releases dir
$relDir = "$root\releases"
New-Item -ItemType Directory -Force -Path $relDir | Out-Null
$relExe = "$relDir\hermes-$Version.exe"
Copy-Item $exe $relExe -Force
Write-Host "Release exe: $relExe"

# Git commit + push
git -C $root -c core.autocrlf=false add -A 2>$null
git -C $root -c core.autocrlf=false commit -m "$Message ($Version)"
git -C $root tag $Version
git -C $root push origin main --tags

# GitHub release
gh -C $root release create $Version --title "Hermes Portable $Version" --notes "$Message" "$relExe"
Write-Host "Done: $Version"
