#requires -Version 7
<#
.SYNOPSIS
    Interactive wizard for the NERV `models:` block: per-role model and
    effort overrides in nerv.yaml.

.DESCRIPTION
    Reads the resolved model/effort table (plugin defaults, current
    nerv.yaml overrides, and gentle-ai `from:` phases), lets the user edit
    it role by role or group by group, then writes the resulting `models:`
    block back to nerv.yaml — backing up the file first. At user scope it
    can also apply the change to the plugin cache immediately via
    `tools/install.ps1 -ApplyModels` (see that script's `-ApplyModels`
    help for what "apply" means and why effort needs it).

    Reuses tools/install.ps1's shared functions (Get-NervHomeDir,
    Resolve-NervModelAssignments, Get-NervPluginDefaults,
    Merge-NervModelAssignments, Invoke-NervApplyModels) by dot-sourcing it;
    that script's own side-effecting body stays inert because it is guarded
    by `if ($MyInvocation.InvocationName -ne '.')`.

.PARAMETER Scope
    'user' (default) edits `~/.claude/nerv/nerv.yaml`; 'project' edits
    `<ProjectDir>/.nerv/nerv.yaml`, which must already exist (created by
    `/nerv:init`). Project-scope `effort` only pins `model` in practice —
    `effort` needs `-ApplyModels` against the user-scope cache to take
    effect (see README "Configuring models and effort").

.PARAMETER RepoPath
    Path to the NERV repo root (for plugin/agents defaults and
    -ApplyModels). Defaults to the grandparent of this script's directory
    (plugin\tools\..\..), i.e. the repo root when this script runs in
    place from the repo. When this script instead runs from the installed
    plugin cache, that grandparent is not the repo; -ApplyModels then
    delegates to install.ps1's own Invoke-NervApplyModels, whose plugin
    defaults lookup degrades to a warning (informational, not fatal) when
    plugin/agents cannot be found under the guessed path. Pass -RepoPath
    explicitly to bypass this.

.PARAMETER ProjectDir
    Directory whose `.nerv/nerv.yaml` is edited when -Scope project.
    Defaults to the current directory.

.PARAMETER ConfigPath
    Overrides the resolved nerv.yaml path for either scope.

.PARAMETER StatePath
    Overrides gentle-ai's state.json path (defaults to
    `~/.gentle-ai/state.json`), used to resolve and list `from:` phases.

.PARAMETER NoApply
    Skip the "apply to the plugin cache now" prompt at the end (user scope
    only; project scope never offers it).

.PARAMETER AnswersFile
    A text file with one answer per line, consumed instead of Read-Host —
    lets the wizard run non-interactively (tests, scripted setup). When the
    answers run out, the wizard behaves as if "done" (and then "Y" for any
    remaining confirmations) had been typed.

.EXAMPLE
    pwsh tools/configure-models.ps1

.EXAMPLE
    pwsh tools/configure-models.ps1 -Scope project

.EXAMPLE
    pwsh tools/configure-models.ps1 -NoApply -AnswersFile answers.txt
#>

[CmdletBinding()]
param(
    [ValidateSet('user', 'project')]
    [string]$Scope = 'user',

    [string]$RepoPath = (Split-Path -Parent (Split-Path -Parent $PSScriptRoot)),

    [string]$ProjectDir = (Get-Location).Path,

    [string]$ConfigPath,

    [string]$StatePath,

    [switch]$NoApply,

    [string]$AnswersFile
)

# =============================================================================
# Shared functions from tools/install.ps1. Dot-sourcing with a bogus
# -SettingsPath is safe regardless: install.ps1's own main body is guarded by
# `if ($MyInvocation.InvocationName -ne '.')`, which is true whenever it is
# dot-sourced (as it is here), so only its functions get defined.
# =============================================================================
$installScriptPath = Join-Path $PSScriptRoot 'install.ps1'
$installGuardPath = Join-Path ([System.IO.Path]::GetTempPath()) 'nerv-configure-models-install-guard-nonexistent.json'
. $installScriptPath -SettingsPath $installGuardPath -RepoPath $RepoPath 2>$null

# =============================================================================
# Pure functions — dot-sourceable, no side effects of their own. Everything
# below this block up to the closing brace of the
# `if ($MyInvocation.InvocationName -ne '.')` guard is this script's
# interactive execution body.
# =============================================================================

