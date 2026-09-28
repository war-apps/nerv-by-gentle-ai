#requires -Version 7
<#
.SYNOPSIS
    Interactive setup wizard for NERV: user-scope config, per-role
    model/effort, per-repo project config, and the Teamwork /task:* slash
    commands, in one run.

.DESCRIPTION
    Dot-sources tools/install.ps1 and tools/configure-models.ps1 to reuse
    their shared functions (Get-NervHomeDir, Invoke-NervApplyModels, the
    models block formatting/writing functions, etc.) — both scripts' own
    side-effecting bodies stay inert because they are guarded by
    `if ($MyInvocation.InvocationName -ne '.')`.

    Runs five sections in order:
      0. Prerequisites   — informational checks for gentle-ai/engram/claude.
      1. User config      — writes ~/.claude/nerv/nerv.yaml (git, tasks,
                             skills, critical_paths, artifacts).
      2. Models            — optionally launches tools/configure-models.ps1.
      3. Repos             — optionally writes <repo>/.nerv/nerv.yaml for one
                             or more repositories.
      4. Slash commands    — optionally copies the Teamwork procedures under
                             plugin/skills/nerv-tasks/providers/teamwork/procedures
                             into ~/.claude/commands/task/.
      5. Apply and refresh — optionally runs Invoke-NervApplyModels and
                             tools/install.ps1 -RefreshCache.

.PARAMETER RepoPath
    Path to the NERV repo root. Defaults to the grandparent of this
    script's directory (plugin\tools\..\..), i.e. the repo root when this
    script runs in place from the repo. When this script instead runs from
    the installed plugin cache, that grandparent is not the repo; this
    script only needs the real repo path for the interactive Section 5
    (apply + -RefreshCache), which delegates to install.ps1 without
    forwarding -RepoPath, so install.ps1's own cache-aware fallback
    resolves it there. Everything else (Sections 0-4) works from the cache
    as is. Pass -RepoPath explicitly to bypass this.

.PARAMETER ConfigPath
    Overrides the resolved user-scope nerv.yaml path (defaults to
    `<HomeDir>/.claude/nerv/nerv.yaml`).

.PARAMETER HomeDir
    Overrides the resolved home directory (defaults to Get-NervHomeDir).

.PARAMETER AnswersFile
    A text file with one answer per line, consumed instead of Read-Host —
    lets the wizard run non-interactively (tests, scripted setup). When the
    answers run out, every remaining prompt is treated as Enter
    (keep/default), and every [Y/n]-style confirmation uses its shown
    default.

.PARAMETER ModelsAnswersFile
    When section 2 (Models) is entered and confirmed, passed through as
    tools/configure-models.ps1's own -AnswersFile so that sub-wizard can
    also run non-interactively. Without it, confirming section 2 launches
    the models wizard interactively.

.PARAMETER SkipSkills
    Skip the required-skills step (tools/install-skills.ps1).

.PARAMETER SkipModels
    Skip section 2 (Models) entirely.

.PARAMETER SkipRepos
    Skip section 3 (Repos) entirely.

.PARAMETER SkipCommands
    Skip section 4 (Slash commands) entirely.

.PARAMETER NoRefresh
    Skip section 5 (Apply and refresh) entirely.

.EXAMPLE
    pwsh tools/configure.ps1

.EXAMPLE
    pwsh tools/configure.ps1 -SkipModels -NoRefresh -AnswersFile answers.txt
#>

[CmdletBinding()]
param(
    [string]$RepoPath = (Split-Path -Parent (Split-Path -Parent $PSScriptRoot)),

    [string]$ConfigPath,

    [string]$HomeDir,

    [string]$AnswersFile,

    [string]$ModelsAnswersFile,

    [switch]$SkipModels,

    [switch]$SkipRepos,

    [switch]$SkipCommands,
    [switch]$SkipSkills,

    [switch]$NoRefresh
)

# =============================================================================
# Shared functions from tools/install.ps1 and tools/configure-models.ps1.
# Dot-sourcing with bogus paths is safe regardless: both scripts' own main
# bodies are guarded by `if ($MyInvocation.InvocationName -ne '.')`, which is
# true whenever they are dot-sourced (as they are here), so only their
# functions get defined.
# =============================================================================
# Dot-sourcing a script re-runs its own `param()` block in THIS scope, which
# clobbers any of this wizard's own variables that share a name with one of
# its params — install.ps1 and configure-models.ps1 both declare -RepoPath,
# and configure-models.ps1 also declares -ConfigPath and -AnswersFile, same
# as this script. Snapshot this wizard's own values first and restore them
# right after, so the dot-sourced scripts' unrelated defaults (most notably
# -ConfigPath/-AnswersFile resetting to $null) never leak into this script's
# own param values.
$wizardRepoPath = $RepoPath
$wizardConfigPath = $ConfigPath
$wizardAnswersFile = $AnswersFile

$installScriptPath = Join-Path $PSScriptRoot 'install.ps1'
$installGuardPath = Join-Path ([System.IO.Path]::GetTempPath()) 'nerv-configure-install-guard-nonexistent.json'
. $installScriptPath -SettingsPath $installGuardPath -RepoPath $wizardRepoPath 2>$null

$modelsScriptPath = Join-Path $PSScriptRoot 'configure-models.ps1'
. $modelsScriptPath -RepoPath $wizardRepoPath -NoApply 2>$null

$RepoPath = $wizardRepoPath
$ConfigPath = $wizardConfigPath
$AnswersFile = $wizardAnswersFile

# =============================================================================
# Pure functions — dot-sourceable, no side effects of their own. Everything
# below this block up to the closing brace of the
# `if ($MyInvocation.InvocationName -ne '.')` guard is this script's
# interactive execution body.
# =============================================================================

function Set-NervYamlBlock {
    <#
    .SYNOPSIS
        Replaces, appends, or removes an arbitrary top-level `<Key>:` block
        inside a nerv.yaml text blob — a generalization of
        tools/configure-models.ps1's Set-NervYamlModelsBlock for any
        top-level key. When $Key is 'models', delegates to that existing
        function directly instead of duplicating its logic.

    .DESCRIPTION
        Locates an existing top-level `<Key>:` line (column 0; the value may
        be absent, a trailing comment, or a block-scalar indicator `|`/`>`)
        and every following blank, full-line-comment, or indented line, up
        to the next column-0 key, and replaces that whole run with
        $BlockText. When no `<Key>:` key exists, appends $BlockText after
        one blank line. When $BlockText is empty or $null, the existing
        block (if any) is removed instead. Everything outside the replaced
        region is preserved verbatim; the file's own EOL style (CRLF or LF,
        detected from $YamlText) is used to join the result.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][AllowEmptyString()][string]$YamlText,
        [Parameter(Mandatory)][string]$Key,
        [AllowEmptyString()][AllowNull()][string]$BlockText
    )

    if ($Key -eq 'models') {
        return Set-NervYamlModelsBlock -YamlText $YamlText -BlockText $BlockText
    }

    $eol = if ($YamlText -match "`r`n") { "`r`n" } else { "`n" }
    $lines = $YamlText -split "`r`n|`n"
    $keyPattern = "^$([regex]::Escape($Key)):\s*(\||>)?[+-]?\s*(#.*)?`$"

    $startIdx = -1
    $endIdx = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match $keyPattern) {
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

function Get-NervYamlBlock {
    <#
    .SYNOPSIS
        Returns the raw text of a top-level `<Key>:` block (the key line
        plus every following blank, full-line-comment, or indented line),
        or $null when the key is not present at column 0.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][AllowEmptyString()][string]$YamlText,
        [Parameter(Mandatory)][string]$Key
    )

    $lines = ($YamlText -replace "`r`n", "`n") -split "`n"
    $keyPattern = "^$([regex]::Escape($Key)):\s*(\||>)?[+-]?\s*(#.*)?`$"

    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match $keyPattern) {
            $j = $i + 1
            while ($j -lt $lines.Count) {
                $line = $lines[$j]
                if ($line.Trim().Length -eq 0) { $j++; continue }
                if ($line -match '^\s*#') { $j++; continue }
                if ($line -match '^\s') { $j++; continue }
                break
            }
            return (($lines[$i..($j - 1)]) -join "`n")
        }
    }
    return $null
}

