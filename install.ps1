# Install simplex from this checkout, or from the GitHub release when invoked with iex.
#
#   irm https://raw.githubusercontent.com/arcticfoxweb/simplex-herdr/main/install.ps1 | iex
#
# From a checkout:
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
#
# The iex form on Windows downloads the newest release binary, including an
# alpha pre-release, and checks SHA256SUMS. Set SIMPLEX_FROM_SOURCE=1 to compile.
# Throw instead of exit so a failed iex install does not close the session.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
if (Test-Path variable:PSNativeCommandUseErrorActionPreference) {
  $PSNativeCommandUseErrorActionPreference = $false
}
try {
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
} catch {}

$simplexRepo = 'arcticfoxweb/simplex-herdr'
$simplexRepoUrl = $env:SIMPLEX_REPO_URL
if (-not $simplexRepoUrl) {
  $simplexRepoUrl = "https://github.com/$simplexRepo.git"
}
$simplexRef = $env:SIMPLEX_REF
$simplexHome = $env:USERPROFILE
if (-not $simplexHome) { $simplexHome = $HOME }
$simplexSrc = $env:SIMPLEX_SRC_DIR
if (-not $simplexSrc) {
  $simplexSrc = Join-Path $simplexHome '.local\src\simplex'
}
$simplexHeaders = @{
  'User-Agent' = 'simplex-install'
  'Accept'     = 'application/vnd.github+json'
}

function Fail([string]$Message) {
  throw "install: $Message"
}

function Get-WindowsArch {
  if ($env:PROCESSOR_ARCHITEW6432) { return $env:PROCESSOR_ARCHITEW6432 }
  return $env:PROCESSOR_ARCHITECTURE
}

function Get-SimplexRelease {
  $uri = "https://api.github.com/repos/$simplexRepo/releases?per_page=20"
  $releases = @(Invoke-RestMethod -Headers $simplexHeaders -Uri $uri)
  foreach ($rel in $releases) {
    if ($rel.draft) { continue }
    if ($env:SIMPLEX_VERSION -and $rel.tag_name -ne $env:SIMPLEX_VERSION) { continue }
    return $rel
  }
  return $null
}

function Get-ReleaseAsset($Release, [string]$Name) {
  foreach ($asset in @($Release.assets)) {
    if ($asset.name -eq $Name) { return $asset }
  }
  return $null
}

function Install-ReleaseBinary($Release) {
  $arch = Get-WindowsArch
  if ($arch -ne 'AMD64') {
    Fail "this Windows install is amd64 only (this machine is $arch). SimpleX publishes a Windows x86-64 chat binary, and the alpha ships simplex-windows-amd64.exe."
  }
  $exeAsset = Get-ReleaseAsset $Release 'simplex-windows-amd64.exe'
  $sumAsset = Get-ReleaseAsset $Release 'SHA256SUMS'
  if (-not $exeAsset -or -not $sumAsset) {
    return $false
  }
  $tmpExe = Join-Path $env:TEMP 'simplex-windows-amd64.exe'
  $tmpSums = Join-Path $env:TEMP 'simplex-SHA256SUMS'
  Write-Host "downloading $($Release.tag_name) simplex-windows-amd64.exe"
  Invoke-WebRequest -Headers $simplexHeaders -Uri $exeAsset.browser_download_url -OutFile $tmpExe -UseBasicParsing
  Invoke-WebRequest -Headers $simplexHeaders -Uri $sumAsset.browser_download_url -OutFile $tmpSums -UseBasicParsing
  $got = (Get-FileHash -Algorithm SHA256 -LiteralPath $tmpExe).Hash.ToLower()
  $ok = $false
  foreach ($line in @(Get-Content -LiteralPath $tmpSums)) {
    $parts = @($line -split '\s+' | Where-Object { $_ -ne '' })
    if ($parts.Count -lt 2) { continue }
    $name = $parts[$parts.Count - 1].TrimStart('*')
    if ($parts[0].ToLower() -eq $got -and $name -eq 'simplex-windows-amd64.exe') {
      $ok = $true
    }
  }
  if (-not $ok) {
    Fail 'checksum mismatch for simplex-windows-amd64.exe'
  }
  $destDir = Join-Path $simplexHome '.local\bin'
  New-Item -ItemType Directory -Force -Path $destDir | Out-Null
  $dest = Join-Path $destDir 'simplex.exe'
  # Move-Item -Force does not replace an existing file on Windows PowerShell.
  if (Test-Path -LiteralPath $dest) {
    try {
      Remove-Item -LiteralPath $dest -Force
    } catch {
      Fail "could not replace $dest. Close any running simplex.exe and run the installer again."
    }
  }
  Move-Item -LiteralPath $tmpExe -Destination $dest
  Write-Host "installed $dest from $($Release.tag_name)"
  return $true
}