function Format-NervModelsBlock {
    <#
    .SYNOPSIS
        Renders a `models:` block from a role -> @{ Model; Effort; From }
        override map: roles sorted, one inline-map line per role, LF line
        endings, no trailing blank line, preceded by a documented header
        comment.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][hashtable]$Overrides
    )

    $header = 'models:                             # per-role model and effort (written by tools/configure-models.ps1)'

    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.Add($header)

    foreach ($role in ($Overrides.Keys | Sort-Object)) {
        $entry = $Overrides[$role]
        $parts = [System.Collections.Generic.List[string]]::new()
        if ($entry.ContainsKey('From')) { $parts.Add("from: $($entry['From'])") }
        if ($entry.ContainsKey('Model')) { $parts.Add("model: $($entry['Model'])") }
        if ($entry.ContainsKey('Effort')) { $parts.Add("effort: $($entry['Effort'])") }
        if ($parts.Count -eq 0) { continue }
        $lines.Add("  ${role}: { $($parts -join ', ') }")
    }

    return ($lines -join "`n")
}

function Set-NervYamlModelsBlock {
    <#
    .SYNOPSIS
        Replaces, appends, or removes the top-level `models:` block inside
        a nerv.yaml text blob.

    .DESCRIPTION
        Locates an existing top-level `models:` key (column 0) and every
        following blank, full-line-comment, or indented line, up to the
        next column-0 key — the same scan tools/install.ps1's
        Resolve-NervModelAssignments uses — and replaces that whole run
        with $BlockText. When no `models:` key exists, appends $BlockText
        after one blank line. When $BlockText is empty or $null, the
        existing block (if any) is removed instead. Everything outside the
        replaced region is preserved; the file's own EOL style (CRLF or LF,
        detected from $YamlText) is used to join the result, including for
        $BlockText's own lines.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][AllowEmptyString()][string]$YamlText,
        [AllowEmptyString()][AllowNull()][string]$BlockText
    )

    $eol = if ($YamlText -match "`r`n") { "`r`n" } else { "`n" }
    $lines = $YamlText -split "`r`n|`n"

    $startIdx = -1
    $endIdx = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match '^models:\s*(#.*)?$') {
            $startIdx = $i
            $j = $i + 1
            while ($j -lt $lines.Count) {
                $line = $lines[$j]
                if ($line.Trim().Length -eq 0) { $j++; continue }
                if ($line -match '^\s*#') { $j++; continue }
                if ($line -match '^\s') { $j++; continue }
                break
            }
            $endIdx = $j
            break
        }
    }

    $blockLines = @()
    if ($BlockText) {
        $blockLines = @($BlockText -split "`r`n|`n")
    }

    if ($startIdx -ge 0) {
        $before = if ($startIdx -gt 0) { @($lines[0..($startIdx - 1)]) } else { @() }
        $after = if ($endIdx -lt $lines.Count) { @($lines[$endIdx..($lines.Count - 1)]) } else { @() }

        $newLines = @()
        $newLines += $before
        $newLines += $blockLines
        $newLines += $after
        $joined = ($newLines -join $eol)
        if (-not $joined.EndsWith($eol)) { $joined += $eol }
        return $joined
    }
    else {
        if ($blockLines.Count -eq 0) {
            return $YamlText
        }
        $base = $YamlText -replace '(\r\n|\n)+$', ''
        $blockJoined = $blockLines -join $eol
        if ($base.Length -eq 0) {
            return $blockJoined + $eol
        }
        return $base + $eol + $eol + $blockJoined + $eol
    }
}