function Get-NervYamlSubBlock {
    <#
    .SYNOPSIS
        Returns the raw text of an indented `<Key>:` sub-block inside
        $BlockText (as returned by Get-NervYamlBlock) — the key line plus
        every following blank, full-line-comment, or more-deeply-indented
        line — or $null when the key is not present at exactly $Indent
        spaces.
    #>
    [CmdletBinding()]
    param(
        [AllowEmptyString()][AllowNull()][string]$BlockText,
        [Parameter(Mandatory)][string]$Key,
        [int]$Indent = 2
    )

    if (-not $BlockText) { return $null }

    $lines = ($BlockText -replace "`r`n", "`n") -split "`n"
    $prefix = ' ' * $Indent
    $keyPattern = "^$prefix$([regex]::Escape($Key)):\s*(\||>)?[+-]?\s*(#.*)?`$"
    $deeperPattern = "^\s{$($Indent + 1),}"

    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match $keyPattern) {
            $j = $i + 1
            while ($j -lt $lines.Count) {
                $line = $lines[$j]
                if ($line.Trim().Length -eq 0) { $j++; continue }
                if ($line -match '^\s*#') { $j++; continue }
                if ($line -match $deeperPattern) { $j++; continue }
                break
            }
            return (($lines[$i..($j - 1)]) -join "`n")
        }
    }
    return $null
}

function Test-NervYamlKeyExists {
    <#
    .SYNOPSIS
        Returns whether a top-level `<Key>:` line exists at all, in ANY
        form: a bare block header (`git:`), an inline map/list
        (`skills: { ... }`, `critical_paths: [ ... ]`), or a plain bare
        scalar (`enabled: true`). Get-NervYamlBlock only recognizes a bare
        block header (or a block-scalar `|`/`>` header) as "existing" —
        this function is the general existence check section 1 uses before
        deciding whether a key needs to be created fresh at all.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][AllowEmptyString()][string]$YamlText,
        [Parameter(Mandatory)][string]$Key
    )

    $lines = ($YamlText -replace "`r`n", "`n") -split "`n"
    $pattern = "^$([regex]::Escape($Key)):"
    foreach ($line in $lines) {
        if ($line -match $pattern) { return $true }
    }
    return $false
}

function Test-NervYamlKeyIsInline {
    <#
    .SYNOPSIS
        Returns whether an EXISTING top-level `<Key>:` line carries its
        value inline on that same line (`skills: { ... }`,
        `critical_paths: [ ... ]`, a bare scalar) rather than being a bare
        block header (`git:`) or a block-scalar header (`sources_howto:
        |`). Returns $false when the key does not exist at all — callers
        must check Test-NervYamlKeyExists separately.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][AllowEmptyString()][string]$YamlText,
        [Parameter(Mandatory)][string]$Key
    )

    $lines = ($YamlText -replace "`r`n", "`n") -split "`n"
    $anyPattern = "^$([regex]::Escape($Key)):"
    $blockHeaderPattern = "^$([regex]::Escape($Key)):\s*(\||>)?[+-]?\s*(#.*)?`$"
    foreach ($line in $lines) {
        if ($line -match $anyPattern) {
            return -not ($line -match $blockHeaderPattern)
        }
    }
    return $false
}

function Get-NervYamlInlineListField {
    <#
    .SYNOPSIS
        Extracts one `<Key>: [ ... ]` entry's raw list content out of an
        inline-map text (e.g. the value of an inline `skills: { ... }`
        line), or $null when that key is not present in the map.
    #>
    [CmdletBinding()]
    param(
        [AllowNull()][AllowEmptyString()][string]$InlineText,
        [Parameter(Mandatory)][string]$Key
    )

    if (-not $InlineText) { return $null }
    $m = [regex]::Match($InlineText, "$([regex]::Escape($Key)):\s*\[([^\]]*)\]")
    if ($m.Success) { return $m.Groups[1].Value.Trim() }
    return $null
}

function Set-NervYamlInlineListField {
    <#
    .SYNOPSIS
        Replaces (or, if absent, appends) one `<Key>: [ ... ]` entry inside
        an inline-map text, leaving every other entry's raw text untouched.
        Used to update a single `skills:` category in place when `skills:`
        itself is written as an inline map, without reformatting the
        categories the caller did not change.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$InlineText,
        [Parameter(Mandatory)][string]$Key,
        [Parameter(Mandatory)][AllowEmptyString()][string]$NewValue
    )

    $pattern = "$([regex]::Escape($Key)):\s*\[[^\]]*\]"
    if ($InlineText -match $pattern) {
        return [regex]::Replace($InlineText, $pattern, "$Key`: [$NewValue]", 1)
    }

    $trimmed = $InlineText.TrimEnd()
    if ($trimmed.EndsWith('}')) {
        $inner = $trimmed.Substring(0, $trimmed.Length - 1).TrimEnd()
        $sep = if ($inner.EndsWith('{')) { '' } else { ', ' }
        return "$inner$sep$Key`: [$NewValue] }"
    }
    return $InlineText
}

function Read-NervScalar {
    <#
    .SYNOPSIS
        Simple dotted-path scalar reader, e.g. 'git.base_branch' or
        'tasks.providers.teamwork.assignee_id', for 1-3 levels of `key:`
        nesting below a top-level key. Returns the trailing scalar text
        (bare or quoted, trailing comment stripped) or $null when any
        segment of the path is not found.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][AllowEmptyString()][string]$YamlText,
        [Parameter(Mandatory)][string]$Path
    )

    $segments = $Path -split '\.'
    $lines = ($YamlText -replace "`r`n", "`n") -split "`n"

    $searchStart = 0
    $searchEnd = $lines.Count - 1
    $indent = 0

    for ($segIndex = 0; $segIndex -lt $segments.Count; $segIndex++) {
        $seg = $segments[$segIndex]
        $isLast = ($segIndex -eq $segments.Count - 1)
        $prefix = ' ' * $indent
        $keyPattern = "^$prefix$([regex]::Escape($seg)):(\s*)(.*)`$"

        $found = -1
        $foundEnd = $searchEnd
        for ($i = $searchStart; $i -le $searchEnd; $i++) {
            if ($lines[$i] -match $keyPattern) {
                $found = $i
                $j = $i + 1
                while ($j -le $searchEnd) {
                    $l = $lines[$j]
                    if ($l.Trim().Length -eq 0 -or $l -match '^\s*#') { $j++; continue }
                    $lineIndent = ([regex]::Match($l, '^\s*')).Value.Length
                    if ($lineIndent -gt $indent) { $j++; continue }
                    break
                }
                $foundEnd = $j - 1
                break
            }
        }

        if ($found -lt 0) { return $null }

        if ($isLast) {
            $valuePart = [regex]::Match($lines[$found], $keyPattern).Groups[2].Value
            $valuePart = $valuePart -replace '\s+#.*$', ''
            $valuePart = $valuePart.Trim()
            if ($valuePart -eq '') { return $null }
            $valuePart = $valuePart.Trim('"').Trim("'")
            return $valuePart
        }

        $searchStart = $found + 1
        $searchEnd = $foundEnd
        $indent += 2
    }

    return $null
}