function Save-ReleaseSource($Release) {
  $plugin = Join-Path $simplexSrc 'herdr-plugin\herdr-plugin.toml'
  if (Test-Path -LiteralPath $plugin) {
    return $simplexSrc
  }
  $marker = Join-Path $simplexSrc '.simplex-release'
  if (Test-Path -LiteralPath $simplexSrc) {
    if (Test-Path -LiteralPath $marker) {
      Remove-Item -LiteralPath $simplexSrc -Recurse -Force
    } else {
      Fail "$simplexSrc exists and is not a previous simplex release download"
    }
  }
  $zip = Join-Path $env:TEMP 'simplex-herdr-src.zip'
  $unpack = Join-Path $env:TEMP 'simplex-herdr-src'
  Write-Host "downloading source for $($Release.tag_name)"
  Invoke-WebRequest -Headers $simplexHeaders -Uri $Release.zipball_url -OutFile $zip -UseBasicParsing
  if (Test-Path -LiteralPath $unpack) {
    Remove-Item -LiteralPath $unpack -Recurse -Force
  }
  Expand-Archive -LiteralPath $zip -DestinationPath $unpack
  $top = @(Get-ChildItem -LiteralPath $unpack -Directory)
  if ($top.Count -lt 1) {
    Fail 'release source archive was empty'
  }
  $manifest = Join-Path $top[0].FullName 'herdr-plugin\herdr-plugin.toml'
  if (-not (Test-Path -LiteralPath $manifest)) {
    Fail 'release source has no herdr plugin'
  }
  $parent = Split-Path -Parent $simplexSrc
  New-Item -ItemType Directory -Force -Path $parent | Out-Null
  Move-Item -LiteralPath $top[0].FullName -Destination $simplexSrc
  Set-Content -LiteralPath $marker -Value $Release.tag_name
  return $simplexSrc
}

function Find-HerdrExe {
  $cmd = Get-Command herdr -ErrorAction SilentlyContinue
  if ($cmd -and $cmd.Source) { return $cmd.Source }
  $candidates = @(
    (Join-Path $env:LOCALAPPDATA 'Programs\Herdr\bin\herdr.exe'),
    (Join-Path $simplexHome '.herdr\packages\standalone\current\herdr.exe')
  )
  foreach ($candidate in $candidates) {
    if ($candidate -and (Test-Path -LiteralPath $candidate)) { return $candidate }
  }
  return $null
}

