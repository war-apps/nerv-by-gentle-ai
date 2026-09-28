#requires -Version 7
<#
.SYNOPSIS
    Installs or verifies the Claude Code user-scope skills that NERV's
    nerv.yaml `skills:` categories reference.

.DESCRIPTION
    NERV's default categories reference skill names that must exist under
    `~/.claude/skills/<name>/SKILL.md`. Two kinds of skills are covered:

    - external:   installed via `npx skills add <repo> [--skill <id>] -g -a
                  claude-code -y` (skills.sh). Missing ones get installed
                  (or, with -DryRun, the exact command is printed instead).
    - gentle-ai:  shipped by `gentle-ai install` / `gentle-ai sync`, never by
                  npx. Missing ones are reported as a gap with a remedy line;
                  this script never tries to install them.
    - builtin:    a Claude Code built-in (e.g. security-review) that needs no
                  directory. Always reported as already satisfied.

    Reads tools/skills-manifest.json (or -ManifestPath) and compares each
    entry against $SkillsDir (default `~/.claude/skills`).

.PARAMETER ManifestPath
    Path to the skills manifest JSON. Defaults to skills-manifest.json next
    to this script.

.PARAMETER SkillsDir
    Directory to check/install skills into. Defaults to
    `<home>/.claude/skills`, where home is $env:HOME with $env:USERPROFILE
    as a fallback.

.PARAMETER Only
    Restrict processing to these skill names (by manifest `name`).

.PARAMETER DryRun
    Print the exact npx command for each missing external skill instead of
    running it. No filesystem or network side effects.

.PARAMETER Json
    Print the computed status array as JSON instead of the human table
    (intended for consumption by the setup wizard).

.EXAMPLE
    pwsh -File tools/install-skills.ps1 -DryRun

.EXAMPLE
    pwsh -File tools/install-skills.ps1 -Only tdd,solid-principles
#>

param(
    [string]$ManifestPath = (Join-Path $PSScriptRoot 'skills-manifest.json'),
    [string]$SkillsDir,
    [string[]]$Only,
    [switch]$DryRun,
    [switch]$Json
)

$ErrorActionPreference = "Stop"

# =============================================================================
# Pure functions (dot-sourceable)
# =============================================================================

function Read-NervSkillsManifest {
    <#
    .SYNOPSIS
        Reads and validates tools/skills-manifest.json, returning its
        `skills` array. Throws on a malformed manifest (wrong schema,
        missing `skills`, or an entry missing a required key).
    #>
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        throw "Skills manifest not found: $Path"
    }

    $raw = Get-Content -LiteralPath $Path -Raw
    $parsed = $raw | ConvertFrom-Json

    if (-not $parsed.PSObject.Properties['schema'] -or $parsed.schema -ne 'nerv.skills-manifest/v1') {
        throw "Skills manifest at '$Path' has an unexpected or missing 'schema' (expected nerv.skills-manifest/v1)."
    }
    if (-not $parsed.PSObject.Properties['skills']) {
        throw "Skills manifest at '$Path' is missing a 'skills' array."
    }

    $entries = @($parsed.skills)
    foreach ($entry in $entries) {
        foreach ($requiredKey in @('name', 'kind', 'used_by')) {
            if (-not $entry.PSObject.Properties[$requiredKey]) {
                throw "Skills manifest entry is missing required key '$requiredKey': $($entry | ConvertTo-Json -Compress)"
            }
        }
        if ($entry.kind -notin @('external', 'gentle-ai', 'builtin')) {
            throw "Skills manifest entry '$($entry.name)' has unknown kind '$($entry.kind)' (expected external, gentle-ai, or builtin)."
        }
        if ($entry.kind -eq 'external' -and -not $entry.PSObject.Properties['repo']) {
            throw "Skills manifest entry '$($entry.name)' is kind 'external' but has no 'repo'."
        }
    }

    return $entries
}

function Test-NervSkillInstalled {
    <#
    .SYNOPSIS
        Returns $true when <SkillsDir>/<Name>/SKILL.md exists.
    #>
    param(
        [Parameter(Mandatory)][string]$SkillsDir,
        [Parameter(Mandatory)][string]$Name
    )
    $skillMdPath = Join-Path (Join-Path $SkillsDir $Name) 'SKILL.md'
    return (Test-Path -LiteralPath $skillMdPath -PathType Leaf)
}