function Format-NervYamlScalarToken {
    <#
    .SYNOPSIS
        Formats a scalar value for Set-NervYamlScalar: `null`/`true`/`false`
        and bare integers stay unquoted; a value made only of safe
        bare-scalar characters (letters, digits, `_ - . / ~`) stays
        unquoted; anything else (spaces, braces, parens, colons, etc.) is
        double-quoted, escaping any embedded double quote.
    #>
    [CmdletBinding()]
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Value)

    if ($Value -eq '') { return '""' }
    if ($Value -in @('null', 'true', 'false')) { return $Value }
    if ($Value -match '^-?\d+$') { return $Value }
    if ($Value -match '^[A-Za-z0-9_.\-/~]+$') { return $Value }
    $escaped = $Value -replace '"', '\"'
    return "`"$escaped`""
}

function Set-NervYamlScalar {
    <#
    .SYNOPSIS
        Updates one existing scalar (or pre-formatted list/inline-map)
        line inside a nerv.yaml text blob in place, or appends it as a new
        line when missing — without touching any other content in the
        file. This is the targeted alternative to Set-NervYamlBlock: it
        never regenerates a whole block, so unknown/unmanaged content next
        to the field it touches (extra keys, comments, nested lists the
        wizard doesn't know about) is always preserved byte for byte.

    .DESCRIPTION
        $Path walks a dotted key chain (e.g. 'git.base_branch' or
        'tasks.providers.teamwork.task_ref_prefix') through nested
        block-style YAML (2-space indent per level).

        When the leaf key already exists, only its value token is
        replaced — same indentation, same trailing `# comment` if any,
        left completely untouched; -Comment is ignored in this case, since
        an existing line's comment is always preserved as is.

        When the leaf key is missing, `  <key>: <value>` is appended as
        the last line of its parent block (right before that block's next
        sibling key / dedent), with `  # <Comment>` appended when -Comment
        is given. Any missing intermediate parent keys along the path are
        created the same way, in order, with the right indentation, before
        the leaf itself is appended into the newly created (empty) block.

        $Value is quoted unless it is null/true/false, a bare integer, or
        made only of safe bare-scalar characters — e.g.
        `feature/{prefix}-{id}-{slug}` gets quoted, `develop` does not.
        Pass -Raw to use $Value verbatim, unquoted — for a pre-formatted
        list (`[a, b]`) or inline-map (`{ a: 1, b: 2 }`) literal, such as
        the `skills:` category lists, `critical_paths:`, or
        `providers.teamwork.stages:`.

        Returns the text with the original EOL style (CRLF or LF, detected
        from $YamlText) preserved, and exactly one trailing EOL.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][AllowEmptyString()][string]$YamlText,
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][AllowEmptyString()][string]$Value,
        [string]$Comment,
        [switch]$Raw
    )

    $eol = if ($YamlText -match "`r`n") { "`r`n" } else { "`n" }
    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.AddRange([string[]]($YamlText -split "`r`n|`n"))
    # A trailing EOL in $YamlText produces one trailing empty split element;
    # drop it here and always re-add exactly one trailing EOL at the end, so
    # insertion indices are computed against real content lines only.
    if ($lines.Count -gt 1 -and $lines[$lines.Count - 1] -eq '') {
        $lines.RemoveAt($lines.Count - 1)
    }

    $formattedValue = if ($Raw) { $Value } else { Format-NervYamlScalarToken -Value $Value }
    $segments = $Path -split '\.'

    $searchStart = 0
    $searchEnd = $lines.Count
    $indent = 0

    for ($segIndex = 0; $segIndex -lt $segments.Count; $segIndex++) {
        $seg = $segments[$segIndex]
        $isLast = ($segIndex -eq $segments.Count - 1)
        $prefix = ' ' * $indent
        $keyLinePattern = "^$prefix$([regex]::Escape($seg)):(.*)`$"

        $found = -1
        for ($i = $searchStart; $i -lt $searchEnd; $i++) {
            if ($lines[$i] -match $keyLinePattern) { $found = $i; break }
        }

        if ($isLast) {
            if ($found -ge 0) {
                $rest = [regex]::Match($lines[$found], $keyLinePattern).Groups[1].Value
                $hashIdx = $rest.IndexOf('#')
                if ($hashIdx -ge 0) {
                    $beforeHash = $rest.Substring(0, $hashIdx)
                    $commentPart = $rest.Substring($hashIdx)
                    $leadingWs = [regex]::Match($beforeHash, '^\s*').Value
                    $trailingWs = [regex]::Match($beforeHash, '\s*$').Value
                    $newRest = "$leadingWs$formattedValue$trailingWs$commentPart"
                }
                else {
                    $leadingWs = [regex]::Match($rest, '^\s*').Value
                    if ($leadingWs -eq '') { $leadingWs = ' ' }
                    $newRest = "$leadingWs$formattedValue"
                }
                $lines[$found] = "$prefix${seg}:$newRest"
            }
            else {
                $newLine = "$prefix${seg}: $formattedValue"
                if ($Comment) { $newLine = "$newLine  # $Comment" }
                $lines.Insert($searchEnd, $newLine)
            }
            break
        }
        else {
            if ($found -ge 0) {
                $childStart = $found + 1
                $j = $childStart
                while ($j -lt $searchEnd) {
                    $l = $lines[$j]
                    if ($l.Trim().Length -eq 0 -or $l -match '^\s*#') { $j++; continue }
                    $lineIndent = ([regex]::Match($l, '^\s*')).Value.Length
                    if ($lineIndent -gt $indent) { $j++; continue }
                    break
                }
                $searchStart = $childStart
                $searchEnd = $j
                $indent += 2
            }
            else {
                $lines.Insert($searchEnd, "$prefix${seg}:")
                $searchStart = $searchEnd + 1
                $searchEnd = $searchEnd + 1
                $indent += 2
            }
        }
    }

    $joined = ($lines -join $eol)
    if (-not $joined.EndsWith($eol)) { $joined += $eol }
    return $joined
}

function Format-NervGitBlock {
    <#
    .SYNOPSIS
        Renders the `git:` block in the documented syntax from $Values
        (base_branch, worktree, branch_pattern, commit_ref_pattern).
    #>
    [CmdletBinding()]
    param([Parameter(Mandatory)][hashtable]$Values)

    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.Add('git:')
    $lines.Add("  base_branch: $($Values['base_branch'])              # default base for the worktree offer")
    $lines.Add("  worktree: $($Values['worktree'])                     # ask | always | never")
    $lines.Add("  branch_pattern: `"$($Values['branch_pattern'])`"   # prefix comes from the provider (tw, gh, jira)")
    $lines.Add("  commit_ref_pattern: `"$($Values['commit_ref_pattern'])`"")
    return ($lines -join "`n")
}

function Format-NervTasksBlock {
    <#
    .SYNOPSIS
        Renders the `tasks:` block in the documented syntax from $Values
        (provider, ask_when_missing, subtasks_per_wave, timer_store,
        rounding_minutes, and the teamwork_* keys), re-emitting
        providers.teamwork from $Values and keeping providers.github-projects
        / providers.jira as the documented inline-map placeholders. When
        $SourcesRaw is given, its raw `sources:` sub-block text (as returned
        by Get-NervYamlSubBlock) is appended verbatim.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][hashtable]$Values,
        [AllowNull()][AllowEmptyString()][string]$SourcesRaw
    )

    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.Add('tasks:')
    $lines.Add("  provider: $($Values['provider'])                # teamwork | github-projects | jira | none ; `"ask`" when absent")
    $lines.Add("  ask_when_missing: $($Values['ask_when_missing'])            # preflight asks task + worktree + branch if no active task")
    $lines.Add("  subtasks_per_wave: $($Values['subtasks_per_wave'])")
    $lines.Add("  timer_store: $($Values['timer_store'])")
    $lines.Add("  rounding_minutes: $($Values['rounding_minutes'])")
    $lines.Add('  providers:                        # one block per provider, only the enabled one is required')
    $lines.Add('    teamwork:')
    $lines.Add("      task_ref_prefix: $($Values['teamwork_task_ref_prefix'])           # {prefix} in branch_pattern / commit_ref_pattern")
    $lines.Add("      assignee_id: $($Values['teamwork_assignee_id'])           # user scope")
    $lines.Add("      default_project_id: $($Values['teamwork_default_project_id'])")
    $lines.Add("      default_tasklist_id: $($Values['teamwork_default_tasklist_id'])")
    $stages = "inDev: $($Values['teamwork_stage_inDev']), testing: $($Values['teamwork_stage_testing']), implemented: $($Values['teamwork_stage_implemented']), blocked: $($Values['teamwork_stage_blocked']), canceled: $($Values['teamwork_stage_canceled']), pending: $($Values['teamwork_stage_pending']), analysis: $($Values['teamwork_stage_analysis'])"
    $lines.Add("      stages: { $stages }")
    $lines.Add('    github-projects: { task_ref_prefix: gh, owner: "", project_number: 0 }    # later')
    $lines.Add('    jira: { task_ref_prefix: jira, site: "", project_key: "" }               # later')

    if ($SourcesRaw) {
        foreach ($l in ($SourcesRaw -split "`r`n|`n")) { $lines.Add($l) }
    }

    return ($lines -join "`n")
}

function Format-NervSkillsBlock {
    <#
    .SYNOPSIS
        Renders the `skills:` block in the documented syntax from $Values
        (comma-separated lists per category: testing, code, best-practices,
        architecture, audit).
    #>
    [CmdletBinding()]
    param([Parameter(Mandatory)][hashtable]$Values)

    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.Add('skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md')
    $lines.Add("  testing: [$($Values['testing'])]                        # ritsuko, kaworu, maya")
    $lines.Add("  code: [$($Values['code'])]         # pilots")
    $lines.Add("  best-practices: [$($Values['best-practices'])]  # balthasar")
    $lines.Add("  architecture: [$($Values['architecture'])]          # melchor")
    $lines.Add("  audit: [$($Values['audit'])]                       # kaji passes")
    return ($lines -join "`n")
}

function Format-NervCriticalPathsLine {
    <#
    .SYNOPSIS
        Renders the single-line `critical_paths:` entry in the documented
        syntax from a list of paths.
    #>
    [CmdletBinding()]
    param([string[]]$Paths)

    $joined = (@($Paths) -join ', ')
    return "critical_paths: [$joined]            # Hyuga auto-critical"
}

