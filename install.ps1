# install.ps1 - install carrier from GitHub Releases
#
# Usage:
#   irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1 | iex
#
# Specific version:
#   powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1))) -Version v0.1.0"

param(
    [string]$Version = "",
    [switch]$System,
    [switch]$Help
)

$ErrorActionPreference = 'Stop'

if ($Help) {
    Write-Host "Usage: install.ps1 [OPTIONS]"
    Write-Host ""
    Write-Host "Install carrier from GitHub Releases."
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Version <tag>   Install a specific release (e.g. v0.1.0). Default: latest"
    Write-Host "  -System          Install to Program Files instead of LocalAppData"
    Write-Host "  -Help            Show this help message"
    exit 0
}

$Repo = "atbuy/carrier"

$InstallDir = if ($System) {
    Join-Path $env:ProgramFiles "carrier"
} else {
    Join-Path $env:LOCALAPPDATA "carrier\bin"
}

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'ARM64' { 'arm64' }
    'AMD64' { 'amd64' }
    'x86' {
        if ($env:PROCESSOR_ARCHITEW6432 -eq 'AMD64') { 'amd64' }
        else {
            Write-Error "32-bit systems are not supported."
            exit 1
        }
    }
    default {
        Write-Error "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
        exit 1
    }
}

if (-not $Version) {
    Write-Host "Fetching latest release version..."
    try {
        $Response = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
        $Version = $Response.tag_name
    } catch {
        Write-Error "Failed to fetch latest release version from GitHub API: $_"
        exit 1
    }
    if (-not $Version) {
        Write-Error "Failed to parse release version from GitHub API response."
        exit 1
    }
}

Write-Host "Installing carrier $Version (windows/$Arch)..."
Write-Host ""

$Archive = "carrier-windows-$Arch.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$Archive"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$TmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $TmpDir | Out-Null

try {
    Write-Host "Downloading $Archive..."
    $ArchivePath = Join-Path $TmpDir $Archive
    try {
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $ArchivePath -UseBasicParsing
    } catch {
        Write-Error "Download failed. Check that $Version exists at: https://github.com/$Repo/releases"
        exit 1
    }

    Write-Host "Extracting..."
    Expand-Archive -Path $ArchivePath -DestinationPath $TmpDir -Force

    $CarrierBin = Join-Path $TmpDir "carrier-windows-$Arch.exe"
    if (-not (Test-Path $CarrierBin)) {
        Write-Error "Expected binary not found in archive: carrier-windows-$Arch.exe"
        exit 1
    }
    Copy-Item $CarrierBin (Join-Path $InstallDir "carrier.exe") -Force
} finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "Installed to $InstallDir"

$CurrentPath = [System.Environment]::GetEnvironmentVariable('PATH', 'User')
if ($null -eq $CurrentPath) { $CurrentPath = "" }

if ($CurrentPath -notlike "*$InstallDir*") {
    $NewPath = if ($CurrentPath) { "$CurrentPath;$InstallDir" } else { $InstallDir }
    [System.Environment]::SetEnvironmentVariable('PATH', $NewPath, 'User')
    Write-Host ""
    Write-Host "Added $InstallDir to your user PATH."
    Write-Host "Restart your terminal for the PATH change to take effect."
}

Write-Host ""
& (Join-Path $InstallDir "carrier.exe") version
