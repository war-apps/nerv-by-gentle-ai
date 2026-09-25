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

# --- Engram "nerv" knowledge-base project (idempotent check-or-create) ---
# NERV mirrors precedents/decisions to a shared Engram project named "nerv"
# across every NERV-governed repo (see README "Engram project detection").
# This step only verifies/creates that project; it never touches
# settings.json and never aborts the install on failure.
if (-not $Uninstall) {
    $engramCommand = Get-Command engram -ErrorAction SilentlyContinue
    if ($null -eq $engramCommand) {
        Write-Warning "engram not found on PATH; could not verify the 'nerv' Engram knowledge base."
    }
    else {
        $global:LASTEXITCODE = 0
        $nervProjectExists = $false
        try {
            $projectsOutput = & engram projects list 2>&1
            if ($LASTEXITCODE -ne 0) {
                Write-Warning "engram projects list exited with code ${LASTEXITCODE}: $projectsOutput"
            }
            else {
                foreach ($line in $projectsOutput) {
                    $firstToken = ([string]$line).Trim() -split '\s+' | Select-Object -First 1
                    if ($firstToken -eq "nerv") {
                        $nervProjectExists = $true
                        break
                    }
                }
            }
        }
        catch {
            Write-Warning "engram projects list could not run: $_"
        }

        if ($nervProjectExists) {
            Write-Host "Engram 'nerv' knowledge base already exists."
        }
        else {
            $global:LASTEXITCODE = 0
            try {
                & engram save "NERV knowledge base" "Shared Engram project for NERV runs: precedents (Misato rulings, Fuyutsuki vetoes, MAGI vote results, Kaji audit findings) mirrored from every NERV-governed repository." --project nerv --type manual
                if ($LASTEXITCODE -ne 0) {
                    Write-Warning "engram save exited with code ${LASTEXITCODE}; could not create the 'nerv' Engram knowledge base."
                }
                else {
                    Write-Host "Engram 'nerv' knowledge base created."
                }
            }
            catch {
                Write-Warning "engram save could not run: $_"
            }
        }
    }
}

# --- Optional cache refresh: re-snapshot the plugin cache from committed
#     HEAD (registration above only touches settings.json; Claude Code
#     itself owns the cache snapshot under a `claude plugin` verb). ---
if ($RefreshCache) {
    Write-Host ""
    Write-Host "=== Refreshing plugin cache (nerv@nerv) ===" -ForegroundColor Cyan

    # Each external call resets $LASTEXITCODE first and treats an
    # unresolvable command as a failure; any failure aborts the refresh
    # instead of falling through to the success sentence.
    foreach ($verb in @('uninstall', 'install')) {
        Write-Host "-> claude plugin $verb nerv@nerv"
        $global:LASTEXITCODE = 0
        try {
            & claude plugin $verb nerv@nerv
        }
        catch {
            Write-Error "claude plugin $verb nerv@nerv could not run: $_"
            exit 1
        }
        if ($LASTEXITCODE -ne 0) {
            Write-Error "claude plugin $verb nerv@nerv exited with code $LASTEXITCODE. Cache NOT refreshed."
            exit 1
        }
    }

    # Readback: the cached gitCommitSha must equal the repository HEAD.
    $homeDir = if ($env:HOME) { $env:HOME } else { $env:USERPROFILE }
    $installedPluginsPath = Join-Path $homeDir ".claude/plugins/installed_plugins.json"
    if (-not (Test-Path -LiteralPath $installedPluginsPath)) {
        Write-Error "$installedPluginsPath not found after install; cannot confirm the cached commit."
        exit 1
    }
    $global:LASTEXITCODE = 0
    $repoHead = (& git -C $RepoPath rev-parse HEAD 2>$null)
    if ($LASTEXITCODE -ne 0 -or -not $repoHead) {
        Write-Error "git rev-parse HEAD failed in $RepoPath; cannot confirm the cached commit."
        exit 1
    }
    try {
        $installedPlugins = Get-Content -LiteralPath $installedPluginsPath -Raw -Encoding UTF8 | ConvertFrom-Json -Depth 50
        # installed_plugins.json nests entries under "plugins" and stores an array per plugin id
        $nervEntries = $installedPlugins.plugins.'nerv@nerv'
        if (-not $nervEntries) { $nervEntries = $installedPlugins.'nerv@nerv' }
        $nervEntry = @($nervEntries) | Select-Object -First 1
    }
    catch {
        Write-Error "Could not parse $installedPluginsPath : $_"
        exit 1
    }
    $cachedSha = if ($nervEntry) { $nervEntry.gitCommitSha } else { $null }
    if (-not $cachedSha) {
        Write-Error "nerv@nerv not found in $installedPluginsPath after install."
        exit 1
    }
    if ($cachedSha -ne $repoHead) {
        Write-Error "Cached gitCommitSha $cachedSha does not match repository HEAD $repoHead. Commit first, then refresh again."
        exit 1
    }
    Write-Host "nerv@nerv gitCommitSha : $cachedSha (matches HEAD; installPath: $($nervEntry.installPath))"
    Write-Host "Cache refreshed from the repo's committed HEAD. Restart Claude Code."
}

Write-Host ""
Write-Host "Restart Claude Code for the change to take effect."