function Format-NervArtifactsBlock {
    <#
    .SYNOPSIS
        Renders the `artifacts:` block in the documented syntax from
        $Commit (with-change | at-close | never).
    #>
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$Commit)

    return "artifacts:`n  commit: $Commit                  # with-change | at-close | never (default: at-close)"
}

function Format-NervProjectFile {
    <#
    .SYNOPSIS
        Renders a project-scope `.nerv/nerv.yaml` text: `enabled: true`,
        then only the keys the caller chose to override
        (git.base_branch, tasks.provider,
        tasks.providers.teamwork.project_id/tasklist_id, artifacts.commit),
        each with a short comment, plus a commented `models:` hint line.
        $Values keys: base_branch, provider, project_id, tasklist_id,
        commit — all optional; absent or empty keys are omitted.
    #>
    [CmdletBinding()]
    param([Parameter(Mandatory)][hashtable]$Values)

    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.Add('enabled: true')

    if ($Values.ContainsKey('base_branch') -and $Values['base_branch']) {
        $lines.Add('')
        $lines.Add('git:')
        $lines.Add("  base_branch: $($Values['base_branch'])              # overrides the user-scope default for this repo")
    }

    $hasProvider = $Values.ContainsKey('provider') -and $Values['provider']
    $hasProjectId = $Values.ContainsKey('project_id') -and $Values['project_id']
    $hasTasklistId = $Values.ContainsKey('tasklist_id') -and $Values['tasklist_id']
    if ($hasProvider -or $hasProjectId -or $hasTasklistId) {
        $lines.Add('')
        $lines.Add('tasks:')
        if ($hasProvider) {
            $lines.Add("  provider: $($Values['provider'])                # overrides the user-scope default for this repo")
        }
        if ($hasProjectId -or $hasTasklistId) {
            $lines.Add('  providers:')
            $lines.Add('    teamwork:')
            if ($hasProjectId) { $lines.Add("      project_id: $($Values['project_id'])                # this repo's Teamwork project") }
            if ($hasTasklistId) { $lines.Add("      tasklist_id: $($Values['tasklist_id'])                # this repo's Teamwork tasklist") }
        }
    }

    if ($Values.ContainsKey('commit') -and $Values['commit']) {
        $lines.Add('')
        $lines.Add('artifacts:')
        $lines.Add("  commit: $($Values['commit'])                  # with-change | at-close | never")
    }

    $lines.Add('')
    $lines.Add('# models:                           # optional: per-role model/effort overrides for this repo (see tools/configure-models.ps1 -Scope project)')

    return (($lines -join "`n") + "`n")
}