function Get-NervSkillsStatus {
    <#
    .SYNOPSIS
        Computes { name, kind, repo, skill, installed, action } for each
        manifest entry against $SkillsDir.

        action is:
          - 'install'           external & missing
          - 'verify-gentle-ai'  gentle-ai & missing
          - 'none'              installed, or builtin (never needs a dir)
    #>
    param(
        [Parameter(Mandatory)][array]$Manifest,
        [Parameter(Mandatory)][string]$SkillsDir
    )

    $results = @()
    foreach ($entry in $Manifest) {
        if ($entry.kind -eq 'builtin') {
            $installed = $true
        }
        else {
            $installed = Test-NervSkillInstalled -SkillsDir $SkillsDir -Name $entry.name
        }

        if ($installed) {
            $action = 'none'
        }
        elseif ($entry.kind -eq 'external') {
            $action = 'install'
        }
        elseif ($entry.kind -eq 'gentle-ai') {
            $action = 'verify-gentle-ai'
        }
        else {
            $action = 'none'
        }

        $repo = $null
        if ($entry.PSObject.Properties['repo']) { $repo = $entry.repo }
        $skill = $null
        if ($entry.PSObject.Properties['skill']) { $skill = $entry.skill }

        $results += [pscustomobject]@{
            name      = $entry.name
            kind      = $entry.kind
            repo      = $repo
            skill     = $skill
            installed = $installed
            action    = $action
        }
    }
    return $results
}

function New-NervSkillInstallArgs {
    <#
    .SYNOPSIS
        Builds the argv array for `npx <args>` that installs one external
        skill: `skills add <repo> [--skill <id>] -g -a claude-code -y`.
    #>
    param([Parameter(Mandatory)]$Entry)

    $argv = @('skills', 'add', $Entry.repo)
    if ($Entry.PSObject.Properties['skill'] -and $Entry.skill) {
        $argv += @('--skill', $Entry.skill)
    }
    $argv += @('-g', '-a', 'claude-code', '-y')
    return $argv
}

# =============================================================================
# Body (guarded: dot-sourcing this file only defines the functions above)
# =============================================================================

if ($MyInvocation.InvocationName -ne '.') {

    if (-not $SkillsDir) {
        $home1 = $env:HOME
        if (-not $home1) { $home1 = $env:USERPROFILE }
        $SkillsDir = Join-Path (Join-Path $home1 '.claude') 'skills'
    }

    $manifest = Read-NervSkillsManifest -Path $ManifestPath

    if ($Only) {
        $known = @($manifest | ForEach-Object { $_.name })
        $unknown = @($Only | Where-Object { $known -notcontains $_ })
        if ($unknown.Count -gt 0) {
            Write-Host "Unknown skill name(s) for -Only: $($unknown -join ', '). Known names: $($known -join ', ')."
            exit 1
        }
        $manifest = @($manifest | Where-Object { $Only -contains $_.name })
    }

    $status = Get-NervSkillsStatus -Manifest $manifest -SkillsDir $SkillsDir

    if ($Json) {
        ConvertTo-Json -InputObject @($status) -Depth 5
    }
    else {
        Write-Host ("{0,-32} {1,-10} {2,-10} {3}" -f "name", "kind", "installed", "action")
        foreach ($s in $status) {
            Write-Host ("{0,-32} {1,-10} {2,-10} {3}" -f $s.name, $s.kind, $s.installed, $s.action)
        }
    }

    $installedCount = 0
    $alreadyPresentCount = 0
    $gentleAiGapCount = 0
    $failureCount = 0

    foreach ($s in $status) {
        if ($s.action -eq 'none') {
            $alreadyPresentCount++
            continue
        }
        if ($s.action -eq 'verify-gentle-ai') {
            $gentleAiGapCount++
            if (-not $Json) {
                Write-Host "Remedy: run 'gentle-ai install' (or 'gentle-ai sync') to provide gentle-ai skill '$($s.name)'."
            }
            continue
        }
        if ($s.action -eq 'install') {
            $manifestEntry = $manifest | Where-Object { $_.name -eq $s.name } | Select-Object -First 1
            $installArgs = New-NervSkillInstallArgs -Entry $manifestEntry

            if ($DryRun) {
                if (-not $Json) {
                    # Print the exact command a human would run. Keep the
                    # literal substring 'npx skills add' so both humans and
                    # this script's own end-to-end tests can grep for it.
                    Write-Host "DryRun: npx $($installArgs -join ' ')"
                }
                continue
            }

            $global:LASTEXITCODE = 0
            $npxFailed = $false
            try {
                & npx @installArgs
                if ($LASTEXITCODE -ne 0) { $npxFailed = $true }
            }
            catch {
                $npxFailed = $true
            }

            if ($npxFailed) {
                $failureCount++
                if (-not $Json) {
                    Write-Host "FAILED to install skill '$($s.name)' from '$($manifestEntry.repo)'."
                }
            }
            else {
                $installedCount++
            }
        }
    }

    if (-not $Json) {
        Write-Host ""
        Write-Host "skills: $installedCount installed, $alreadyPresentCount already present, $gentleAiGapCount gentle-ai gaps, $failureCount failures"
    }

    if ($failureCount -gt 0) { exit 1 }
    exit 0
}
