#Requires -Version 5.1
<#
.SYNOPSIS
    nerv installer for Windows: downloads the right release, installs the
    nerv.exe binary, and runs `nerv install`.

.DESCRIPTION
    Usage:
      powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.ps1 | iex"

    Every option is an environment variable (this script takes no
    arguments of its own, since it is piped into iex):

      NERV_CHANNEL        stable (default), alpha or rc.
      NERV_VERSION         Exact tag to install (e.g. "v1.2.3"). Overrides
                            NERV_CHANNEL entirely; no API call is made.
      NERV_INSTALL_DIR     Where to put the binary. Default:
                            $env:LOCALAPPDATA\nerv\bin.
      NERV_NO_INSTALL      When set to any non-empty value, skip running
                            `nerv install` after placing the binary.

    Test seams (tests/install-ps1.test.ps1 only, never needed by end
    users):
      NERV_API_BASE        GitHub API base used to resolve releases
                            (default: https://api.github.com/repos/war-apps/nerv-by-gentle-ai).
      NERV_DOWNLOAD_BASE   Release download base (default:
                            https://github.com/war-apps/nerv-by-gentle-ai/releases/download).
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$Repo = "war-apps/nerv-by-gentle-ai"
$Binary = "nerv"

function Write-Log($Message) {
    Write-Host "nerv install.ps1: $Message"
}

function Get-ApiBase {
    if ($env:NERV_API_BASE) { return $env:NERV_API_BASE }
    return "https://api.github.com/repos/$Repo"
}

function Get-DownloadBase {
    if ($env:NERV_DOWNLOAD_BASE) { return $env:NERV_DOWNLOAD_BASE }
    return "https://github.com/$Repo/releases/download"
}

function Get-Arch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($env:PROCESSOR_ARCHITEW6432) {
        $arch = $env:PROCESSOR_ARCHITEW6432
    }
    switch -Regex ($arch) {
        "ARM64" { return "arm64" }
        "AMD64|x86_64" { return "amd64" }
        default { throw "unsupported architecture: $arch (see https://github.com/$Repo#install)" }
    }
}

function Resolve-Version {
    if ($env:NERV_VERSION) {
        return $env:NERV_VERSION
    }
    $channel = $env:NERV_CHANNEL
    if (-not $channel) { $channel = "stable" }
    $apiBase = Get-ApiBase

    switch ($channel) {
        "stable" {
            $release = Invoke-RestMethod -UseBasicParsing -Uri "$apiBase/releases/latest"
            if (-not $release.tag_name) {
                throw "could not resolve the latest stable release from $apiBase/releases/latest"
            }
            return $release.tag_name
        }
        { $_ -in "alpha", "rc" } {
            $releases = Invoke-RestMethod -UseBasicParsing -Uri "$apiBase/releases?per_page=50"
            $marker = "-$channel."
            $best = $releases | Where-Object {
                -not $_.draft -and $_.prerelease -and $_.tag_name -like "*$marker*"
            } | Select-Object -First 1
            if (-not $best) {
                throw "no $channel pre-release published yet"
            }
            return $best.tag_name
        }
        default {
            throw "unknown channel: $channel (expected stable, alpha or rc)"
        }
    }
}

function Install-Nerv {
    $goarch = Get-Arch
    $tag = Resolve-Version
    $version = $tag.TrimStart("v")
    $archiveName = "${Binary}_${version}_windows_${goarch}.zip"
    $baseUrl = "$(Get-DownloadBase)/$tag"

    $workDir = Join-Path $env:TEMP ("nerv-install-" + [System.Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $workDir -Force | Out-Null
    try {
        $archivePath = Join-Path $workDir $archiveName
        $checksumsPath = Join-Path $workDir "checksums.txt"

        Write-Log "Downloading $archiveName ($tag)..."
        Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$archiveName" -OutFile $archivePath
        Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/checksums.txt" -OutFile $checksumsPath

        Test-Checksum -ArchivePath $archivePath -ChecksumsPath $checksumsPath

        Expand-Archive -Path $archivePath -DestinationPath $workDir -Force
        $exePath = Join-Path $workDir "$Binary.exe"
        if (-not (Test-Path $exePath)) {
            throw "$Binary.exe was not found inside $archiveName"
        }

        $installDir = $env:NERV_INSTALL_DIR
        if (-not $installDir) {
            $installDir = Join-Path $env:LOCALAPPDATA "$Binary\bin"
        }
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        $destExe = Join-Path $installDir "$Binary.exe"
        Copy-Item -Path $exePath -Destination $destExe -Force
        Write-Log "Installed $Binary to $destExe"

        Add-ToUserPath -Dir $installDir
        return $destExe
    }
    finally {
        Remove-Item -Path $workDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Test-Checksum {
    param([string]$ArchivePath, [string]$ChecksumsPath)

    $name = Split-Path -Leaf $ArchivePath
    # checksums.txt lines are "<sha256>  <file name>": compare the name field
    # exactly, so a dotted version or a longer file name never mismatches.
    $line = Get-Content -Path $ChecksumsPath |
        Where-Object { ($_.Trim() -split '\s+')[1] -eq $name } |
        Select-Object -First 1
    if (-not $line) {
        throw "no checksum entry for $name in checksums.txt"
    }
    $expected = ($line.Trim() -split '\s+')[0]
    $actual = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash
    if ($expected.ToLower() -ne $actual.ToLower()) {
        throw "checksum mismatch for $name`: expected $expected, got $actual"
    }
}

function Add-ToUserPath {
    param([string]$Dir)

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not $userPath) {
        $userPath = ""
    }
    $parts = $userPath -split ";" | Where-Object { $_ -ne "" }
    if ($parts -contains $Dir) {
        return
    }
    $newPath = if ($userPath) { "$userPath;$Dir" } else { $Dir }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    $env:Path = "$env:Path;$Dir"
    Write-Log "Added $Dir to your user PATH. Restart your shell for it to take effect everywhere."
}

function Invoke-NervInstall {
    param([string]$ExePath)

    if ($env:NERV_NO_INSTALL) {
        Write-Log "NERV_NO_INSTALL is set: skipping nerv install."
        return
    }
    & $ExePath install
}

$exe = Install-Nerv
Invoke-NervInstall -ExePath $exe