# =============================================================================
# Interactive body — skipped entirely when this file is dot-sourced (tests
# drive the pure functions above directly; the end-to-end scenario launches
# this file as a real child process instead).
# =============================================================================
if ($MyInvocation.InvocationName -ne '.') {

$ErrorActionPreference = "Stop"

Write-Host "=== NERV Setup Wizard ===" -ForegroundColor Cyan
Write-Host ""

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

function Read-NervConfigureAnswer {
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

function Get-NervAnswerOrDefault {
    param([string]$Prompt, [string]$Current)
    $ans = (Read-NervConfigureAnswer -Prompt "$Prompt [$Current]:" -DefaultWhenExhausted '').Trim()
    if ($ans -eq '') { return $Current }
    return $ans
}

function Get-NervChoiceOrDefault {
    param([string]$Prompt, [string[]]$ValidValues, [string]$Current)
    while ($true) {
        $ans = (Read-NervConfigureAnswer -Prompt "$Prompt ($($ValidValues -join '|')) [$Current]:" -DefaultWhenExhausted '').Trim()
        if ($ans -eq '') { return $Current }
        if ($ValidValues -contains $ans) { return $ans }
        Write-Host "Invalid input. Allowed: $($ValidValues -join ', ')."
    }
}

function Get-NervListDefaultFromScalar {
    param([string]$RawValue)
    if (-not $RawValue) { return '' }
    $v = $RawValue.Trim()
    if ($v.StartsWith('[') -and $v.EndsWith(']')) { $v = $v.Substring(1, $v.Length - 2) }
    return $v.Trim()
}

function Get-NervStageDefault {
    param([string]$StagesRaw, [string]$StageKey, [string]$Fallback)
    if (-not $StagesRaw) { return $Fallback }
    $m = [regex]::Match($StagesRaw, "$([regex]::Escape($StageKey)):\s*([^,}]+)")
    if ($m.Success) { return $m.Groups[1].Value.Trim() }
    return $Fallback
}

function Get-NervSkillsCategoryDefault {
    <#
    .SYNOPSIS
        Reads one skills category's current comma-separated list, whether
        `skills:` is written as a block (each category on its own indented
        line) or as an inline map (`skills: { testing: [...], ... }`) —
        Read-NervScalar's dotted-path walker cannot see into an inline
        map's fields, so this checks the inline form first and only falls
        back to the block-style nested lookup when `skills:` is not
        inline. Falls back to $Fallback when the category is not found in
        either form (or `skills:` does not exist at all).
    #>
    param([string]$YamlText, [string]$Category, [string]$Fallback)

    if (Test-NervYamlKeyExists -YamlText $YamlText -Key 'skills') {
        if (Test-NervYamlKeyIsInline -YamlText $YamlText -Key 'skills') {
            $inlineRaw = Read-NervScalar -YamlText $YamlText -Path 'skills'
            $val = Get-NervYamlInlineListField -InlineText $inlineRaw -Key $Category
            if ($val) { return $val }
        }
        else {
            $val = Get-NervListDefaultFromScalar (Read-NervScalar -YamlText $YamlText -Path "skills.$Category")
            if ($val) { return $val }
        }
    }
    return $Fallback
}

# --- Section 0: Prerequisites ---
Write-Host "--- Prerequisites ---"

$gentleAiVersionOutput = $null
try { $gentleAiVersionOutput = (gentle-ai --version 2>&1 | Select-Object -First 1) -as [string] } catch { $gentleAiVersionOutput = $null }
if ([string]::IsNullOrWhiteSpace($gentleAiVersionOutput)) {
    Write-Host "gentle-ai      : NOT FOUND on PATH (NERV requires gentle-ai 3.x)"
}
else {
    $versionMatch = [regex]::Match($gentleAiVersionOutput, '(\d+)\.(\d+)\.(\d+)')
    if ($versionMatch.Success -and [int]$versionMatch.Groups[1].Value -eq 3) {
        Write-Host "gentle-ai      : $($versionMatch.Value) (OK)"
    }
    else {
        Write-Host "gentle-ai      : $gentleAiVersionOutput (WARNING: NERV requires major version 3)"
    }
}

$engramCmd = Get-Command engram -ErrorAction SilentlyContinue
Write-Host "engram         : $(if ($engramCmd) { 'found on PATH (optional)' } else { 'not found on PATH (optional)' })"

$claudeCmd = Get-Command claude -ErrorAction SilentlyContinue
Write-Host "claude         : $(if ($claudeCmd) { 'found on PATH' } else { 'NOT FOUND on PATH (required for the refresh step)' })"

# --- Section 0b: Required skills ---
if (-not $SkipSkills) {
    $installSkillsPath = Join-Path $PSScriptRoot "install-skills.ps1"
    if (Test-Path -LiteralPath $installSkillsPath) {
        Write-Host ""
        Write-Host "--- Required skills ---"
        $skillsAnswer = Read-NervConfigureAnswer "Install the skills the plugin references (missing ones only, via npx skills add -g)? [Y/n]"
        if ([string]::IsNullOrWhiteSpace($skillsAnswer) -or $skillsAnswer.Trim() -match '^[Yy]') {
            $global:LASTEXITCODE = 0
            & pwsh -NoProfile -File $installSkillsPath
            if ($LASTEXITCODE -ne 0) { Write-Warning "install-skills.ps1 reported failures; see the lines above." }
        }
        else {
            Write-Host "Skipped. Run pwsh tools/install-skills.ps1 later (add -DryRun to preview)."
        }
    }
    else {
        Write-Warning "install-skills.ps1 not found next to this script; skills not verified."
    }
}

# --- Section 1: User config ---
$homeDirResolved = if ($HomeDir) { $HomeDir } else { Get-NervHomeDir }
$userConfigPath = if ($ConfigPath) { $ConfigPath } else { Join-Path $homeDirResolved ".claude/nerv/nerv.yaml" }

Write-Host ""
Write-Host "--- User config ---"
$userConfigExists = Test-Path -LiteralPath $userConfigPath
Write-Host "Config path    : $userConfigPath $(if ($userConfigExists) { '(exists)' } else { '(will be created)' })"

$existingUserYaml = if ($userConfigExists) { Get-Content -LiteralPath $userConfigPath -Raw -Encoding UTF8 } else { '' }
if ($null -eq $existingUserYaml) { $existingUserYaml = '' }

$defaultBaseBranch = Read-NervScalar -YamlText $existingUserYaml -Path 'git.base_branch'
if (-not $defaultBaseBranch) { $defaultBaseBranch = 'develop' }
$defaultWorktree = Read-NervScalar -YamlText $existingUserYaml -Path 'git.worktree'
if (-not $defaultWorktree) { $defaultWorktree = 'ask' }
$defaultBranchPattern = Read-NervScalar -YamlText $existingUserYaml -Path 'git.branch_pattern'
if (-not $defaultBranchPattern) { $defaultBranchPattern = 'feature/{prefix}-{id}-{slug}' }
$defaultCommitRefPattern = Read-NervScalar -YamlText $existingUserYaml -Path 'git.commit_ref_pattern'
if (-not $defaultCommitRefPattern) { $defaultCommitRefPattern = '({PREFIX}-{id})' }

$defaultProvider = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.provider'
if (-not $defaultProvider) { $defaultProvider = 'teamwork' }
$defaultAskWhenMissing = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.ask_when_missing'
if (-not $defaultAskWhenMissing) { $defaultAskWhenMissing = 'true' }
$defaultSubtasksPerWave = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.subtasks_per_wave'
if (-not $defaultSubtasksPerWave) { $defaultSubtasksPerWave = 'false' }
$defaultTimerStore = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.timer_store'
if (-not $defaultTimerStore) { $defaultTimerStore = '~/.claude/work/timers.json' }
$defaultRoundingMinutes = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.rounding_minutes'
if (-not $defaultRoundingMinutes) { $defaultRoundingMinutes = '15' }

$defaultTaskRefPrefix = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.providers.teamwork.task_ref_prefix'
if (-not $defaultTaskRefPrefix) { $defaultTaskRefPrefix = 'tw' }
$defaultAssigneeId = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.providers.teamwork.assignee_id'
if (-not $defaultAssigneeId) { $defaultAssigneeId = '' }
$defaultDefaultProjectId = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.providers.teamwork.default_project_id'
if (-not $defaultDefaultProjectId) { $defaultDefaultProjectId = '' }
$defaultDefaultTasklistId = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.providers.teamwork.default_tasklist_id'
if (-not $defaultDefaultTasklistId) { $defaultDefaultTasklistId = '' }

$stagesRaw = Read-NervScalar -YamlText $existingUserYaml -Path 'tasks.providers.teamwork.stages'
$defaultStageInDev = Get-NervStageDefault -StagesRaw $stagesRaw -StageKey 'inDev' -Fallback 'DESARROLLO'
$defaultStageTesting = Get-NervStageDefault -StagesRaw $stagesRaw -StageKey 'testing' -Fallback 'TESTING'
$defaultStageImplemented = Get-NervStageDefault -StagesRaw $stagesRaw -StageKey 'implemented' -Fallback 'IMPLEMENTA'
$defaultStageBlocked = Get-NervStageDefault -StagesRaw $stagesRaw -StageKey 'blocked' -Fallback 'BLOQUEA'
$defaultStageCanceled = Get-NervStageDefault -StagesRaw $stagesRaw -StageKey 'canceled' -Fallback 'CANCEL'
$defaultStagePending = Get-NervStageDefault -StagesRaw $stagesRaw -StageKey 'pending' -Fallback 'PENDIENTE'
$defaultStageAnalysis = Get-NervStageDefault -StagesRaw $stagesRaw -StageKey 'analysis' -Fallback 'ANALISIS'

$defaultSkillsTesting = Get-NervSkillsCategoryDefault -YamlText $existingUserYaml -Category 'testing' -Fallback 'tdd, playwright-best-practices'
$defaultSkillsCode = Get-NervSkillsCategoryDefault -YamlText $existingUserYaml -Category 'code' -Fallback 'dotnet-best-practices, typescript-best-practices'
$defaultSkillsBestPractices = Get-NervSkillsCategoryDefault -YamlText $existingUserYaml -Category 'best-practices' -Fallback 'best-practices, solid-principles, clean-code-guard'
$defaultSkillsArchitecture = Get-NervSkillsCategoryDefault -YamlText $existingUserYaml -Category 'architecture' -Fallback 'hexagonal-architecture, c4-architecture'
$defaultSkillsAudit = Get-NervSkillsCategoryDefault -YamlText $existingUserYaml -Category 'audit' -Fallback 'security-review, clean-code-guard'

$defaultCriticalPaths = Get-NervListDefaultFromScalar (Read-NervScalar -YamlText $existingUserYaml -Path 'critical_paths')
if (-not $defaultCriticalPaths) { $defaultCriticalPaths = 'auth/, payments/, migrations/, infra/' }

$defaultArtifactsCommit = Read-NervScalar -YamlText $existingUserYaml -Path 'artifacts.commit'
if (-not $defaultArtifactsCommit) { $defaultArtifactsCommit = 'at-close' }

Write-Host ""
Write-Host "-- git --"
$gitBaseBranch = Get-NervAnswerOrDefault -Prompt 'Base branch' -Current $defaultBaseBranch
$gitWorktree = Get-NervChoiceOrDefault -Prompt 'Worktree policy' -ValidValues @('ask', 'always', 'never') -Current $defaultWorktree
$gitBranchPattern = Get-NervAnswerOrDefault -Prompt 'Branch pattern' -Current $defaultBranchPattern
$gitCommitRefPattern = Get-NervAnswerOrDefault -Prompt 'Commit ref pattern' -Current $defaultCommitRefPattern

Write-Host ""
Write-Host "-- tasks --"
$tasksProvider = Get-NervChoiceOrDefault -Prompt 'Task provider' -ValidValues @('teamwork', 'github-projects', 'jira', 'none') -Current $defaultProvider
$tasksAskWhenMissing = Get-NervChoiceOrDefault -Prompt 'Ask when missing' -ValidValues @('true', 'false') -Current $defaultAskWhenMissing
$tasksSubtasksPerWave = Get-NervChoiceOrDefault -Prompt 'Subtasks per wave' -ValidValues @('true', 'false') -Current $defaultSubtasksPerWave
$tasksTimerStore = Get-NervAnswerOrDefault -Prompt 'Timer store path' -Current $defaultTimerStore
$tasksRoundingMinutes = Get-NervAnswerOrDefault -Prompt 'Rounding minutes' -Current $defaultRoundingMinutes

$teamworkTaskRefPrefix = $defaultTaskRefPrefix
$teamworkAssigneeId = $defaultAssigneeId
$teamworkDefaultProjectId = $defaultDefaultProjectId
$teamworkDefaultTasklistId = $defaultDefaultTasklistId
$stageInDev = $defaultStageInDev
$stageTesting = $defaultStageTesting
$stageImplemented = $defaultStageImplemented
$stageBlocked = $defaultStageBlocked
$stageCanceled = $defaultStageCanceled
$stagePending = $defaultStagePending
$stageAnalysis = $defaultStageAnalysis

if ($tasksProvider -eq 'teamwork') {
    Write-Host ""
    Write-Host "-- tasks.providers.teamwork --"
    $teamworkTaskRefPrefix = Get-NervAnswerOrDefault -Prompt 'Task ref prefix' -Current $defaultTaskRefPrefix
    $teamworkAssigneeId = Get-NervAnswerOrDefault -Prompt 'Assignee id' -Current $defaultAssigneeId
    $teamworkDefaultProjectId = Get-NervAnswerOrDefault -Prompt 'Default project id' -Current $defaultDefaultProjectId
    $teamworkDefaultTasklistId = Get-NervAnswerOrDefault -Prompt 'Default tasklist id' -Current $defaultDefaultTasklistId
    $stageInDev = Get-NervAnswerOrDefault -Prompt 'Stage: in development' -Current $defaultStageInDev
    $stageTesting = Get-NervAnswerOrDefault -Prompt 'Stage: testing' -Current $defaultStageTesting
    $stageImplemented = Get-NervAnswerOrDefault -Prompt 'Stage: implemented' -Current $defaultStageImplemented
    $stageBlocked = Get-NervAnswerOrDefault -Prompt 'Stage: blocked' -Current $defaultStageBlocked
    $stageCanceled = Get-NervAnswerOrDefault -Prompt 'Stage: canceled' -Current $defaultStageCanceled
    $stagePending = Get-NervAnswerOrDefault -Prompt 'Stage: pending' -Current $defaultStagePending
    $stageAnalysis = Get-NervAnswerOrDefault -Prompt 'Stage: analysis' -Current $defaultStageAnalysis
}

Write-Host ""
Write-Host "-- skills (comma-separated) --"
$skillsTesting = Get-NervAnswerOrDefault -Prompt 'testing' -Current $defaultSkillsTesting
$skillsCode = Get-NervAnswerOrDefault -Prompt 'code' -Current $defaultSkillsCode
$skillsBestPractices = Get-NervAnswerOrDefault -Prompt 'best-practices' -Current $defaultSkillsBestPractices
$skillsArchitecture = Get-NervAnswerOrDefault -Prompt 'architecture' -Current $defaultSkillsArchitecture
$skillsAudit = Get-NervAnswerOrDefault -Prompt 'audit' -Current $defaultSkillsAudit

Write-Host ""
$criticalPaths = Get-NervAnswerOrDefault -Prompt 'Critical paths (comma-separated)' -Current $defaultCriticalPaths

Write-Host ""
$artifactsCommit = Get-NervChoiceOrDefault -Prompt 'Artifacts commit policy' -ValidValues @('with-change', 'at-close', 'never') -Current $defaultArtifactsCommit

# --- Apply only what actually changed, key by key, via Set-NervYamlScalar.
#     Format-NervGitBlock/Format-NervTasksBlock/Format-NervSkillsBlock/
#     Format-NervArtifactsBlock (whole-block builders) are used ONLY when
#     the block does not exist in the file at all yet (fresh setup); an
#     existing block is never regenerated wholesale, so unmanaged content
#     next to a field the wizard knows about (extra keys, trailing
#     comments, nested lists the wizard has no field for — known_projects,
#     sources:, sources_howto:, ...) is always preserved byte for byte. ---
$changeLog = [System.Collections.Generic.List[string]]::new()

# A brand-new file (no existing user config at all) is the one case where a
# missing block/key is always created with the built-in defaults — first-run
# setup should produce a complete config even when every prompt is kept at
# its default. For an EXISTING file, a missing block/key is created only
# when the user's answer for at least one of its fields actually differs
# from the built-in default; an empty/"keep" answer alone never adds
# anything to an existing file.
$isBrandNewFile = ($existingUserYaml -eq '')

$newUserYaml = $existingUserYaml
if ($newUserYaml -eq '') {
    $newUserYaml = "# ~/.claude/nerv/nerv.yaml -- NERV user-scope configuration`n# Generated/updated by tools/configure.ps1`n"
}

# -- git: --
$gitAnyChanged = ($gitBaseBranch -ne $defaultBaseBranch) -or ($gitWorktree -ne $defaultWorktree) -or
    ($gitBranchPattern -ne $defaultBranchPattern) -or ($gitCommitRefPattern -ne $defaultCommitRefPattern)
if (-not (Test-NervYamlKeyExists -YamlText $newUserYaml -Key 'git')) {
    if ($isBrandNewFile -or $gitAnyChanged) {
        $gitBlockText = Format-NervGitBlock -Values @{
            base_branch        = $gitBaseBranch
            worktree           = $gitWorktree
            branch_pattern     = $gitBranchPattern
            commit_ref_pattern = $gitCommitRefPattern
        }
        $newUserYaml = Set-NervYamlBlock -YamlText $newUserYaml -Key 'git' -BlockText $gitBlockText
        $changeLog.Add('git: (new block)')
    }
}
else {
    if ($gitBaseBranch -ne $defaultBaseBranch) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'git.base_branch' -Value $gitBaseBranch
        $changeLog.Add("git.base_branch: $defaultBaseBranch -> $gitBaseBranch")
    }
    if ($gitWorktree -ne $defaultWorktree) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'git.worktree' -Value $gitWorktree
        $changeLog.Add("git.worktree: $defaultWorktree -> $gitWorktree")
    }
    if ($gitBranchPattern -ne $defaultBranchPattern) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'git.branch_pattern' -Value $gitBranchPattern
        $changeLog.Add("git.branch_pattern: $defaultBranchPattern -> $gitBranchPattern")
    }
    if ($gitCommitRefPattern -ne $defaultCommitRefPattern) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'git.commit_ref_pattern' -Value $gitCommitRefPattern
        $changeLog.Add("git.commit_ref_pattern: $defaultCommitRefPattern -> $gitCommitRefPattern")
    }
}

