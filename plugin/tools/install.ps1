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
    call — registration runs first, the cache refresh runs after. Also runs
    the same model/effort apply step as -ApplyModels, at the end, once the
    cache readback confirms the refreshed sha.

.PARAMETER Skills
    Install the skills the plugin defaults reference (tools/install-skills.ps1):
    external ones from skills.sh with npx skills add -g, gentle-ai-shipped ones
    verified only. Idempotent; already-present skills are left alone.

.PARAMETER Configure
    Runs `tools/configure.ps1 -RepoPath $RepoPath -NoRefresh` at the end of
    this script's main body (skipped on -Uninstall). Without -Configure, if
    no user-scope `~/.claude/nerv/nerv.yaml` is found, a one-line hint to
    run the configuration wizard is printed instead.

.PARAMETER ApplyModels
    Applies the `models:` block from the user-scope `~/.claude/nerv/nerv.yaml`
    (never the project-scope file — effort is a local-cache concern) to the
    cached agent frontmatter under
    `~/.claude/plugins/cache/nerv/nerv/<version>/agents/` (version read from
    `plugin/.claude-plugin/plugin.json`). For each role in `models:`, rewrites
    that agent's `model:`/`effort:` frontmatter keys (adding `effort:` when
    absent), resolving `from: <gentle-ai-phase>` entries against
    `~/.gentle-ai/state.json`'s `claude_phase_assignments`. Idempotent, and
    safe to run any time after the plugin cache exists — a missing
    `models:` block or a missing cache directory is informational, not an
    error. `-RefreshCache` runs this same step automatically; use
    `-ApplyModels` on its own after only editing `models:` in
    `~/.claude/nerv/nerv.yaml`, with no code change to refresh.

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

.EXAMPLE
    pwsh tools/install.ps1 -ApplyModels
#>

[CmdletBinding()]
param(
    [string]$SettingsPath = (Join-Path $env:USERPROFILE ".claude\settings.json"),

    [string]$RepoPath = (Split-Path -Parent $PSScriptRoot),

    [switch]$Uninstall,

    [switch]$RequireGentleAi,

    [switch]$RefreshCache,

    [switch]$ApplyModels,

    [switch]$Configure,
    [switch]$Skills
)

# =============================================================================
# Function definitions — dot-sourceable, no side effects of their own.
#
# Everything below this block up to the closing brace of the
# `if ($MyInvocation.InvocationName -ne '.')` guard is this script's main
# execution body. Dot-sourcing this file (`. tools/install.ps1 ...`, as
# tests/install-apply-models.test.ps1 does) sets $MyInvocation.InvocationName
# to '.', so the guard skips the whole body — including the gentle-ai
# preflight, the settings.json mutation, and the Engram/RefreshCache calls —
# and only these functions get defined in the caller's scope.
# =============================================================================

function Get-NervHomeDir {
    <#
    .SYNOPSIS
        Resolves the user's home directory: $env:HOME first, then
        $env:USERPROFILE. Matches the fallback already used elsewhere in
        this script for `~/.claude/plugins/installed_plugins.json`.
    #>
    if ($env:HOME) { return $env:HOME }
    return $env:USERPROFILE
}