function Install-SimplexShim([string]$Exe) {
  if ($env:OS -ne 'Windows_NT') { return }
  $dirs = @()
  $herdr = Find-HerdrExe
  if ($herdr) { $dirs += (Split-Path -Parent $herdr) }
  $dirs += (Join-Path $env:LOCALAPPDATA 'Programs\Herdr\bin')
  $dirs += (Join-Path $simplexHome '.herdr\packages\standalone\current')
  $seen = @{}
  $body = "@echo off`r`n`"$Exe`" %*`r`n"
  foreach ($dir in $dirs) {
    if (-not $dir) { continue }
    $key = $dir.TrimEnd('\').ToLower()
    if ($seen.ContainsKey($key)) { continue }
    $seen[$key] = $true
    if (-not (Test-Path -LiteralPath $dir)) { continue }
    $dest = Join-Path $dir 'simplex.cmd'
    try {
      Set-Content -LiteralPath $dest -Value $body -Encoding Ascii
      Write-Host "installed $dest"
    } catch {
      Write-Host "could not write $dest"
    }
  }
}

function Ensure-SimplexChat([string]$Exe) {
  if ($env:SIMPLEX_CHAT_BIN -and (Test-Path -LiteralPath $env:SIMPLEX_CHAT_BIN)) {
    Write-Host "using simplex-chat at $env:SIMPLEX_CHAT_BIN"
    return
  }
  $share = Join-Path $simplexHome '.local\share\simplex\bin\simplex-chat.exe'
  if ((Test-Path -LiteralPath $share) -or (Get-Command simplex-chat -ErrorAction SilentlyContinue)) {
    Write-Host 'simplex-chat already installed; skipping download'
    return
  }
  & $Exe install
  if ($LASTEXITCODE -ne 0) {
    Fail "simplex install failed ($LASTEXITCODE)"
  }
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
$usedRelease = $false
$fromCheckout = $false
if ($simplexFrom -and (Test-Path -LiteralPath (Join-Path $simplexFrom 'go.mod')) -and (Test-Path -LiteralPath (Join-Path $simplexFrom 'herdr-plugin\build.ps1'))) {
  $fromCheckout = $true
  $simplexRoot = $simplexFrom
}

if (-not $fromCheckout -and $env:SIMPLEX_FROM_SOURCE -ne '1' -and $env:OS -eq 'Windows_NT') {
  try {
    $release = Get-SimplexRelease
  } catch {
    $release = $null
    Write-Host "release lookup failed; will build from source"
  }
  if ($release -and (Install-ReleaseBinary $release)) {
    $simplexRoot = Save-ReleaseSource $release
    Ensure-SimplexChat (Join-Path $simplexHome '.local\bin\simplex.exe')
    $usedRelease = $true
  } elseif (-not $release) {
    Write-Host 'no GitHub release found; will build from source'
  }
}

if (-not $usedRelease) {
  if (-not $simplexRoot) {
    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
      Fail 'need git on PATH to build from source'
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
  } else {
    Write-Host "building $simplexRoot"
  }
  $build = Join-Path $simplexRoot 'herdr-plugin\build.ps1'
  if (-not (Test-Path -LiteralPath $build)) {
    Fail "no herdr-plugin\build.ps1 in $simplexRoot"
  }
  & $build
}

$installedExe = Join-Path (Join-Path $simplexHome '.local\bin') 'simplex.exe'
if ($env:OS -ne 'Windows_NT') {
  $installedExe = Join-Path (Join-Path $simplexHome '.local\bin') 'simplex'
}
Install-SimplexShim $installedExe

if ($env:SIMPLEX_SKIP_LINK -eq '1') {
  Write-Host 'skipping Herdr plugin link'
} else {
  $herdrExe = Find-HerdrExe
  if ($herdrExe) {
    $plugin = Join-Path $simplexRoot 'herdr-plugin'
    & $herdrExe plugin link $plugin
    if ($LASTEXITCODE -ne 0) { Fail "herdr plugin link failed ($LASTEXITCODE)" }
    Write-Host "linked Herdr plugin simplex.agents from $plugin"
  } else {
    Write-Host 'Herdr is not installed yet, so the plugin was not linked.'
    Write-Host 'Install Herdr, then run this installer again. It puts simplex.cmd next to herdr.exe.'
  }
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
Remove-Item -ErrorAction SilentlyContinue Function:Get-WindowsArch
Remove-Item -ErrorAction SilentlyContinue Function:Get-SimplexRelease
Remove-Item -ErrorAction SilentlyContinue Function:Get-ReleaseAsset
Remove-Item -ErrorAction SilentlyContinue Function:Install-ReleaseBinary
Remove-Item -ErrorAction SilentlyContinue Function:Save-ReleaseSource
Remove-Item -ErrorAction SilentlyContinue Function:Ensure-SimplexChat
Remove-Item -ErrorAction SilentlyContinue Function:Find-HerdrExe
Remove-Item -ErrorAction SilentlyContinue Function:Install-SimplexShim