function Read-NervModelsOverrides {
    <#
    .SYNOPSIS
        Parses the raw `models:` block of $ConfigPath into a
        role -> @{ Model; Effort; From } map, keeping the fields exactly as
        written (an unresolved `from:` phase name, not the model/effort it
        resolves to).

    .DESCRIPTION
        Scans the block with the same column-0/indented-line rules as
        tools/install.ps1's Resolve-NervModelAssignments, then cross-checks
        the result against that function's validated, resolved assignments
        (over $ConfigPath/$StatePath) so a role with an invalid model/effort
        value, or an unresolved `from:` phase, is dropped exactly the way
        `-ApplyModels` would drop it — keeping the two scanners consistent.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$ConfigPath,
        [string]$StatePath = (Join-Path (Get-NervHomeDir) ".gentle-ai/state.json")
    )

    $raw = @{}

    if (-not (Test-Path -LiteralPath $ConfigPath)) {
        return $raw
    }

    $text = Get-Content -LiteralPath $ConfigPath -Raw -Encoding UTF8
    if ($null -eq $text) { $text = '' }
    $lines = ($text -replace "`r`n", "`n") -split "`n"

    $blockLines = [System.Collections.Generic.List[string]]::new()
    $inBlock = $false
    foreach ($line in $lines) {
        if (-not $inBlock) {
            if ($line -match '^models:\s*(#.*)?$') { $inBlock = $true }
            continue
        }
        if ($line.Trim().Length -eq 0 -or $line -match '^\s*#') { continue }
        if ($line -match '^\s') { $blockLines.Add($line) }
        else { break }
    }

    foreach ($line in $blockLines) {
        $entryMatch = [regex]::Match($line, '^\s*(?<role>[A-Za-z0-9_-]+):\s*\{(?<body>[^}]*)\}\s*(#.*)?$')
        if (-not $entryMatch.Success) { continue }

        $role = $entryMatch.Groups['role'].Value
        $body = $entryMatch.Groups['body'].Value
        $entry = @{}

        foreach ($pair in ($body -split ',')) {
            if ($pair.Trim().Length -eq 0) { continue }
            $kv = $pair -split ':', 2
            if ($kv.Count -ne 2) { continue }
            $key = $kv[0].Trim()
            $value = $kv[1].Trim().Trim('"').Trim("'")
            switch ($key) {
                'from' { $entry['From'] = $value }
                'model' { $entry['Model'] = $value }
                'effort' { $entry['Effort'] = $value }
            }
        }

        if ($entry.Count -gt 0) { $raw[$role] = $entry }
    }

    $validated = Resolve-NervModelAssignments -ConfigPath $ConfigPath -StatePath $StatePath -WarningAction SilentlyContinue
    foreach ($role in @($raw.Keys)) {
        if (-not $validated.ContainsKey($role)) { $raw.Remove($role) }
    }

    return $raw
}

function Get-NervModelTable {
    <#
    .SYNOPSIS
        Builds the display table (role, model, effort, source) from the
        plugin defaults and the raw nerv.yaml overrides, resolving any
        `from:` phase against gentle-ai's state.json for display.

    .DESCRIPTION
        `source` is 'override' when the role has an explicit `model` or
        `effort` key in $Overrides, 'gentle-ai:<phase>' when it only has a
        `from:` key, or 'default' when it has no override at all. A role
        present in $Overrides but not $Defaults still appears (its model
        and/or effort may be $null if neither the override nor a resolved
        `from:` phase supplied one).
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][hashtable]$Defaults,
        [Parameter(Mandatory)][hashtable]$Overrides,
        $State
    )

    $allRoles = @(@($Defaults.Keys) + @($Overrides.Keys) | Select-Object -Unique | Sort-Object)
    $table = [System.Collections.Generic.List[object]]::new()

    foreach ($role in $allRoles) {
        $default = $Defaults[$role]
        $override = $Overrides[$role]

        $model = $null
        $effort = $null
        if ($default) {
            if ($default.ContainsKey('Model')) { $model = $default['Model'] }
            if ($default.ContainsKey('Effort')) { $effort = $default['Effort'] }
        }
        $source = 'default'

        if ($override) {
            if ($override.ContainsKey('From')) {
                $phase = $override['From']
                $source = "gentle-ai:$phase"
                $phaseEntry = $null
                if ($State -and $State.claude_phase_assignments -and $State.claude_phase_assignments.PSObject.Properties[$phase]) {
                    $phaseEntry = $State.claude_phase_assignments.$phase
                }
                if ($phaseEntry) {
                    if ($phaseEntry.PSObject.Properties['model']) { $model = [string]$phaseEntry.model }
                    if ($phaseEntry.PSObject.Properties['effort']) { $effort = [string]$phaseEntry.effort }
                }
            }
            if ($override.ContainsKey('Model')) {
                $model = $override['Model']
                $source = 'override'
            }
            if ($override.ContainsKey('Effort')) {
                $effort = $override['Effort']
                $source = 'override'
            }
        }

        $table.Add([pscustomobject]@{
            role   = $role
            model  = $model
            effort = $effort
            source = $source
        })
    }

    return $table.ToArray()
}

