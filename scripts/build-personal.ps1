# Builds the personal build (mods, Google sign-in, launcher API) with its
# installer into build\bin. It never updates itself, so pull and run this again
# to update. See RELEASING.md, "Personal build".
#
# The Google client comes from .env.personal at the repo root (gitignored):
#   GDRIVE_CLIENT_ID=...
#   GDRIVE_CLIENT_SECRET=...
# or from environment variables of the same names. Without it the build hides
# Sign in with Google.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$client = @{ GDRIVE_CLIENT_ID = $env:GDRIVE_CLIENT_ID; GDRIVE_CLIENT_SECRET = $env:GDRIVE_CLIENT_SECRET }
$file = Join-Path $root '.env.personal'
if (Test-Path $file) {
    foreach ($line in Get-Content $file) {
        if ($line -match '^\s*(GDRIVE_CLIENT_ID|GDRIVE_CLIENT_SECRET)\s*=\s*(.*?)\s*$') {
            $client[$Matches[1]] = $Matches[2].Trim('"', "'")
        }
    }
}
if (-not $client.GDRIVE_CLIENT_ID -or -not $client.GDRIVE_CLIENT_SECRET) {
    Write-Warning 'No Google client (.env.personal or GDRIVE_CLIENT_ID/GDRIVE_CLIENT_SECRET): Sign in with Google will be hidden.'
}

# The last release tag plus this commit, e.g. v1.0.0+personal.3a79b1e. Build
# metadata is ignored when versions are compared, so the app still says when a
# newer release is out: the cue to rebuild.
$tag = git describe --tags --abbrev=0 --match 'v*' 2>$null
if (-not $tag) { $tag = 'v0.0.0' }
$commit = git rev-parse --short HEAD
# Ignores what wails build itself rewrites (installer template, dist, line endings).
git diff --quiet HEAD -- . ':!build/windows/installer' ':!frontend/dist'
if ($LASTEXITCODE -ne 0) { $commit += '.dirty' }
$version = "$tag+personal.$commit"

$ldflags = "-s -w -X main.version=$version"
if ($client.GDRIVE_CLIENT_ID -and $client.GDRIVE_CLIENT_SECRET) {
    $ldflags += " -X main.gdriveClientID=$($client.GDRIVE_CLIENT_ID) -X main.gdriveClientSecret=$($client.GDRIVE_CLIENT_SECRET)"
}

Write-Host "Building Syncer $version (personal)"
wails build -clean -nsis -trimpath -tags personal -ldflags $ldflags
if ($LASTEXITCODE -ne 0) { throw "wails build failed ($LASTEXITCODE)" }
if (-not (Test-Path build\bin\Syncer-amd64-installer.exe)) {
    Write-Warning 'No installer: install NSIS (winget install NSIS.NSIS) and run this again, or use build\bin\Syncer.exe as is.'
}
Get-ChildItem build\bin\Syncer*.exe | ForEach-Object { Write-Host "Built $($_.FullName)" }
