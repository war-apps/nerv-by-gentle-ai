#requires -Version 7
<#
.SYNOPSIS
    Registers (or removes) the NERV plugin marketplace in Claude Code's
    global settings.json.

.DESCRIPTION
    Adds extraKnownMarketplaces.nerv (pointing at this repo as a local
    "directory" marketplace) and enabledPlugins["nerv@nerv"] = $true to
    settings.json, preserving every other key untouched. Idempotent: running
    it twice produces the same settings.json (after the second run reports
    no changes). Always backs up settings.json before writing.

.PARAMETER SettingsPath
    Path to Claude Code's settings.json. Defaults to
    "$env:USERPROFILE\.claude\settings.json".

.PARAMETER RepoPath
    Path to the NERV repo root (the directory containing .claude-plugin\
    marketplace.json). Defaults to the parent of this script's directory
    (tools\..), i.e. the repo root when the script is run in place.

.PARAMETER Uninstall
    Remove extraKnownMarketplaces.nerv and enabledPlugins["nerv@nerv"]
    instead of adding them.

.EXAMPLE
    pwsh tools/install.ps1

.EXAMPLE
    pwsh tools/install.ps1 -Uninstall
#>

[CmdletBinding()]
param(
    [string]$SettingsPath = (Join-Path $env:USERPROFILE ".claude\settings.json"),

    [string]$RepoPath = (Split-Path -Parent $PSScriptRoot),

    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $SettingsPath)) {
    throw "settings.json not found at: $SettingsPath"
}

$RepoPath = (Resolve-Path -LiteralPath $RepoPath).ProviderPath
# Normalize to backslashes for the JSON value, matching the shape of the
# existing dim-tools entry in settings.json.
$repoPathNormalized = $RepoPath -replace '/', '\'

Write-Host "Settings file : $SettingsPath"
Write-Host "Repo path     : $repoPathNormalized"
Write-Host ""

$raw = Get-Content -LiteralPath $SettingsPath -Raw -Encoding UTF8
$settings = $raw | ConvertFrom-Json -Depth 50

$changed = $false

function Ensure-Property {
    param($Object, [string]$Name, $Value)
    if ($null -eq $Object.PSObject.Properties[$Name]) {
        $Object | Add-Member -MemberType NoteProperty -Name $Name -Value $Value
        return $true
    }
    else {
        $Object.$Name = $Value
        return $true
    }
}

if (-not $Uninstall) {
    # --- extraKnownMarketplaces.nerv ---
    if ($null -eq $settings.PSObject.Properties["extraKnownMarketplaces"]) {
        $settings | Add-Member -MemberType NoteProperty -Name "extraKnownMarketplaces" -Value ([pscustomobject]@{})
        $changed = $true
    }

    $desiredMarketplace = [pscustomobject]@{
        source = [pscustomobject]@{
            source = "directory"
            path   = $repoPathNormalized
        }
    }

    $existingMarketplace = $settings.extraKnownMarketplaces.PSObject.Properties["nerv"]
    $marketplaceJson = ($desiredMarketplace | ConvertTo-Json -Depth 10 -Compress)
    $existingMarketplaceJson = if ($existingMarketplace) { ($existingMarketplace.Value | ConvertTo-Json -Depth 10 -Compress) } else { $null }

    if ($existingMarketplaceJson -ne $marketplaceJson) {
        if (Ensure-Property -Object $settings.extraKnownMarketplaces -Name "nerv" -Value $desiredMarketplace) {
            $changed = $true
            Write-Host "extraKnownMarketplaces.nerv -> path: $repoPathNormalized"
        }
    }
    else {
        Write-Host "extraKnownMarketplaces.nerv already up to date."
    }

    # --- enabledPlugins["nerv@nerv"] ---
    if ($null -eq $settings.PSObject.Properties["enabledPlugins"]) {
        $settings | Add-Member -MemberType NoteProperty -Name "enabledPlugins" -Value ([pscustomobject]@{})
        $changed = $true
    }

    $pluginKey = "nerv@nerv"
    $existingPluginValue = $settings.enabledPlugins.PSObject.Properties[$pluginKey]
    if ($null -eq $existingPluginValue -or $existingPluginValue.Value -ne $true) {
        Ensure-Property -Object $settings.enabledPlugins -Name $pluginKey -Value $true | Out-Null
        $changed = $true
        Write-Host "enabledPlugins[`"$pluginKey`"] -> true"
    }
    else {
        Write-Host "enabledPlugins[`"$pluginKey`"] already true."
    }
}
else {
    # --- Uninstall: remove both keys if present ---
    if ($settings.PSObject.Properties["extraKnownMarketplaces"] -and
        $settings.extraKnownMarketplaces.PSObject.Properties["nerv"]) {
        $settings.extraKnownMarketplaces.PSObject.Properties.Remove("nerv")
        $changed = $true
        Write-Host "Removed extraKnownMarketplaces.nerv"
    }
    else {
        Write-Host "extraKnownMarketplaces.nerv not present."
    }

    if ($settings.PSObject.Properties["enabledPlugins"] -and
        $settings.enabledPlugins.PSObject.Properties["nerv@nerv"]) {
        $settings.enabledPlugins.PSObject.Properties.Remove("nerv@nerv")
        $changed = $true
        Write-Host "Removed enabledPlugins[`"nerv@nerv`"]"
    }
    else {
        Write-Host "enabledPlugins[`"nerv@nerv`"] not present."
    }
}

if (-not $changed) {
    Write-Host ""
    Write-Host "No changes needed. settings.json left untouched."
    exit 0
}

# --- Backup before writing ---
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$backupPath = "$SettingsPath.bak-nerv-$timestamp"
Copy-Item -LiteralPath $SettingsPath -Destination $backupPath -Force
Write-Host ""
Write-Host "Backup written: $backupPath"

# --- Write to a temp file, verify it, then swap it in (never a truncating
#     write to the live file) ---
$tempPath = "$SettingsPath.tmp-nerv"

try {
    $json = $settings | ConvertTo-Json -Depth 50
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($tempPath, $json, $utf8NoBom)

    # Parse-verify the temp file before it ever touches the target.
    try {
        Get-Content -LiteralPath $tempPath -Raw -Encoding UTF8 | ConvertFrom-Json -Depth 50 | Out-Null
    }
    catch {
        Remove-Item -LiteralPath $tempPath -Force -ErrorAction SilentlyContinue
        throw "Temp settings file failed to parse-verify: $_"
    }

    Move-Item -LiteralPath $tempPath -Destination $SettingsPath -Force

    # Re-read and parse the final file once more to confirm the swap landed cleanly.
    Get-Content -LiteralPath $SettingsPath -Raw -Encoding UTF8 | ConvertFrom-Json -Depth 50 | Out-Null
    Write-Host "settings.json verified."
}
catch {
    if (Test-Path -LiteralPath $backupPath) {
        Copy-Item -LiteralPath $backupPath -Destination $SettingsPath -Force
        Write-Host "settings.json restored from backup $backupPath"
    }
    if (Test-Path -LiteralPath $tempPath) {
        Remove-Item -LiteralPath $tempPath -Force -ErrorAction SilentlyContinue
    }
    throw
}

Write-Host "settings.json updated."
Write-Host ""
Write-Host "Restart Claude Code for the change to take effect."
