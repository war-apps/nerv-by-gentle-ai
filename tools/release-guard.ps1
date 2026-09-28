#requires -Version 7
<#
.SYNOPSIS
    Release readiness guard: fails a CI run before it tags or publishes
    anything unless plugin/.claude-plugin/plugin.json's version, the git
    tag `v<version>`, and CHANGELOG.md are in agreement.

.DESCRIPTION
    This is repository tooling, not plugin runtime: it lives only under
    tools/ (never under plugin/) and is never shipped to installed users.

    Reads the current version out of plugin.json (reusing
    Get-NervPluginVersion from tools/release.ps1, dot-sourced for that
    purpose -- its own CLI body stays inert behind its
    `$MyInvocation.InvocationName -ne '.'` guard). The guard fails when:
      - the tag `v<version>` already exists in the repository, or
      - CHANGELOG.md is missing, or
      - CHANGELOG.md has no `## [<version>]` section.

    On success it prints the version and, when -NotesPath is given,
    writes the body of that changelog section (the lines after the
    `## [<version>] - <date>` heading up to the next `## [` heading,
    trimmed of leading/trailing blank lines) to that file as UTF-8
    without a BOM, LF line endings, ending in exactly one newline.

    Never prompts. Never tags, commits, or writes anything other than the
    optional notes file on success.

.PARAMETER RepoPath
    Repository root to operate on. Defaults to the parent of this
    script's directory, i.e. the repo root when this script runs in
    place from `tools/release-guard.ps1`.

.PARAMETER NotesPath
    When given and the guard succeeds, the changelog section body is
    written to this file.

.PARAMETER Json
    Print the result as one JSON object instead of human-readable text,
    in every outcome (success or failure):
    `{version, tag, changelog_section, tag_exists, notes_path, ok}`.

.EXAMPLE
    pwsh tools/release-guard.ps1

.EXAMPLE
    pwsh tools/release-guard.ps1 -NotesPath notes.md -Json
#>

[CmdletBinding()]
param(
    [string]$RepoPath = (Split-Path -Parent $PSScriptRoot),

    [string]$NotesPath,

    [switch]$Json
)

Set-StrictMode -Version Latest

# Capture this script's own parameters under distinct names *before*
# dot-sourcing tools/release.ps1 below: dot-sourcing shares this scope, so
# release.ps1's own param block (RepoPath, Preview, Apply, Version,
# PreRelease, Json) would otherwise silently reassign any same-named
# variable here (in particular $RepoPath and $Json) to release.ps1's
# defaults.
$guardRepoPath = $RepoPath
$guardNotesPath = $NotesPath
$guardJson = $Json.IsPresent

# Reuse Get-NervPluginVersion instead of re-implementing plugin.json
# parsing. Passing -RepoPath explicitly (even though this guard does not
# use release.ps1's own $RepoPath afterward) keeps the shared-scope
# reassignment harmless -- see the capture above.
. (Join-Path $PSScriptRoot 'release.ps1') -RepoPath $guardRepoPath

# =============================================================================
# Pure/dot-sourceable functions -- same pattern as tools/release.ps1.
# =============================================================================

function Get-NervChangelogSectionBody {
    <#
    .SYNOPSIS
        Extracts the body of one `## [<version>] ...` changelog section
        from $Text: everything after that heading line up to (but not
        including) the next `## [` heading, or end of text when it is the
        last section. Leading and trailing blank lines are trimmed, so
        the result never starts or ends with a newline. Returns $null
        when no `## [<version>]` heading is found.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Text,
        [Parameter(Mandatory)][string]$Version
    )

    $escapedVersion = [regex]::Escape($Version)
    $headingMatch = [regex]::Match($Text, "(?m)^## \[$escapedVersion\][^\r\n]*")
    if (-not $headingMatch.Success) {
        return $null
    }

    $afterHeadingIndex = $headingMatch.Index + $headingMatch.Length
    $rest = $Text.Substring($afterHeadingIndex)
    $nextHeadingMatch = [regex]::Match($rest, '(?m)^## \[')
    $body = if ($nextHeadingMatch.Success) { $rest.Substring(0, $nextHeadingMatch.Index) } else { $rest }

    return $body.Trim("`r", "`n")
}

