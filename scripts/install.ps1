$ErrorActionPreference = 'Stop'

$Repo = if ($env:QUBIT_REPO) { $env:QUBIT_REPO } else { 'zayr0-9/qubit' }
$Version = if ($env:QUBIT_VERSION) { $env:QUBIT_VERSION } else { 'latest' }
$InstallDir = if ($env:QUBIT_INSTALL_DIR) { $env:QUBIT_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Qubit' }
$BinDir = if ($env:QUBIT_BIN_DIR) { $env:QUBIT_BIN_DIR } else { Join-Path $InstallDir 'bin' }
$ArchiveUrl = if ($env:QUBIT_ARCHIVE_URL) { $env:QUBIT_ARCHIVE_URL } else { '' }

if (-not [Environment]::Is64BitOperatingSystem) {
  throw 'Qubit install.ps1 currently supports Windows x64 only.'
}
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
  throw 'Node.js must be installed and available on PATH.'
}

if (-not $ArchiveUrl) {
  if ($Version -eq 'latest') {
    $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $latest.tag_name
  }
  $versionNoV = $Version -replace '^v', ''
  $asset = "qubit-v$versionNoV-windows-x64.zip"
  $ArchiveUrl = "https://github.com/$Repo/releases/download/$Version/$asset"
} else {
  $asset = Split-Path $ArchiveUrl -Leaf
}

$tmp = New-Item -ItemType Directory -Path (Join-Path ([IO.Path]::GetTempPath()) ("qubit-install-" + [guid]::NewGuid()))
try {
  $archive = Join-Path $tmp.FullName $asset
  Write-Host "Downloading $ArchiveUrl"
  Invoke-WebRequest -Uri $ArchiveUrl -OutFile $archive

  $checksumUrl = "$ArchiveUrl.sha256"
  $checksumPath = "$archive.sha256"
  try {
    Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath
    $expected = ((Get-Content $checksumPath -Raw) -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 $archive).Hash.ToLowerInvariant()
    if ($expected -ne $actual) { throw "Checksum mismatch. Expected $expected, got $actual" }
  } catch {
    Write-Warning "Checksum verification skipped or failed to download checksum: $($_.Exception.Message)"
  }

  New-Item -ItemType Directory -Force -Path $InstallDir, $BinDir | Out-Null
  $rootName = [IO.Path]::GetFileNameWithoutExtension($asset) -replace '\.tar$', ''
  $installRoot = Join-Path $InstallDir $rootName
  Remove-Item -Recurse -Force $installRoot -ErrorAction SilentlyContinue
  Expand-Archive -Path $archive -DestinationPath $InstallDir -Force
  $exe = Join-Path $installRoot 'bin\qubit.exe'
  if (-not (Test-Path $exe)) { throw 'Archive did not contain bin\qubit.exe.' }

  $launcher = Join-Path $BinDir 'qubit.cmd'
  Set-Content -Path $launcher -Encoding ASCII -Value "@echo off`r`n`"$exe`" %*`r`n"

  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (($userPath -split ';') -notcontains $BinDir) {
    [Environment]::SetEnvironmentVariable('Path', (($userPath, $BinDir | Where-Object { $_ }) -join ';'), 'User')
    Write-Host "Added $BinDir to your user PATH. Open a new terminal to use qubit."
  }

  Write-Host "Qubit installed to $installRoot"
  Write-Host "Launcher written to $launcher"
  Write-Host 'Try in a new terminal: $env:QUBIT_STUB=1; qubit'
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