# -- tasks: --
$tasksTopFieldsChanged = ($tasksProvider -ne $defaultProvider) -or ($tasksAskWhenMissing -ne $defaultAskWhenMissing) -or
    ($tasksSubtasksPerWave -ne $defaultSubtasksPerWave) -or ($tasksTimerStore -ne $defaultTimerStore) -or
    ($tasksRoundingMinutes -ne $defaultRoundingMinutes)
$teamworkFieldsChanged = ($tasksProvider -eq 'teamwork') -and (
    ($teamworkTaskRefPrefix -ne $defaultTaskRefPrefix) -or ($teamworkAssigneeId -ne $defaultAssigneeId) -or
    ($teamworkDefaultProjectId -ne $defaultDefaultProjectId) -or ($teamworkDefaultTasklistId -ne $defaultDefaultTasklistId) -or
    ($stageInDev -ne $defaultStageInDev) -or ($stageTesting -ne $defaultStageTesting) -or
    ($stageImplemented -ne $defaultStageImplemented) -or ($stageBlocked -ne $defaultStageBlocked) -or
    ($stageCanceled -ne $defaultStageCanceled) -or ($stagePending -ne $defaultStagePending) -or
    ($stageAnalysis -ne $defaultStageAnalysis)
)
$tasksAnyChanged = $tasksTopFieldsChanged -or $teamworkFieldsChanged