# =============================================================================
# Interactive body — skipped entirely when this file is dot-sourced (tests
# drive the pure functions above directly; the end-to-end scenario launches
# this file as a real child process instead).
# =============================================================================
if ($MyInvocation.InvocationName -ne '.') {

$ErrorActionPreference = "Stop"

$allRoleNames = @(
    'aoba', 'asuka', 'balthasar', 'casper', 'fuyutsuki', 'hyuga', 'kaji',
    'kaji-coverage', 'kaji-refuter', 'kaji-security', 'kaworu', 'maya',
    'melchor', 'misato', 'rei', 'ritsuko', 'shinji', 'toji'
)
$roleGroups = @{
    'magi'         = @('balthasar', 'melchor', 'casper')
    'pilots'       = @('rei', 'shinji', 'asuka', 'toji', 'kaworu')
    'kaji-passes'  = @('kaji', 'kaji-security', 'kaji-coverage', 'kaji-refuter')
    'all'          = $allRoleNames
}

if (-not $StatePath) {
    $StatePath = Join-Path (Get-NervHomeDir) ".gentle-ai/state.json"
}

if (-not $ConfigPath) {
    if ($Scope -eq 'user') {
        $ConfigPath = Join-Path (Get-NervHomeDir) ".claude/nerv/nerv.yaml"
    }
    else {
        $ConfigPath = Join-Path $ProjectDir ".nerv/nerv.yaml"
    }
}

if ($Scope -eq 'project' -and -not (Test-Path -LiteralPath $ConfigPath)) {
    Write-Host "Project config not found at $ConfigPath. Run /nerv:init first, or pass -ConfigPath explicitly."
    exit 1
}

# --- answers-file driven prompting (never Read-Host once -AnswersFile is set) ---
$script:answerLines = $null
$script:answerIndex = 0
if ($AnswersFile) {
    if (Test-Path -LiteralPath $AnswersFile) {
        $script:answerLines = @(Get-Content -LiteralPath $AnswersFile -Encoding UTF8)
    }
    else {
        $script:answerLines = @()
    }
}

function Read-NervAnswer {
    param([string]$Prompt, [string]$DefaultWhenExhausted = '')
    if ($null -ne $script:answerLines) {
        if ($script:answerIndex -lt $script:answerLines.Count) {
            $ans = $script:answerLines[$script:answerIndex]
            $script:answerIndex++
        }
        else {
            $ans = $DefaultWhenExhausted
        }
        Write-Host "$Prompt $ans"
        return $ans
    }
    return (Read-Host $Prompt)
}

function Read-NervChoice {
    param([string]$Prompt, [string[]]$ValidValues, [string]$DefaultWhenExhausted = '')
    while ($true) {
        $ans = (Read-NervAnswer -Prompt $Prompt -DefaultWhenExhausted $DefaultWhenExhausted).Trim()
        if ($ans -eq '') { return $ans }
        if ($ValidValues -contains $ans) { return $ans }
        Write-Host "Invalid input. Allowed: $($ValidValues -join ', '), or Enter to keep current."
    }
}

function Resolve-NervRoleTarget {
    param([string]$Target, [string[]]$AllRoles, [hashtable]$Groups, [hashtable]$NumberMap)
    $t = $Target.Trim()
    if ($t -eq '') { return $null }
    if ($Groups.ContainsKey($t.ToLower())) { return $Groups[$t.ToLower()] }
    if ($NumberMap.ContainsKey($t)) { return @($NumberMap[$t]) }
    if ($AllRoles -contains $t.ToLower()) { return @($t.ToLower()) }
    return $null
}

function Show-NervTable {
    param([object[]]$Table, [string]$Scope)
    Write-Host ""
    Write-Host "Scope: $Scope   Config: $ConfigPath"
    Write-Host ("{0,-16} {1,-16} {2,-8} {3}" -f 'ROLE', 'MODEL', 'EFFORT', 'SOURCE')
    for ($i = 0; $i -lt $Table.Count; $i++) {
        $row = $Table[$i]
        Write-Host ("{0,2}) {1,-16} {2,-16} {3,-8} {4}" -f ($i + 1), $row.role, $row.model, $row.effort, $row.source)
    }
    if ($Scope -eq 'project') {
        Write-Host ""
        Write-Host "Note: at project scope, 'effort' overrides only take effect after applying them at user scope with 'pwsh tools/install.ps1 -ApplyModels' — only 'model' is honored directly from a project-scope nerv.yaml."
    }
    Write-Host ""
}

$defaults = Get-NervPluginDefaults -AgentsDir (Join-Path $RepoPath "plugin/agents")

$existingYamlText = if (Test-Path -LiteralPath $ConfigPath) { Get-Content -LiteralPath $ConfigPath -Raw -Encoding UTF8 } else { '' }
if ($null -eq $existingYamlText) { $existingYamlText = '' }

$stateObj = $null
if (Test-Path -LiteralPath $StatePath) {
    try {
        $stateObj = Get-Content -LiteralPath $StatePath -Raw -Encoding UTF8 | ConvertFrom-Json -Depth 50
    }
    catch {
        Write-Warning "Could not parse $StatePath : $_"
        $stateObj = $null
    }
}

$overrides = Read-NervModelsOverrides -ConfigPath $ConfigPath -StatePath $StatePath

$modelMenu = @{
    '1' = 'sonnet'
    '2' = 'opus'
    '3' = 'haiku'
    '4' = 'fable'
    '5' = 'inherit'
}
$effortMenu = @{
    '1' = 'low'
    '2' = 'medium'
    '3' = 'high'
    '4' = 'xhigh'
    '5' = 'max'
}

$done = $false
while (-not $done) {
    $table = Get-NervModelTable -Defaults $defaults -Overrides $overrides -State $stateObj
    Show-NervTable -Table $table -Scope $Scope

    $numberMap = @{}
    for ($i = 0; $i -lt $table.Count; $i++) { $numberMap["$($i + 1)"] = $table[$i].role }

    $roleAnswer = (Read-NervAnswer -Prompt 'Role (name, number, magi | pilots | kaji-passes | all), "reset" to clear an override, "done" to finish:' -DefaultWhenExhausted 'done').Trim()

    if ($roleAnswer -eq '') { continue }
    if ($roleAnswer -ieq 'done') { $done = $true; continue }

    if ($roleAnswer -match '(?i)^reset\s*(?<t>.*)$') {
        $resetTarget = $Matches['t'].Trim()
        if ($resetTarget -eq '') {
            $resetTarget = (Read-NervAnswer -Prompt 'Reset which role or group?' -DefaultWhenExhausted 'done').Trim()
        }
        if ($resetTarget -eq '' -or $resetTarget -ieq 'done') { continue }

        $resetRoles = Resolve-NervRoleTarget -Target $resetTarget -AllRoles $allRoleNames -Groups $roleGroups -NumberMap $numberMap
        if (-not $resetRoles) {
            Write-Host "Unknown role/group/number: $resetTarget. Valid roles: $($allRoleNames -join ', '); groups: magi, pilots, kaji-passes, all."
            continue
        }
        foreach ($r in $resetRoles) { $overrides.Remove($r) }
        continue
    }

    $roles = Resolve-NervRoleTarget -Target $roleAnswer -AllRoles $allRoleNames -Groups $roleGroups -NumberMap $numberMap
    if (-not $roles) {
        Write-Host "Unknown role/group/number: $roleAnswer. Valid roles: $($allRoleNames -join ', '); groups: magi, pilots, kaji-passes, all."
        continue
    }

    $currentDisplay = if ($roles.Count -eq 1) {
        $row = $table | Where-Object { $_.role -eq $roles[0] }
        if ($row) { "$($row.model)/$($row.effort)" } else { '(none)' }
    }
    else {
        '(varies)'
    }

    $modelChoice = Read-NervChoice -Prompt "Model (1 sonnet, 2 opus, 3 haiku, 4 fable, 5 inherit, 6 custom id, 7 from gentle-ai phase, Enter keeps $currentDisplay):" -ValidValues @('1', '2', '3', '4', '5', '6', '7')

    $newModel = $null
    $newFrom = $null
    if ($modelChoice -ne '') {
        switch ($modelChoice) {
            '6' {
                while ($true) {
                    $customId = (Read-NervAnswer -Prompt 'Custom model id (claude-...):' -DefaultWhenExhausted '').Trim()
                    if ($customId -match '^claude-.+$') { $newModel = $customId; break }
                    Write-Host "Invalid model id; must match ^claude-.+$"
                }
            }
            '7' {
                $phases = @()
                if ($stateObj -and $stateObj.claude_phase_assignments) {
                    $phases = @($stateObj.claude_phase_assignments.PSObject.Properties.Name | Sort-Object)
                }
                if ($phases.Count -eq 0) {
                    Write-Host "No gentle-ai phases found in $StatePath."
                }
                else {
                    Write-Host "Phases:"
                    for ($i = 0; $i -lt $phases.Count; $i++) {
                        $p = $phases[$i]
                        $pe = $stateObj.claude_phase_assignments.$p
                        $pm = if ($pe.PSObject.Properties['model']) { $pe.model } else { '' }
                        $pf = if ($pe.PSObject.Properties['effort']) { $pe.effort } else { '' }
                        Write-Host "  $($i + 1)) $p ($pm/$pf)"
                    }
                    $phaseValid = @(1..$phases.Count | ForEach-Object { "$_" })
                    $phaseIdx = Read-NervChoice -Prompt 'Phase number:' -ValidValues $phaseValid
                    if ($phaseIdx -ne '') { $newFrom = $phases[[int]$phaseIdx - 1] }
                }
            }
            default { $newModel = $modelMenu[$modelChoice] }
        }
    }

    if ($modelChoice -eq '7' -and $newFrom) {
        $effortChoice = Read-NervChoice -Prompt "Effort (1 low, 2 medium, 3 high, 4 xhigh, 5 max, Enter keeps inherited):" -ValidValues @('1', '2', '3', '4', '5')
    }
    else {
        $effortChoice = Read-NervChoice -Prompt "Effort (1 low, 2 medium, 3 high, 4 xhigh, 5 max, Enter keeps $currentDisplay):" -ValidValues @('1', '2', '3', '4', '5')
    }
    $newEffort = if ($effortChoice -ne '') { $effortMenu[$effortChoice] } else { $null }

    foreach ($r in $roles) {
        $entry = if ($overrides.ContainsKey($r)) { $overrides[$r] } else { @{} }

        if ($modelChoice -ne '') {
            if ($modelChoice -eq '7') {
                if ($newFrom) {
                    $entry['From'] = $newFrom
                    $entry.Remove('Model')
                }
            }
            else {
                $entry['Model'] = $newModel
                $entry.Remove('From')
            }
        }

        if ($modelChoice -eq '7' -and $newFrom) {
            if ($newEffort) { $entry['Effort'] = $newEffort } else { $entry.Remove('Effort') }
        }
        elseif ($newEffort) {
            $entry['Effort'] = $newEffort
        }

        if ($entry.Count -eq 0) { $overrides.Remove($r) } else { $overrides[$r] = $entry }
    }
}

$blockText = if ($overrides.Count -gt 0) { Format-NervModelsBlock -Overrides $overrides } else { '' }
$hadExistingBlock = ($existingYamlText -match '(?m)^models:\s*(#.*)?$')

Write-Host ""
if ($blockText -ne '') {
    Write-Host "The following models: block will be written to ${ConfigPath}:"
    Write-Host ""
    Write-Host $blockText
    Write-Host ""
}
elseif ($hadExistingBlock) {
    Write-Host "No overrides left; the existing models: block will be removed from $ConfigPath."
}
else {
    Write-Host "No overrides configured; nothing to write."
}

if ($blockText -eq '' -and -not $hadExistingBlock) {
    Write-Host ""
    Write-Host "Restart Claude Code for the change to take effect."
}
else {
    $writeConfirm = (Read-NervAnswer -Prompt "Write to ${ConfigPath}? [Y/n]" -DefaultWhenExhausted 'Y').Trim()
    $writeYes = ($writeConfirm -eq '') -or ($writeConfirm -match '(?i)^y(es)?$')

    if (-not $writeYes) {
        Write-Host "Aborted; no changes written."
    }
    else {
        if (Test-Path -LiteralPath $ConfigPath) {
            $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
            $backupPath = "$ConfigPath.bak-models-$timestamp"
            Copy-Item -LiteralPath $ConfigPath -Destination $backupPath -Force
            Write-Host "Backup written: $backupPath"
        }
        else {
            $configDir = Split-Path -Parent $ConfigPath
            if ($configDir -and -not (Test-Path -LiteralPath $configDir)) {
                New-Item -ItemType Directory -Path $configDir -Force | Out-Null
            }
        }

        $newYamlText = Set-NervYamlModelsBlock -YamlText $existingYamlText -BlockText $blockText
        $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($ConfigPath, $newYamlText, $utf8NoBom)
        Write-Host "Written: $ConfigPath"

        if (-not $NoApply -and $Scope -eq 'user') {
            $applyConfirm = (Read-NervAnswer -Prompt "Apply to the plugin cache now with tools/install.ps1 -ApplyModels? [Y/n]" -DefaultWhenExhausted 'Y').Trim()
            $applyYes = ($applyConfirm -eq '') -or ($applyConfirm -match '(?i)^y(es)?$')
            if ($applyYes) {
                Invoke-NervApplyModels -RepoPath $RepoPath
            }
        }

        Write-Host ""
        Write-Host "Restart Claude Code for the change to take effect."
    }
}

} # end of `if ($MyInvocation.InvocationName -ne '.')` interactive-body guard
