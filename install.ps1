# Install simplex from this checkout, or clone it when PowerShell invokes this with iex.
#
#   irm https://raw.githubusercontent.com/arcticfoxweb/simplex-herdr/main/install.ps1 | iex
#
# From a checkout:
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
#
# The iex form uses SIMPLEX_REPO_URL (default below). That repository has to
# exist before the one-liner works. From a checkout, this script never clones.
# Throw instead of exit so a failed iex install does not close the session.
$ErrorActionPreference = 'Stop'
if (Test-Path variable:PSNativeCommandUseErrorActionPreference) {
  $PSNativeCommandUseErrorActionPreference = $false
}

$simplexRepoUrl = $env:SIMPLEX_REPO_URL
if (-not $simplexRepoUrl) {
  $simplexRepoUrl = 'https://github.com/arcticfoxweb/simplex-herdr.git'
}
$simplexRef = $env:SIMPLEX_REF
$simplexHome = $env:USERPROFILE
if (-not $simplexHome) { $simplexHome = $HOME }
$simplexSrc = $env:SIMPLEX_SRC_DIR
if (-not $simplexSrc) {
  $simplexSrc = Join-Path $simplexHome '.local\src\simplex'
}

function Fail([string]$Message) {
  throw "install: $Message"
}

# Directory of this file when PowerShell ran it as a script. Empty under iex.
$simplexFrom = $PSScriptRoot
if (-not $simplexFrom -and $PSCommandPath) {
  $simplexFrom = Split-Path -Parent $PSCommandPath
}
if (-not $simplexFrom -and $MyInvocation.MyCommand.Path) {
  $simplexFrom = Split-Path -Parent $MyInvocation.MyCommand.Path
}

$simplexRoot = $null
if ($simplexFrom -and (Test-Path -LiteralPath (Join-Path $simplexFrom 'go.mod')) -and (Test-Path -LiteralPath (Join-Path $simplexFrom 'herdr-plugin\build.ps1'))) {
  $simplexRoot = $simplexFrom
  Write-Host "building $simplexRoot"
} else {
  if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Fail 'need git on PATH'
  }
  $parent = Split-Path -Parent $simplexSrc
  if (-not (Test-Path -LiteralPath $parent)) {
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
  }
  if (Test-Path -LiteralPath (Join-Path $simplexSrc '.git')) {
    Write-Host "updating $simplexSrc"
    & git -C $simplexSrc remote set-url origin $simplexRepoUrl
    if ($LASTEXITCODE -ne 0) { Fail "git remote set-url failed ($LASTEXITCODE)" }
    if ($simplexRef) {
      & git -C $simplexSrc fetch --depth 1 origin $simplexRef
      if ($LASTEXITCODE -ne 0) { Fail "git fetch failed ($LASTEXITCODE)" }
      & git -C $simplexSrc checkout --detach FETCH_HEAD
      if ($LASTEXITCODE -ne 0) { Fail "git checkout failed ($LASTEXITCODE)" }
    } else {
      & git -C $simplexSrc pull --ff-only
      if ($LASTEXITCODE -ne 0) { Fail "git pull failed ($LASTEXITCODE)" }
    }
  } elseif (Test-Path -LiteralPath $simplexSrc) {
    Fail "$simplexSrc exists and is not a git checkout"
  } else {
    Write-Host "cloning $simplexRepoUrl"
    if ($simplexRef) {
      & git clone --depth 1 --branch $simplexRef $simplexRepoUrl $simplexSrc
    } else {
      & git clone --depth 1 $simplexRepoUrl $simplexSrc
    }
    if ($LASTEXITCODE -ne 0) { Fail "git clone failed ($LASTEXITCODE)" }
  }
  $simplexRoot = $simplexSrc
}

$build = Join-Path $simplexRoot 'herdr-plugin\build.ps1'
if (-not (Test-Path -LiteralPath $build)) {
  Fail "no herdr-plugin\build.ps1 in $simplexRoot"
}
& $build

if ($env:SIMPLEX_SKIP_LINK -eq '1') {
  Write-Host 'skipping Herdr plugin link'
} elseif (Get-Command herdr -ErrorAction SilentlyContinue) {
  $plugin = Join-Path $simplexRoot 'herdr-plugin'
  & herdr plugin link $plugin
  if ($LASTEXITCODE -ne 0) { Fail "herdr plugin link failed ($LASTEXITCODE)" }
  Write-Host "linked Herdr plugin simplex.agents from $plugin"
} else {
  Write-Host 'herdr is not on PATH; the plugin was not linked'
}

$bindir = Join-Path $simplexHome '.local\bin'
if ($env:OS -eq 'Windows_NT') {
  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  $already = $false
  if ($userPath) {
    foreach ($entry in ($userPath -split ';')) {
      if ($entry.TrimEnd('\') -ieq $bindir.TrimEnd('\')) {
        $already = $true
        break
      }
    }
  }
  if (-not $already) {
    if ([string]::IsNullOrEmpty($userPath)) {
      $updated = $bindir
    } else {
      $updated = "$userPath;$bindir"
    }
    [Environment]::SetEnvironmentVariable('Path', $updated, 'User')
    Write-Host "added $bindir to the user Path"
  }
}
$sep = [IO.Path]::PathSeparator
$marker = "$sep$bindir$sep"
$haystack = "$sep$env:Path$sep"
if ($haystack.ToLower().IndexOf($marker.ToLower()) -lt 0) {
  $env:Path = "$bindir$sep$env:Path"
}

$installedName = 'simplex'
if ($env:OS -eq 'Windows_NT') { $installedName = 'simplex.exe' }
Write-Host ""
Write-Host "Installed $(Join-Path $bindir $installedName)"
Write-Host ""
Write-Host "From the Herdr pane where the agent is already running:"
Write-Host ""
Write-Host "  simplex init"
Write-Host "  herdr plugin action invoke simplex.agents.attach"
Write-Host "  simplex qr"
Write-Host ""
Write-Host "Send through the plugin:"
Write-Host ""
Write-Host "  herdr plugin pane open --plugin simplex.agents --entrypoint send --env SIMPLEX_TO=NAME --env SIMPLEX_TEXT=hello"
Write-Host ""
Write-Host "Open a new terminal if simplex is not found yet. Restart Herdr after the first install so the plugin startup hook runs."
Remove-Item -ErrorAction SilentlyContinue Function:Fail