function Resolve-NervModelAssignments {
    <#
    .SYNOPSIS
        Parses the `models:` block (inline-map syntax) out of a NERV
        user-scope nerv.yaml and returns a hashtable of
        role -> @{ Model = '...'; Effort = '...' } (only the keys that
        actually resolved are present per role).

    .DESCRIPTION
        Only the `models:` top-level key is parsed — this is not a general
        YAML parser. Each entry line inside the block must look like:
          <role>: { model: <value>, effort: <value> }
        or
          <role>: { from: <gentle-ai-phase> }
        with any mix of `model`, `effort`, `from`, spaces, quoted or bare
        values, and a trailing `# comment` tolerated. `from:` resolves
        against StatePath's `claude_phase_assignments[<phase>]`; an explicit
        `model`/`effort` on the same line wins over the one inherited via
        `from:`. Invalid `model`/`effort` values, and `from:` phases missing
        from the state file, are warned about and the whole role entry is
        skipped (never partially applied).
    #>
    [CmdletBinding()]
    param(
        [string]$ConfigPath = (Join-Path (Get-NervHomeDir) ".claude/nerv/nerv.yaml"),
        [string]$StatePath = (Join-Path (Get-NervHomeDir) ".gentle-ai/state.json")
    )

    $assignments = @{}

    if (-not (Test-Path -LiteralPath $ConfigPath)) {
        Write-Host "No NERV models: overrides configured ($ConfigPath not found); plugin defaults apply."
        return $assignments
    }

    $raw = Get-Content -LiteralPath $ConfigPath -Raw -Encoding UTF8
    $lines = ($raw -replace "`r`n", "`n") -split "`n"

    # Locate the top-level "models:" key (column 0) and collect the indented
    # lines that follow it, stopping at the next de-indented (or EOF) line.
    $blockLines = [System.Collections.Generic.List[string]]::new()
    $inBlock = $false
    foreach ($line in $lines) {
        if (-not $inBlock) {
            if ($line -match '^models:\s*(#.*)?$') {
                $inBlock = $true
            }
            continue
        }
        # Blank lines and full-line comments never end the block: the
        # /nerv:init template interleaves column-0 "# role: {...}" lines
        # with the entries the user uncomments.
        if ($line.Trim().Length -eq 0 -or $line -match '^\s*#') {
            continue
        }
        if ($line -match '^\s') {
            $blockLines.Add($line)
        }
        else {
            break
        }
    }

    if (-not $inBlock) {
        Write-Host "No 'models:' section found in $ConfigPath; plugin defaults apply."
        return $assignments
    }

    $statePhaseAssignments = $null
    $stateLoadAttempted = $false

    $validModelPattern = '^(sonnet|opus|haiku|fable|inherit)$|^claude-.+$'
    $validEffortPattern = '^(low|medium|high|xhigh|max)$'

    foreach ($line in $blockLines) {
        $entryMatch = [regex]::Match($line, '^\s*(?<role>[A-Za-z0-9_-]+):\s*\{(?<body>[^}]*)\}\s*(#.*)?$')
        if (-not $entryMatch.Success) {
            Write-Warning "Could not parse models: entry, skipping line: $($line.Trim())"
            continue
        }

        $role = $entryMatch.Groups['role'].Value
        $body = $entryMatch.Groups['body'].Value

        $fields = @{}
        foreach ($pair in ($body -split ',')) {
            if ($pair.Trim().Length -eq 0) { continue }
            $kv = $pair -split ':', 2
            if ($kv.Count -ne 2) { continue }
            $key = $kv[0].Trim()
            $value = $kv[1].Trim().Trim('"').Trim("'")
            $fields[$key] = $value
        }

        $model = $null
        $effort = $null
        $hasModel = $false
        $hasEffort = $false
        $skipRole = $false

        if ($fields.ContainsKey('from')) {
            if (-not $stateLoadAttempted) {
                $stateLoadAttempted = $true
                if (Test-Path -LiteralPath $StatePath) {
                    try {
                        $state = Get-Content -LiteralPath $StatePath -Raw -Encoding UTF8 | ConvertFrom-Json -Depth 50
                        $statePhaseAssignments = $state.claude_phase_assignments
                    }
                    catch {
                        Write-Warning "Could not parse $StatePath : $_"
                        $statePhaseAssignments = $null
                    }
                }
                else {
                    Write-Warning "gentle-ai state file not found at $StatePath; 'from:' entries cannot be resolved."
                }
            }

            $phase = $fields['from']
            $phaseEntry = $null
            if ($statePhaseAssignments -and $statePhaseAssignments.PSObject.Properties[$phase]) {
                $phaseEntry = $statePhaseAssignments.$phase
            }

            if (-not $phaseEntry) {
                Write-Warning "role '$role': from: $phase not found in $StatePath; skipping."
                $skipRole = $true
            }
            else {
                if ($phaseEntry.PSObject.Properties['model']) {
                    $model = [string]$phaseEntry.model
                    $hasModel = $true
                }
                if ($phaseEntry.PSObject.Properties['effort']) {
                    $effort = [string]$phaseEntry.effort
                    $hasEffort = $true
                }
            }
        }

        if ($skipRole) { continue }

        if ($fields.ContainsKey('model')) {
            $model = $fields['model']
            $hasModel = $true
        }
        if ($fields.ContainsKey('effort')) {
            $effort = $fields['effort']
            $hasEffort = $true
        }

        if ($hasModel -and ($model -notmatch $validModelPattern)) {
            Write-Warning "role '$role': invalid model '$model'; skipping."
            continue
        }
        if ($hasEffort -and ($effort -notmatch $validEffortPattern)) {
            Write-Warning "role '$role': invalid effort '$effort'; skipping."
            continue
        }
        if (-not $hasModel -and -not $hasEffort) {
            Write-Warning "role '$role': models: entry resolved neither model nor effort; skipping."
            continue
        }

        $entry = @{}
        if ($hasModel) { $entry['Model'] = $model }
        if ($hasEffort) { $entry['Effort'] = $effort }
        $assignments[$role] = $entry
    }

    return $assignments
}

