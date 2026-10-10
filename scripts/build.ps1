# Run from any directory; all Go/npm caches stay in the repository.
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot
$goExe = Join-Path $projectRoot '.tools\go\bin\go.exe'
if (!(Test-Path -LiteralPath $goExe)) { $goExe = (Get-Command go -ErrorAction Stop).Source }
$env:GOPATH = Join-Path $projectRoot '.cache\gopath'
$env:GOCACHE = Join-Path $projectRoot '.cache\go-build'
$env:GOTMPDIR = Join-Path $projectRoot '.cache\tmp'
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
$env:npm_config_cache = Join-Path $projectRoot '.cache\npm'
$env:GOTOOLCHAIN = 'local'
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR,'bin/downloads' | Out-Null
function Checked-Go { & $goExe @args; if ($LASTEXITCODE -ne 0) { throw "Go command failed: $args" } }
Checked-Go test -tags nomsgpack ./...
Checked-Go vet -tags nomsgpack ./...
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
 $env:CGO_ENABLED = '0'
 $env:GOOS = 'windows'; $env:GOARCH = 'amd64'
 Checked-Go build -tags nomsgpack -trimpath -ldflags='-s -w' -o bin/cloud-server.exe ./server
 Checked-Go build -trimpath -ldflags='-s -w' -o bin/downloads/cloud-agent.exe ./agent
 $env:GOOS = 'linux'
 Checked-Go build -tags nomsgpack -trimpath -ldflags='-s -w' -o bin/cloud-server ./server
 Checked-Go build -trimpath -ldflags='-s -w' -o bin/downloads/cloud-agent-linux-amd64 ./agent
 Copy-Item -LiteralPath bin/downloads/cloud-agent-linux-amd64 -Destination bin/cloud-agent -Force
 $env:GOARCH = 'arm64'
 Checked-Go build -trimpath -ldflags='-s -w' -o bin/downloads/cloud-agent-linux-arm64 ./agent
} finally {
 $env:GOOS = $previousOS; $env:GOARCH = $previousArch; $env:CGO_ENABLED = $previousCGO
}
npm.cmd ci --prefix web --registry=https://registry.npmjs.org
if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
npm.cmd run build --prefix web
if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
Write-Output 'Release programs: bin/ and web/dist/'
