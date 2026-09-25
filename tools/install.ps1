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

.PARAMETER RequireGentleAi
    Fail the install (exit 1) when gentle-ai is missing from PATH, or when
    it is present but not major version 3. Without this switch, both cases
    only print a warning and installation continues — NERV requires
    gentle-ai 3.x (tested against 3.7.0) but the installer does not enforce
    it unless asked to.

.PARAMETER RefreshCache
    Re-snapshot the plugin cache from the repo's committed HEAD: runs
    `claude plugin uninstall nerv@nerv` followed by
    `claude plugin install nerv@nerv` (both through `& claude`), then
    reports the `gitCommitSha` recorded for `nerv@nerv` in
    `~/.claude/plugins/installed_plugins.json` so you can confirm it
    matches your latest commit. Claude Code only snapshots a directory
    marketplace from committed HEAD (see README "Updating after local
    changes"), so uncommitted edits stay invisible without this. Can be
    combined with the normal marketplace/plugin registration in the same
    call — registration runs first, the cache refresh runs after.

.EXAMPLE
    pwsh tools/install.ps1

.EXAMPLE
    pwsh tools/install.ps1 -Uninstall

.EXAMPLE
    pwsh tools/install.ps1 -RequireGentleAi

.EXAMPLE
    pwsh tools/install.ps1 -RefreshCache

.EXAMPLE
    pwsh tools/install.ps1 -RefreshCache -RequireGentleAi
#>

[CmdletBinding()]
param(
    [string]$SettingsPath = (Join-Path $env:USERPROFILE ".claude\settings.json"),

    [string]$RepoPath = (Split-Path -Parent $PSScriptRoot),

    [switch]$Uninstall,

    [switch]$RequireGentleAi,

    [switch]$RefreshCache
)

# --- gentle-ai version preflight (informational unless -RequireGentleAi) ---
# NERV requires gentle-ai 3.x (major version 3; tested against 3.7.0).
$gentleAiVersionOutput = $null
try {
    $gentleAiVersionOutput = (gentle-ai --version 2>&1 | Select-Object -First 1) -as [string]
}
catch {
    $gentleAiVersionOutput = $null
}

if ([string]::IsNullOrWhiteSpace($gentleAiVersionOutput)) {
    Write-Warning "gentle-ai not found on PATH; NERV requires gentle-ai 3.x (https://github.com/Gentleman-Programming/gentle-ai)"
    if ($RequireGentleAi) {
        exit 1
    }
}
else {
    $versionMatch = [regex]::Match($gentleAiVersionOutput, '(\d+)\.(\d+)\.(\d+)')
    if ($versionMatch.Success) {
        $gentleAiVersion = $versionMatch.Value
        $gentleAiMajor = [int]$versionMatch.Groups[1].Value
        if ($gentleAiMajor -ne 3) {
            Write-Warning "NERV requires gentle-ai 3.x; found $gentleAiVersion"
            if ($RequireGentleAi) {
                exit 1
            }
        }
        else {
            Write-Host "gentle-ai version : $gentleAiVersion (tested against 3.7.0)"
        }
    }
    else {
        Write-Warning "gentle-ai --version returned an unparseable value: $gentleAiVersionOutput"
        if ($RequireGentleAi) {
            exit 1
        }
    }
}

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
}
else {
    # --- Backup before writing ---
    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $backupPath = "$SettingsPath.bak-nerv-$timestamp"
    Copy-Item -LiteralPath $SettingsPath -Destination $backupPath -Force
    Write-Host ""
    Write-Host "Backup written: $backupPath"

    # --- Write to a temp file, verify it, then swap it in (never a
    #     truncating write to the live file) ---
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
}

# --- Optional cache refresh: re-snapshot the plugin cache from committed
#     HEAD (registration above only touches settings.json; Claude Code
#     itself owns the cache snapshot under a `claude plugin` verb). ---
if ($RefreshCache) {
    Write-Host ""
    Write-Host "=== Refreshing plugin cache (nerv@nerv) ===" -ForegroundColor Cyan

    Write-Host "-> claude plugin uninstall nerv@nerv"
    & claude plugin uninstall nerv@nerv
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "claude plugin uninstall nerv@nerv exited with code $LASTEXITCODE"
    }

    Write-Host "-> claude plugin install nerv@nerv"
    & claude plugin install nerv@nerv
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "claude plugin install nerv@nerv exited with code $LASTEXITCODE"
    }

    $installedPluginsPath = Join-Path $env:USERPROFILE ".claude\plugins\installed_plugins.json"
    if (Test-Path -LiteralPath $installedPluginsPath) {
        try {
            $installedPlugins = Get-Content -LiteralPath $installedPluginsPath -Raw -Encoding UTF8 | ConvertFrom-Json -Depth 50
            # installed_plugins.json nests entries under "plugins" and stores an array per plugin id
            $nervEntries = $installedPlugins.plugins.'nerv@nerv'
            if (-not $nervEntries) { $nervEntries = $installedPlugins.'nerv@nerv' }
            $nervEntry = @($nervEntries) | Select-Object -First 1
            if ($nervEntry -and $nervEntry.gitCommitSha) {
                Write-Host "nerv@nerv gitCommitSha : $($nervEntry.gitCommitSha) (installPath: $($nervEntry.installPath))"
            }
            else {
                Write-Warning "nerv@nerv not found in $installedPluginsPath after refresh."
            }
        }
        catch {
            Write-Warning "Could not parse $installedPluginsPath : $_"
        }
    }
    else {
        Write-Warning "$installedPluginsPath not found; cannot report the cached gitCommitSha."
    }

    Write-Host "Cache refreshed from the repo's committed HEAD. Restart Claude Code."
}

Write-Host ""
Write-Host "Restart Claude Code for the change to take effect."