function Set-NervAgentFrontmatter {
    <#
    .SYNOPSIS
        Rewrites the `model:`/`effort:` frontmatter keys of each role's
        cached agent file according to $Assignments (as returned by
        Resolve-NervModelAssignments).

    .DESCRIPTION
        For each role: opens <AgentsDir>/<role>.md (warns and skips when
        missing), locates the YAML frontmatter between the first two `---`
        lines (warns and skips when malformed), and replaces the value
        token on the `model:` line and, when present, the `effort:` line —
        preserving any trailing `# comment` on either line verbatim. When an
        Effort assignment is present but the file has no `effort:` line
        yet, one is inserted immediately after `model:`. The body (anything
        after the closing `---`) is never touched. Detects and preserves
        the file's own line-ending style (CRLF or LF) and always writes
        UTF-8 without a BOM. Prints one line per role whose frontmatter
        actually changed, plus a one-line summary.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$AgentsDir,
        [Parameter(Mandatory)][hashtable]$Assignments
    )

    $changedCount = 0
    $unchangedCount = 0
    $skippedCount = 0

    foreach ($role in ($Assignments.Keys | Sort-Object)) {
        $assignment = $Assignments[$role]
        $agentFile = Join-Path $AgentsDir "$role.md"

        if (-not (Test-Path -LiteralPath $agentFile)) {
            Write-Warning "Agent file not found, skipping: $agentFile"
            $skippedCount++
            continue
        }

        $content = [System.IO.File]::ReadAllText($agentFile)
        $eol = if ($content -match "`r`n") { "`r`n" } else { "`n" }
        $lines = [System.Collections.Generic.List[string]]::new()
        $lines.AddRange([string[]]($content -split "`r`n|`n"))

        if ($lines.Count -lt 2 -or $lines[0].Trim() -ne '---') {
            Write-Warning "Malformed frontmatter (missing opening ---), skipping: $agentFile"
            $skippedCount++
            continue
        }

        $closeIdx = -1
        for ($i = 1; $i -lt $lines.Count; $i++) {
            if ($lines[$i].Trim() -eq '---') { $closeIdx = $i; break }
        }
        if ($closeIdx -lt 0) {
            Write-Warning "Malformed frontmatter (missing closing ---), skipping: $agentFile"
            $skippedCount++
            continue
        }

        $modelLineIdx = -1
        $effortLineIdx = -1
        for ($i = 1; $i -lt $closeIdx; $i++) {
            if ($lines[$i] -match '^model:\s') { $modelLineIdx = $i }
            elseif ($lines[$i] -match '^effort:\s') { $effortLineIdx = $i }
        }

        if ($modelLineIdx -lt 0) {
            Write-Warning "No 'model:' key in frontmatter, skipping: $agentFile"
            $skippedCount++
            continue
        }

        $oldModel = $null
        $oldEffort = $null
        $modelChanged = $false
        $effortChanged = $false

        if ($assignment.ContainsKey('Model')) {
            $m = [regex]::Match($lines[$modelLineIdx], '^model:(\s*)(\S+)(.*)$')
            if ($m.Success) {
                $oldModel = $m.Groups[2].Value
                $newModel = $assignment['Model']
                if ($oldModel -ne $newModel) {
                    $lines[$modelLineIdx] = "model:$($m.Groups[1].Value)$newModel$($m.Groups[3].Value)"
                    $modelChanged = $true
                }
            }
        }

        if ($assignment.ContainsKey('Effort')) {
            $newEffort = $assignment['Effort']
            if ($effortLineIdx -ge 0) {
                $m = [regex]::Match($lines[$effortLineIdx], '^effort:(\s*)(\S+)(.*)$')
                if ($m.Success) {
                    $oldEffort = $m.Groups[2].Value
                    if ($oldEffort -ne $newEffort) {
                        $lines[$effortLineIdx] = "effort:$($m.Groups[1].Value)$newEffort$($m.Groups[3].Value)"
                        $effortChanged = $true
                    }
                }
            }
            else {
                $oldEffort = '(absent)'
                $lines.Insert($modelLineIdx + 1, "effort: $newEffort")
                $effortChanged = $true
            }
        }

        $anyChange = $modelChanged -or $effortChanged

        if ($anyChange) {
            $newContent = ($lines -join $eol)
            $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
            [System.IO.File]::WriteAllText($agentFile, $newContent, $utf8NoBom)

            $parts = @()
            if ($modelChanged) {
                $parts += "model $oldModel -> $($assignment['Model'])"
            }
            if ($effortChanged) {
                $parts += "effort $oldEffort -> $($assignment['Effort'])"
            }
            Write-Host "$role`: $($parts -join ', ')"
            $changedCount++
        }
        else {
            $unchangedCount++
        }
    }

    Write-Host "Model/effort apply summary: $changedCount changed, $unchangedCount already up to date, $skippedCount skipped."
}