if (-not (Test-NervYamlKeyExists -YamlText $newUserYaml -Key 'tasks')) {
    if ($isBrandNewFile -or $tasksAnyChanged) {
        $tasksBlockText = Format-NervTasksBlock -Values @{
            provider                     = $tasksProvider
            ask_when_missing             = $tasksAskWhenMissing
            subtasks_per_wave            = $tasksSubtasksPerWave
            timer_store                  = $tasksTimerStore
            rounding_minutes             = $tasksRoundingMinutes
            teamwork_task_ref_prefix     = $teamworkTaskRefPrefix
            teamwork_assignee_id         = $teamworkAssigneeId
            teamwork_default_project_id  = $teamworkDefaultProjectId
            teamwork_default_tasklist_id = $teamworkDefaultTasklistId
            teamwork_stage_inDev         = $stageInDev
            teamwork_stage_testing       = $stageTesting
            teamwork_stage_implemented   = $stageImplemented
            teamwork_stage_blocked       = $stageBlocked
            teamwork_stage_canceled      = $stageCanceled
            teamwork_stage_pending       = $stagePending
            teamwork_stage_analysis      = $stageAnalysis
        } -SourcesRaw $null
        $newUserYaml = Set-NervYamlBlock -YamlText $newUserYaml -Key 'tasks' -BlockText $tasksBlockText
        $changeLog.Add('tasks: (new block)')
    }
}
else {
    if ($tasksProvider -ne $defaultProvider) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.provider' -Value $tasksProvider
        $changeLog.Add("tasks.provider: $defaultProvider -> $tasksProvider")
    }
    if ($tasksAskWhenMissing -ne $defaultAskWhenMissing) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.ask_when_missing' -Value $tasksAskWhenMissing
        $changeLog.Add("tasks.ask_when_missing: $defaultAskWhenMissing -> $tasksAskWhenMissing")
    }
    if ($tasksSubtasksPerWave -ne $defaultSubtasksPerWave) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.subtasks_per_wave' -Value $tasksSubtasksPerWave
        $changeLog.Add("tasks.subtasks_per_wave: $defaultSubtasksPerWave -> $tasksSubtasksPerWave")
    }
    if ($tasksTimerStore -ne $defaultTimerStore) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.timer_store' -Value $tasksTimerStore
        $changeLog.Add("tasks.timer_store: $defaultTimerStore -> $tasksTimerStore")
    }
    if ($tasksRoundingMinutes -ne $defaultRoundingMinutes) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.rounding_minutes' -Value $tasksRoundingMinutes
        $changeLog.Add("tasks.rounding_minutes: $defaultRoundingMinutes -> $tasksRoundingMinutes")
    }

    if ($tasksProvider -eq 'teamwork') {
        $tasksBlockNow = Get-NervYamlBlock -YamlText $newUserYaml -Key 'tasks'
        $providersSub = Get-NervYamlSubBlock -BlockText $tasksBlockNow -Key 'providers' -Indent 2
        $teamworkSub = if ($providersSub) { Get-NervYamlSubBlock -BlockText $providersSub -Key 'teamwork' -Indent 4 } else { $null }

        if ($null -eq $teamworkSub) {
            if ($isBrandNewFile -or $teamworkFieldsChanged) {
                # Fresh sub-block: Set-NervYamlScalar's own missing-parent-chain
                # creation builds "  providers:" / "    teamwork:" as needed,
                # one field at a time — the same tested mechanism used for a
                # single missing field, just applied to every field here.
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.task_ref_prefix' -Value $teamworkTaskRefPrefix
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.assignee_id' -Value $teamworkAssigneeId
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.default_project_id' -Value $teamworkDefaultProjectId
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.default_tasklist_id' -Value $teamworkDefaultTasklistId
                $stagesValue = "{ inDev: $stageInDev, testing: $stageTesting, implemented: $stageImplemented, blocked: $stageBlocked, canceled: $stageCanceled, pending: $stagePending, analysis: $stageAnalysis }"
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.stages' -Value $stagesValue -Raw
                $changeLog.Add('tasks.providers.teamwork: (new sub-block)')
            }
        }
        else {
            if ($teamworkTaskRefPrefix -ne $defaultTaskRefPrefix) {
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.task_ref_prefix' -Value $teamworkTaskRefPrefix
                $changeLog.Add("tasks.providers.teamwork.task_ref_prefix: $defaultTaskRefPrefix -> $teamworkTaskRefPrefix")
            }
            if ($teamworkAssigneeId -ne $defaultAssigneeId) {
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.assignee_id' -Value $teamworkAssigneeId
                $changeLog.Add("tasks.providers.teamwork.assignee_id: $defaultAssigneeId -> $teamworkAssigneeId")
            }
            if ($teamworkDefaultProjectId -ne $defaultDefaultProjectId) {
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.default_project_id' -Value $teamworkDefaultProjectId
                $changeLog.Add("tasks.providers.teamwork.default_project_id: $defaultDefaultProjectId -> $teamworkDefaultProjectId")
            }
            if ($teamworkDefaultTasklistId -ne $defaultDefaultTasklistId) {
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.default_tasklist_id' -Value $teamworkDefaultTasklistId
                $changeLog.Add("tasks.providers.teamwork.default_tasklist_id: $defaultDefaultTasklistId -> $teamworkDefaultTasklistId")
            }

            $stagesChanged = ($stageInDev -ne $defaultStageInDev) -or ($stageTesting -ne $defaultStageTesting) -or
                ($stageImplemented -ne $defaultStageImplemented) -or ($stageBlocked -ne $defaultStageBlocked) -or
                ($stageCanceled -ne $defaultStageCanceled) -or ($stagePending -ne $defaultStagePending) -or
                ($stageAnalysis -ne $defaultStageAnalysis)
            if ($stagesChanged) {
                $stagesValue = "{ inDev: $stageInDev, testing: $stageTesting, implemented: $stageImplemented, blocked: $stageBlocked, canceled: $stageCanceled, pending: $stagePending, analysis: $stageAnalysis }"
                $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'tasks.providers.teamwork.stages' -Value $stagesValue -Raw
                $changeLog.Add('tasks.providers.teamwork.stages: updated')
            }
        }
    }
}

# -- skills: --
$skillsAnyChanged = ($skillsTesting -ne $defaultSkillsTesting) -or ($skillsCode -ne $defaultSkillsCode) -or
    ($skillsBestPractices -ne $defaultSkillsBestPractices) -or ($skillsArchitecture -ne $defaultSkillsArchitecture) -or
    ($skillsAudit -ne $defaultSkillsAudit)

if (-not (Test-NervYamlKeyExists -YamlText $newUserYaml -Key 'skills')) {
    if ($isBrandNewFile -or $skillsAnyChanged) {
        $skillsBlockText = Format-NervSkillsBlock -Values @{
            testing          = $skillsTesting
            code             = $skillsCode
            'best-practices' = $skillsBestPractices
            architecture     = $skillsArchitecture
            audit            = $skillsAudit
        }
        $newUserYaml = Set-NervYamlBlock -YamlText $newUserYaml -Key 'skills' -BlockText $skillsBlockText
        $changeLog.Add('skills: (new block)')
    }
}
elseif (Test-NervYamlKeyIsInline -YamlText $newUserYaml -Key 'skills') {
    # skills: written as a single-line inline map (`skills: { testing: [...], ... }`).
    # Rewrite only the changed categories' bracket content in place, keeping
    # every other category's raw text and the line's own trailing comment
    # exactly as written, instead of regenerating the whole line.
    if ($skillsAnyChanged) {
        $currentInline = Read-NervScalar -YamlText $newUserYaml -Path 'skills'
        if ($skillsTesting -ne $defaultSkillsTesting) {
            $currentInline = Set-NervYamlInlineListField -InlineText $currentInline -Key 'testing' -NewValue $skillsTesting
            $changeLog.Add("skills.testing: $defaultSkillsTesting -> $skillsTesting")
        }
        if ($skillsCode -ne $defaultSkillsCode) {
            $currentInline = Set-NervYamlInlineListField -InlineText $currentInline -Key 'code' -NewValue $skillsCode
            $changeLog.Add("skills.code: $defaultSkillsCode -> $skillsCode")
        }
        if ($skillsBestPractices -ne $defaultSkillsBestPractices) {
            $currentInline = Set-NervYamlInlineListField -InlineText $currentInline -Key 'best-practices' -NewValue $skillsBestPractices
            $changeLog.Add("skills.best-practices: $defaultSkillsBestPractices -> $skillsBestPractices")
        }
        if ($skillsArchitecture -ne $defaultSkillsArchitecture) {
            $currentInline = Set-NervYamlInlineListField -InlineText $currentInline -Key 'architecture' -NewValue $skillsArchitecture
            $changeLog.Add("skills.architecture: $defaultSkillsArchitecture -> $skillsArchitecture")
        }
        if ($skillsAudit -ne $defaultSkillsAudit) {
            $currentInline = Set-NervYamlInlineListField -InlineText $currentInline -Key 'audit' -NewValue $skillsAudit
            $changeLog.Add("skills.audit: $defaultSkillsAudit -> $skillsAudit")
        }
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'skills' -Value $currentInline -Raw
    }
}
else {
    if ($skillsTesting -ne $defaultSkillsTesting) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'skills.testing' -Value "[$skillsTesting]" -Raw
        $changeLog.Add("skills.testing: $defaultSkillsTesting -> $skillsTesting")
    }
    if ($skillsCode -ne $defaultSkillsCode) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'skills.code' -Value "[$skillsCode]" -Raw
        $changeLog.Add("skills.code: $defaultSkillsCode -> $skillsCode")
    }
    if ($skillsBestPractices -ne $defaultSkillsBestPractices) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'skills.best-practices' -Value "[$skillsBestPractices]" -Raw
        $changeLog.Add("skills.best-practices: $defaultSkillsBestPractices -> $skillsBestPractices")
    }
    if ($skillsArchitecture -ne $defaultSkillsArchitecture) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'skills.architecture' -Value "[$skillsArchitecture]" -Raw
        $changeLog.Add("skills.architecture: $defaultSkillsArchitecture -> $skillsArchitecture")
    }
    if ($skillsAudit -ne $defaultSkillsAudit) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'skills.audit' -Value "[$skillsAudit]" -Raw
        $changeLog.Add("skills.audit: $defaultSkillsAudit -> $skillsAudit")
    }
}

# -- critical_paths: -- (always a single inline line; Test-NervYamlKeyExists,
#    not Get-NervYamlBlock, is the correct existence check for it)
if ($isBrandNewFile -or ($criticalPaths -ne $defaultCriticalPaths)) {
    $criticalPathsList = @($criticalPaths -split ',' | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne '' })
    if (-not (Test-NervYamlKeyExists -YamlText $newUserYaml -Key 'critical_paths')) {
        $newUserYaml = Set-NervYamlBlock -YamlText $newUserYaml -Key 'critical_paths' -BlockText (Format-NervCriticalPathsLine -Paths $criticalPathsList)
    }
    else {
        $joinedCriticalPaths = ($criticalPathsList -join ', ')
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'critical_paths' -Value "[$joinedCriticalPaths]" -Raw
    }
    if ($criticalPaths -ne $defaultCriticalPaths) {
        $changeLog.Add("critical_paths: $defaultCriticalPaths -> $criticalPaths")
    }
    else {
        $changeLog.Add('critical_paths: (new)')
    }
}