function Test-NervReleaseReadiness {
    <#
    .SYNOPSIS
        Checks whether $RepoPath is ready to release the version
        currently set in plugin/.claude-plugin/plugin.json: the tag
        `v<version>` must not already exist, and CHANGELOG.md must
        contain a matching `## [<version>]` section. Returns
        `{Version, Tag, TagExists, ChangelogSection, Ok, Reason, Body}`.

        Throws on a git failure (e.g. $RepoPath is not a git repository)
        or when plugin.json has no readable version -- callers map that
        to exit 2. A returned object with `Ok = $false` and a `Reason`
        is a normal, expected outcome (callers map that to exit 1).
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$RepoPath
    )

    $pluginJsonPath = Join-Path $RepoPath 'plugin/.claude-plugin/plugin.json'
    $version = Get-NervPluginVersion -Path $pluginJsonPath
    $tag = "v$version"

    $global:LASTEXITCODE = 0
    $rawTags = & git -C $RepoPath tag -l $tag 2>$null
    if ($LASTEXITCODE -ne 0) {
        throw "git tag failed in '$RepoPath' (exit $LASTEXITCODE)."
    }
    $tagExists = @($rawTags | Where-Object { $_ -eq $tag }).Count -gt 0

    $changelogPath = Join-Path $RepoPath 'CHANGELOG.md'
    $changelogExists = Test-Path -LiteralPath $changelogPath

    $body = $null
    $hasSection = $false
    if ($changelogExists) {
        $text = [System.IO.File]::ReadAllText($changelogPath)
        $body = Get-NervChangelogSectionBody -Text $text -Version $version
        $hasSection = $null -ne $body
    }

    $ok = $true
    $reason = $null
    if ($tagExists) {
        $ok = $false
        $reason = "Tag '$tag' already exists."
    }
    elseif (-not $changelogExists) {
        $ok = $false
        $reason = "CHANGELOG.md not found at '$changelogPath'."
    }
    elseif (-not $hasSection) {
        $ok = $false
        $reason = "No '## [$version]' section found in CHANGELOG.md."
    }

    return [PSCustomObject]@{
        Version          = $version
        Tag              = $tag
        TagExists        = $tagExists
        ChangelogSection = $hasSection
        Ok               = $ok
        Reason           = $reason
        Body             = $body
    }
}

# =============================================================================
# CLI body. Guarded so dot-sourcing (tests) only defines the functions above
# and never runs the CLI -- same pattern as tools/release.ps1.
# =============================================================================
if ($MyInvocation.InvocationName -ne '.') {

    $ErrorActionPreference = "Stop"

    try {
        if (-not (Test-Path -LiteralPath $guardRepoPath)) {
            Write-Host "Repository path not found: '$guardRepoPath'."
            exit 2
        }
        $repoPathResolved = (Resolve-Path -LiteralPath $guardRepoPath).ProviderPath

        $pluginJsonPath = Join-Path $repoPathResolved 'plugin/.claude-plugin/plugin.json'
        if (-not (Test-Path -LiteralPath $pluginJsonPath)) {
            Write-Host "plugin.json not found at '$pluginJsonPath'."
            exit 2
        }

        $readiness = Test-NervReleaseReadiness -RepoPath $repoPathResolved

        if ($readiness.Ok -and $guardNotesPath) {
            $notesDir = Split-Path -Parent $guardNotesPath
            if ($notesDir -and -not (Test-Path -LiteralPath $notesDir)) {
                New-Item -ItemType Directory -Path $notesDir -Force | Out-Null
            }
            $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
            [System.IO.File]::WriteAllText($guardNotesPath, ($readiness.Body + "`n"), $utf8NoBom)
        }

        $result = [ordered]@{
            version           = $readiness.Version
            tag               = $readiness.Tag
            changelog_section = $readiness.ChangelogSection
            tag_exists        = $readiness.TagExists
            notes_path        = if ($guardNotesPath) { $guardNotesPath } else { $null }
            ok                = $readiness.Ok
        }

        if ($guardJson) {
            ConvertTo-Json -InputObject $result -Depth 5
        }
        else {
            if ($readiness.Ok) {
                Write-Host $readiness.Version
            }
            else {
                Write-Host $readiness.Reason
            }
        }

        if ($readiness.Ok) { exit 0 } else { exit 1 }
    }
    catch {
        Write-Host "Unexpected error: $_"
        exit 2
    }
}