function Get-NervPluginDefaults {
    <#
    .SYNOPSIS
        Reads the committed model/effort frontmatter of every agent in the
        repo's plugin/agents directory: the plugin defaults. Applying the
        merged table (defaults + overrides) means that removing an override
        from nerv.yaml restores the default on the next -ApplyModels, instead
        of leaving the previous override in the cache.
    #>
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$AgentsDir)

    $defaults = @{}
    if (-not (Test-Path -LiteralPath $AgentsDir)) {
        Write-Warning "Plugin agents directory not found: $AgentsDir; no defaults available."
        return $defaults
    }
    foreach ($file in Get-ChildItem -LiteralPath $AgentsDir -Filter '*.md' -File) {
        $role = [System.IO.Path]::GetFileNameWithoutExtension($file.Name)
        $lines = [System.IO.File]::ReadAllText($file.FullName) -split "`r`n|`n"
        if ($lines.Count -lt 2 -or $lines[0].Trim() -ne '---') { continue }
        $entry = @{}
        for ($i = 1; $i -lt $lines.Count; $i++) {
            $line = $lines[$i]
            if ($line.Trim() -eq '---') { break }
            if ($line -match '^model:\s*([^#\s]+)') { $entry['Model'] = $Matches[1].Trim() }
            elseif ($line -match '^effort:\s*([^#\s]+)') { $entry['Effort'] = $Matches[1].Trim() }
        }
        if ($entry.Count -gt 0) { $defaults[$role] = $entry }
    }
    return $defaults
}