# -- artifacts: --
if (-not (Test-NervYamlKeyExists -YamlText $newUserYaml -Key 'artifacts')) {
    if ($isBrandNewFile -or ($artifactsCommit -ne $defaultArtifactsCommit)) {
        $newUserYaml = Set-NervYamlBlock -YamlText $newUserYaml -Key 'artifacts' -BlockText (Format-NervArtifactsBlock -Commit $artifactsCommit)
        $changeLog.Add("artifacts: (new block, commit: $artifactsCommit)")
    }
}
else {
    if ($artifactsCommit -ne $defaultArtifactsCommit) {
        $newUserYaml = Set-NervYamlScalar -YamlText $newUserYaml -Path 'artifacts.commit' -Value $artifactsCommit
        $changeLog.Add("artifacts.commit: $defaultArtifactsCommit -> $artifactsCommit")
    }
}

Write-Host ""
if ($changeLog.Count -eq 0) {
    Write-Host "No changes; $userConfigPath left untouched."
}
else {
    Write-Host "--- Summary ---"
    foreach ($c in $changeLog) { Write-Host "  $c" }

    $writeConfirm = (Read-NervConfigureAnswer -Prompt "Write to ${userConfigPath}? [Y/n]" -DefaultWhenExhausted 'Y').Trim()
    $writeYes = ($writeConfirm -eq '') -or ($writeConfirm -match '(?i)^y(es)?$')

    if ($writeYes) {
        if ($userConfigExists) {
            $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
            $backupPath = "$userConfigPath.bak-configure-$timestamp"
            Copy-Item -LiteralPath $userConfigPath -Destination $backupPath -Force
            Write-Host "Backup written: $backupPath"
        }
        else {
            $configDir = Split-Path -Parent $userConfigPath
            if ($configDir -and -not (Test-Path -LiteralPath $configDir)) {
                New-Item -ItemType Directory -Path $configDir -Force | Out-Null
            }
        }
        $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($userConfigPath, $newUserYaml, $utf8NoBom)
        Write-Host "Written: $userConfigPath"
    }
    else {
        Write-Host "Aborted; no changes written to user config."
    }
}

# --- Section 2: Models ---
if (-not $SkipModels) {
    Write-Host ""
    Write-Host "--- Models ---"
    $configureModelsConfirm = (Read-NervConfigureAnswer -Prompt 'Configure per-role model and effort now? [y/N]' -DefaultWhenExhausted 'N').Trim()
    $configureModelsYes = ($configureModelsConfirm -match '(?i)^y(es)?$')
    if ($configureModelsYes) {
        $modelsArgs = @('-NoProfile', '-File', $modelsScriptPath, '-ConfigPath', $userConfigPath, '-NoApply')
        if ($ModelsAnswersFile) { $modelsArgs += @('-AnswersFile', $ModelsAnswersFile) }
        & pwsh @modelsArgs
    }
}

# --- Section 3: Repos ---
if (-not $SkipRepos) {
    Write-Host ""
    Write-Host "--- Repositories ---"
    $reposDone = $false
    while (-not $reposDone) {
        $repoPathAnswer = (Read-NervConfigureAnswer -Prompt 'Initialize a repository for NERV now? (path, or Enter to skip)' -DefaultWhenExhausted '').Trim()
        if ($repoPathAnswer -eq '') { $reposDone = $true; continue }

        if (-not (Test-Path -LiteralPath $repoPathAnswer)) {
            Write-Host "Path not found: $repoPathAnswer"
            continue
        }

        $global:LASTEXITCODE = 0
        $repoToplevel = $null
        try { $repoToplevel = (& git -C $repoPathAnswer rev-parse --show-toplevel 2>$null) } catch { $repoToplevel = $null }
        if ($LASTEXITCODE -ne 0 -or -not $repoToplevel) {
            Write-Host "Not a git repository: $repoPathAnswer"
            continue
        }
        $repoToplevel = ([string]$repoToplevel).Trim()

        $repoConfigPath = Join-Path $repoToplevel ".nerv/nerv.yaml"
        if (Test-Path -LiteralPath $repoConfigPath) {
            Write-Host "$repoConfigPath already initialized, left untouched."
            continue
        }

        $repoBaseBranch = Get-NervAnswerOrDefault -Prompt 'Base branch for this repo' -Current $gitBaseBranch
        $repoProvider = Get-NervChoiceOrDefault -Prompt 'Task provider for this repo' -ValidValues @('teamwork', 'github-projects', 'jira', 'none') -Current $tasksProvider

        $repoProjectValues = @{
            base_branch = $repoBaseBranch
            provider    = $repoProvider
        }

        if ($repoProvider -eq 'teamwork') {
            $repoProjectId = Get-NervAnswerOrDefault -Prompt 'Teamwork project id for this repo' -Current ''
            $repoTasklistId = Get-NervAnswerOrDefault -Prompt 'Teamwork tasklist id for this repo' -Current ''
            $repoProjectValues['project_id'] = $repoProjectId
            $repoProjectValues['tasklist_id'] = $repoTasklistId
        }

        $repoProjectYaml = Format-NervProjectFile -Values $repoProjectValues
        $repoConfigDir = Split-Path -Parent $repoConfigPath
        if (-not (Test-Path -LiteralPath $repoConfigDir)) {
            New-Item -ItemType Directory -Path $repoConfigDir -Force | Out-Null
        }
        $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($repoConfigPath, $repoProjectYaml, $utf8NoBom)
        Write-Host "Written: $repoConfigPath"
        Write-Host "Reminder: open Claude Code in that repo and run /nerv:init once to bootstrap gentle-ai's SDD registry if it is missing."
    }
}

# --- Section 4: Slash commands ---
if (-not $SkipCommands) {
    Write-Host ""
    Write-Host "--- Slash commands ---"
    $proceduresDir = Join-Path $RepoPath "plugin/skills/nerv-tasks/providers/teamwork/procedures"
    if (Test-Path -LiteralPath $proceduresDir) {
        $installCommandsConfirm = (Read-NervConfigureAnswer -Prompt "Install the Teamwork procedures as /task:* commands in $homeDirResolved\.claude\commands\task\? [y/N]" -DefaultWhenExhausted 'N').Trim()
        $installCommandsYes = ($installCommandsConfirm -match '(?i)^y(es)?$')
        if ($installCommandsYes) {
            $destDir = Join-Path $homeDirResolved ".claude/commands/task"
            if (-not (Test-Path -LiteralPath $destDir)) {
                New-Item -ItemType Directory -Path $destDir -Force | Out-Null
            }
            $copied = [System.Collections.Generic.List[string]]::new()
            $skipped = [System.Collections.Generic.List[string]]::new()
            foreach ($file in Get-ChildItem -LiteralPath $proceduresDir -Filter '*.md' -File) {
                $destFile = Join-Path $destDir $file.Name
                if (Test-Path -LiteralPath $destFile) {
                    $skipped.Add($file.Name)
                    continue
                }
                Copy-Item -LiteralPath $file.FullName -Destination $destFile
                $copied.Add($file.Name)
            }
            Write-Host "Copied: $($copied.Count) file(s)."
            if ($skipped.Count -gt 0) {
                Write-Host "Skipped (already exist): $($skipped -join ', ')"
            }
        }
    }
    else {
        Write-Host "Teamwork procedures not found at $proceduresDir; skipping."
    }
}

# --- Section 5: Apply and refresh ---
if (-not $NoRefresh) {
    Write-Host ""
    $applyRefreshConfirm = (Read-NervConfigureAnswer -Prompt 'Apply models to the plugin cache and refresh it now? [Y/n]' -DefaultWhenExhausted 'Y').Trim()
    $applyRefreshYes = ($applyRefreshConfirm -eq '') -or ($applyRefreshConfirm -match '(?i)^y(es)?$')
    if ($applyRefreshYes) {
        Invoke-NervApplyModels -RepoPath $RepoPath
        & pwsh -NoProfile -File (Join-Path $PSScriptRoot 'install.ps1') -RefreshCache
    }
}

Write-Host ""
Write-Host "Restart Claude Code for the change to take effect."

} # end of `if ($MyInvocation.InvocationName -ne '.')` interactive-body guard
