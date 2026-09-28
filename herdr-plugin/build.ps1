# Build the simplex CLI and download simplex-chat when it is not already present.
# Herdr runs this during plugin install on Windows. install.ps1 calls it too.
# Throw instead of exit: exit would close a session that invoked install.ps1 with iex.
$ErrorActionPreference = 'Stop'
if (Test-Path variable:PSNativeCommandUseErrorActionPreference) {
  $PSNativeCommandUseErrorActionPreference = $false
}

$here = $PSScriptRoot
if (-not $here -and $PSCommandPath) {
  $here = Split-Path -Parent $PSCommandPath
}
if (-not $here -and $MyInvocation.MyCommand.Path) {
  $here = Split-Path -Parent $MyInvocation.MyCommand.Path
}
if (-not $here) {
  throw 'simplex build: run build.ps1 as a file'
}

$root = $here
while (-not (Test-Path -LiteralPath (Join-Path $root 'go.mod'))) {
  $parent = Split-Path -Parent $root
  if (-not $parent -or $parent -eq $root) {
    throw "simplex build: no go.mod above $here"
  }
  $root = $parent
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  throw 'simplex build: need Go on PATH (1.22 or newer; the module uses 1.26)'
}

$simplexHome = $env:USERPROFILE
if (-not $simplexHome) { $simplexHome = $HOME }

$bindir = Join-Path $simplexHome '.local\bin'
New-Item -ItemType Directory -Force -Path $bindir | Out-Null
$outName = 'simplex'
if ($env:OS -eq 'Windows_NT') { $outName = 'simplex.exe' }
$out = Join-Path $bindir $outName

Push-Location $root
try {
  & go build -o $out ./cmd/simplex
  if ($LASTEXITCODE -ne 0) {
    throw "simplex build: go build failed ($LASTEXITCODE)"
  }
} finally {
  Pop-Location
}
Write-Host "built $out"

if ($env:SIMPLEX_CHAT_BIN -and (Test-Path -LiteralPath $env:SIMPLEX_CHAT_BIN)) {
  Write-Host "using simplex-chat at $env:SIMPLEX_CHAT_BIN"
  return
}

$share = Join-Path $simplexHome '.local\share\simplex\bin\simplex-chat'
if ($env:OS -eq 'Windows_NT') { $share = "$share.exe" }
$onPath = Get-Command simplex-chat -ErrorAction SilentlyContinue
if ((Test-Path -LiteralPath $share) -or $onPath) {
  Write-Host 'simplex-chat already installed; skipping download'
  return
}

& $out install
if ($LASTEXITCODE -ne 0) {
  throw "simplex build: simplex install failed ($LASTEXITCODE)"
}