function Merge-NervModelAssignments {
    <#
    .SYNOPSIS
        Overlays the nerv.yaml overrides on the plugin defaults, key by key:
        an override that names only model keeps the default effort, and
        vice versa. Roles present only in the overrides are kept as is.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][hashtable]$Defaults,
        [Parameter(Mandatory)][hashtable]$Overrides
    )
    $merged = @{}
    foreach ($role in $Defaults.Keys) {
        $merged[$role] = @{}
        foreach ($k in $Defaults[$role].Keys) { $merged[$role][$k] = $Defaults[$role][$k] }
    }
    foreach ($role in $Overrides.Keys) {
        if (-not $merged.ContainsKey($role)) { $merged[$role] = @{} }
        foreach ($k in $Overrides[$role].Keys) { $merged[$role][$k] = $Overrides[$role][$k] }
    }
    return $merged
}

function Invoke-NervApplyModels {
    <#
    .SYNOPSIS
        Resolves the user-scope models: block and applies it to the plugin
        cache's agent frontmatter. Shared by -ApplyModels and -RefreshCache.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$RepoPath
    )

    $pluginJsonPath = Join-Path $RepoPath "plugin/.claude-plugin/plugin.json"
    if (-not (Test-Path -LiteralPath $pluginJsonPath)) {
        Write-Warning "plugin.json not found at $pluginJsonPath; cannot resolve the plugin cache version. Skipping model/effort apply."
        return
    }

    $version = $null
    try {
        $pluginJson = Get-Content -LiteralPath $pluginJsonPath -Raw -Encoding UTF8 | ConvertFrom-Json -Depth 10
        $version = $pluginJson.version
    }
    catch {
        Write-Warning "Could not parse $pluginJsonPath : $_. Skipping model/effort apply."
        return
    }
    if (-not $version) {
        Write-Warning "plugin.json at $pluginJsonPath has no 'version'; cannot resolve the plugin cache. Skipping model/effort apply."
        return
    }

    $agentsDir = Join-Path (Get-NervHomeDir) ".claude/plugins/cache/nerv/nerv/$version/agents"

    Write-Host ""
    Write-Host "=== Applying model/effort assignments (nerv@nerv $version) ===" -ForegroundColor Cyan

    if (-not (Test-Path -LiteralPath $agentsDir)) {
        Write-Warning "Plugin cache agents directory not found: $agentsDir. Install/refresh the plugin first."
        return
    }

    $assignments = Resolve-NervModelAssignments
    $defaults = Get-NervPluginDefaults -AgentsDir (Join-Path $RepoPath "plugin/agents")
    if ($assignments.Count -eq 0) {
        Write-Host "Applying plugin defaults to the cached agents (restores any override removed from nerv.yaml)."
    }
    $merged = Merge-NervModelAssignments -Defaults $defaults -Overrides $assignments

    Set-NervAgentFrontmatter -AgentsDir $agentsDir -Assignments $merged
}

if ($MyInvocation.InvocationName -ne '.') {

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

    Invoke-NervApplyModels -RepoPath $RepoPath
}
elseif ($ApplyModels) {
    Invoke-NervApplyModels -RepoPath $RepoPath
}

if (-not $Uninstall) {
    if ($Skills) {
        Write-Host ""
        Write-Host "=== Installing required skills ===" -ForegroundColor Cyan
        $global:LASTEXITCODE = 0
        & pwsh -NoProfile -File (Join-Path $PSScriptRoot "install-skills.ps1")
        if ($LASTEXITCODE -ne 0) { Write-Warning "install-skills.ps1 reported failures; see the lines above." }
    }
    if ($Configure) {
        Write-Host ""
        Write-Host "=== Running configuration wizard ===" -ForegroundColor Cyan
        & pwsh -NoProfile -File (Join-Path $PSScriptRoot "configure.ps1") -RepoPath $RepoPath -NoRefresh
    }
    else {
        $homeDirForHint = Get-NervHomeDir
        $userConfigHintPath = Join-Path $homeDirForHint ".claude/nerv/nerv.yaml"
        if (-not (Test-Path -LiteralPath $userConfigHintPath)) {
            Write-Host ""
            Write-Host "No user config found: run pwsh tools/configure.ps1 to set up NERV (or re-run with -Configure)."
        }
    }
}

Write-Host ""
Write-Host "Restart Claude Code for the change to take effect."

} # end of `if ($MyInvocation.InvocationName -ne '.')` main-execution guard
