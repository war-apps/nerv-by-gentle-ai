#requires -Version 7
<#
.SYNOPSIS
    Tag-driven release helper: computes the next semantic version from
    Conventional Commits since the last release tag, and can write that
    version into plugin/.claude-plugin/plugin.json plus a matching
    Keep-a-Changelog section in CHANGELOG.md.

.DESCRIPTION
    This is repository tooling, not plugin runtime: it lives only under
    tools/ (never under plugin/) and is never shipped to installed users.

    Conventional Commits drive the bump: `feat` -> minor; `fix`, `perf` ->
    patch; a `!` before the colon or a `BREAKING CHANGE:` footer -> major;
    `docs`, `chore`, `test`, `refactor`, `ci`, `build`, `style`, `revert`
    (and non-conventional subjects) -> no bump on their own. The bump is
    computed from the commits reachable from HEAD since the highest
    `vX.Y.Z` tag (stable tags only, unless a function caller opts into
    pre-releases), or from the whole history when no tag exists yet.

    This script never creates a git tag and never commits: tagging and
    publishing are CI's job (see the release pipeline feature document).
    Running -Apply twice against the exact same tag state recomputes the
    exact same bump from the exact same commit history, so it is only a
    genuine no-op once a `vX.Y.Z` tag exists that covers the applied
    commits (normally created by CI after the release PR merges).

.PARAMETER RepoPath
    Repository root to operate on. Defaults to the parent of this
    script's directory, i.e. the repo root when this script runs in
    place from `tools/release.ps1`.

.PARAMETER Preview
    Compute and print the next version, bump kind, and changelog section
    without writing anything. This is also the default behavior when
    neither -Preview nor -Apply is passed.

.PARAMETER Apply
    Write the computed version into plugin.json and insert the computed
    changelog section into CHANGELOG.md.

.PARAMETER Version
    Explicit version override (plain `X.Y.Z`, no pre-release suffix).
    Must be valid semver and strictly greater than the current
    plugin.json version. Bypasses the Conventional Commits bump
    computation and the "nothing to release" check.

.PARAMETER PreRelease
    Pre-release label (e.g. `alpha`, `beta`, `rc`; must match `^[a-z]+$`)
    appended to the base version as `-<PreRelease>.<N>`, where N is one
    more than the highest existing `v<base>-<PreRelease>.N` tag for that
    same base version (other labels never interfere with the count).
    Ignored when -Version is also given, since -Version is already a
    complete version string. An invalid label exits 1 with a clear
    message.

.PARAMETER PreReleaseBase
    Selects what `-PreRelease` bumps from: `next` (default) uses the
    version computed from Conventional Commits since the last stable tag
    (today's behavior, requires a releasable commit); `current` uses the
    version already in plugin.json as-is and does not require a
    releasable commit. Meaningless without -PreRelease (exits 1).

.PARAMETER Json
    Print the result as one JSON object instead of human-readable text:
    `{current, last_tag, bump, next, tag, base, prerelease, commits,
    section, applied}`, plus `written` when -Apply ran. `tag` is
    `"v" + next` (`null` when next is null); `base` is the base version
    without any pre-release suffix.

.EXAMPLE
    pwsh tools/release.ps1 -Preview

.EXAMPLE
    pwsh tools/release.ps1 -Apply -Json

.EXAMPLE
    pwsh tools/release.ps1 -Version 1.0.0 -Apply

.EXAMPLE
    pwsh tools/release.ps1 -PreRelease rc -Json
#>

[CmdletBinding()]
param(
    [string]$RepoPath = (Split-Path -Parent $PSScriptRoot),

    [switch]$Preview,

    [switch]$Apply,

    [string]$Version,

    [string]$PreRelease,

    [ValidateSet('next', 'current')]
    [string]$PreReleaseBase = 'next',

    [switch]$Json
)

Set-StrictMode -Version Latest

# =============================================================================
# Pure functions -- dot-sourceable, no side effects other than the two
# explicitly designated writers (Set-NervPluginVersion, Update-NervChangelog).
# Everything below this block up to the closing brace of the
# `if ($MyInvocation.InvocationName -ne '.')` guard is this script's CLI body.
# =============================================================================

function Get-NervPluginVersion {
    <#
    .SYNOPSIS
        Reads the `version` field out of a plugin.json file via a targeted
        regex match (not a JSON parse/reserialize), so callers never risk
        reformatting the file just to read a value from it.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        throw "plugin.json not found at '$Path'."
    }

    $raw = [System.IO.File]::ReadAllText($Path)
    $match = [regex]::Match($raw, '"version"\s*:\s*"([^"]*)"')
    if (-not $match.Success) {
        throw "No `"version`" field found in '$Path'."
    }

    return $match.Groups[1].Value
}

function Get-NervLastReleaseTag {
    <#
    .SYNOPSIS
        Returns the highest `vX.Y.Z` tag in $RepoPath, ordered by semantic
        version (not by tag creation date or lexical order -- so `v0.10.0`
        correctly outranks `v0.9.0`). Returns $null when no matching tag
        exists.

    .PARAMETER IncludePreRelease
        Also consider `vX.Y.Z-rc.N` tags. A stable tag always outranks a
        pre-release tag of the same base version.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$RepoPath,
        [switch]$IncludePreRelease
    )

    $global:LASTEXITCODE = 0
    $rawTags = & git -C $RepoPath tag -l 'v*' 2>$null
    if ($LASTEXITCODE -ne 0) {
        throw "git tag failed in '$RepoPath' (exit $LASTEXITCODE)."
    }

    $releasePattern = '^v(\d+)\.(\d+)\.(\d+)$'
    $preReleasePattern = '^v(\d+)\.(\d+)\.(\d+)-rc\.(\d+)$'

    $candidates = [System.Collections.Generic.List[object]]::new()
    foreach ($t in @($rawTags | Where-Object { $_ })) {
        if ($t -match $releasePattern) {
            $candidates.Add([PSCustomObject]@{
                Tag          = $t
                Major        = [int]$Matches[1]
                Minor        = [int]$Matches[2]
                Patch        = [int]$Matches[3]
                ReleaseRank  = 1
                PreN         = 0
            })
        }
        elseif ($IncludePreRelease -and $t -match $preReleasePattern) {
            $candidates.Add([PSCustomObject]@{
                Tag          = $t
                Major        = [int]$Matches[1]
                Minor        = [int]$Matches[2]
                Patch        = [int]$Matches[3]
                ReleaseRank  = 0
                PreN         = [int]$Matches[4]
            })
        }
    }

    if ($candidates.Count -eq 0) {
        return $null
    }

    $sorted = $candidates | Sort-Object -Property `
        @{Expression = 'Major'; Descending = $true }, `
        @{Expression = 'Minor'; Descending = $true }, `
        @{Expression = 'Patch'; Descending = $true }, `
        @{Expression = 'ReleaseRank'; Descending = $true }, `
        @{Expression = 'PreN'; Descending = $true }

    return $sorted[0].Tag
}

function Get-NervCommitsSince {
    <#
    .SYNOPSIS
        Returns the commits reachable from HEAD since $Tag (or the whole
        history when $Tag is $null/empty), oldest first, merge commits
        excluded. Each result is `{Sha, ShortSha, Subject, Body}`.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$RepoPath,
        [string]$Tag
    )

    $gitArgs = @('-C', $RepoPath, 'log', '--no-merges', '--reverse', '--pretty=format:%H%x1f%h%x1f%s%x1f%b%x1e')
    if ($Tag) {
        $gitArgs += "$Tag..HEAD"
    }

    $global:LASTEXITCODE = 0
    $raw = & git @gitArgs 2>$null
    if ($LASTEXITCODE -ne 0) {
        $range = if ($Tag) { "$Tag..HEAD" } else { "HEAD" }
        throw "git log failed in '$RepoPath' for range '$range' (exit $LASTEXITCODE)."
    }

    $text = ($raw -join "`n")
    if ([string]::IsNullOrEmpty($text)) {
        return @()
    }

    $records = $text -split "`u{1e}" | Where-Object { $_.Trim("`r", "`n") -ne '' }
    $commits = foreach ($rec in $records) {
        $clean = $rec.TrimStart("`r", "`n")
        $parts = $clean -split "`u{1f}", 4
        if ($parts.Count -lt 4) { continue }
        [PSCustomObject]@{
            Sha      = $parts[0]
            ShortSha = $parts[1]
            Subject  = $parts[2]
            Body     = $parts[3].Trim("`r", "`n")
        }
    }

    return @($commits)
}

function Get-NervConventionalCommitInfo {
    <#
    .SYNOPSIS
        Parses one commit's Subject/Body into Conventional Commits parts:
        Type, Scope, IsBreaking (from a `!` before the colon, or a
        `BREAKING CHANGE:` footer in the body), and Description (the
        subject with the `type(scope)!:` prefix stripped). Non-conventional
        subjects get IsConventional = $false.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][AllowEmptyString()][string]$Subject,
        [AllowNull()][AllowEmptyString()][string]$Body
    )

    $pattern = '^(?<type>[a-zA-Z]+)(\((?<scope>[^)]*)\))?(?<breaking>!)?:\s*(?<desc>.*)$'
    $m = [regex]::Match($Subject, $pattern)

    if (-not $m.Success) {
        return [PSCustomObject]@{
            IsConventional = $false
            Type           = $null
            Scope          = $null
            IsBreaking     = $false
            Description    = $Subject
        }
    }

    $isBreaking = $m.Groups['breaking'].Success
    if (-not $isBreaking -and $Body -and ($Body -match '(?m)^BREAKING CHANGE:')) {
        $isBreaking = $true
    }

    return [PSCustomObject]@{
        IsConventional = $true
        Type           = $m.Groups['type'].Value.ToLowerInvariant()
        Scope          = if ($m.Groups['scope'].Success) { $m.Groups['scope'].Value } else { $null }
        IsBreaking     = $isBreaking
        Description    = $m.Groups['desc'].Value
    }
}

function Get-NervBumpKind {
    <#
    .SYNOPSIS
        Reduces a list of commits (as returned by Get-NervCommitsSince) to
        one bump kind: `major` | `minor` | `patch` | `none`. Highest wins
        across all commits.
    #>
    [CmdletBinding()]
    param(
        [AllowNull()][object[]]$Commits
    )

    $rank = @{ none = 0; patch = 1; minor = 2; major = 3 }
    $best = 'none'

    # `@($Commits)` alone would wrap a $null $Commits into a one-element
    # array containing $null (a well-known PowerShell array-unwrapping
    # pitfall), so filter truthy elements instead of just re-wrapping.
    foreach ($c in @($Commits | Where-Object { $_ })) {
        $info = Get-NervConventionalCommitInfo -Subject $c.Subject -Body $c.Body

        $kind = 'none'
        if ($info.IsBreaking) {
            $kind = 'major'
        }
        elseif ($info.IsConventional) {
            switch ($info.Type) {
                'feat' { $kind = 'minor' }
                'fix' { $kind = 'patch' }
                'perf' { $kind = 'patch' }
                default { $kind = 'none' }
            }
        }

        if ($rank[$kind] -gt $rank[$best]) {
            $best = $kind
        }
    }

    return $best
}

function Get-NervNextPreReleaseNumber {
    <#
    .SYNOPSIS
        1 + the highest existing `v<Base>-<Label>.N` tag in $ExistingTags
        for that exact base version and label (other labels, or other
        base versions, never interfere).
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Base,
        [Parameter(Mandatory)][string]$Label,
        [string[]]$ExistingTags = @()
    )

    $maxN = 0
    $escapedBase = [regex]::Escape($Base)
    $escapedLabel = [regex]::Escape($Label)
    foreach ($t in @($ExistingTags | Where-Object { $_ })) {
        if ($t -match "^v$escapedBase-$escapedLabel\.(\d+)$") {
            $n = [int]$Matches[1]
            if ($n -gt $maxN) { $maxN = $n }
        }
    }
    return $maxN + 1
}

function Get-NervNextVersion {
    <#
    .SYNOPSIS
        Computes the next semver string from $Current + $Bump. Returns
        $null when $Bump is 'none' (unless -PreReleaseBase 'current' is
        given, which never requires a releasable bump).

    .PARAMETER PreRelease
        Pre-release label (e.g. `rc`; must match `^[a-z]+$`, or this
        throws). When given, the result is `<base>-<PreRelease>.N`, where
        N is one more than the highest existing `v<base>-<PreRelease>.N`
        tag in -ExistingTags for that same base version (or 1 when none
        exists). Other labels never interfere with the count.

    .PARAMETER PreReleaseBase
        Only meaningful together with -PreRelease. `next` (default): the
        base is $Current bumped by $Bump (today's behavior). `current`:
        the base is $Current as-is, ignoring $Bump entirely -- so this
        never requires a releasable commit.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Current,
        [Parameter(Mandatory)][ValidateSet('major', 'minor', 'patch', 'none')][string]$Bump,
        [string]$PreRelease,
        [ValidateSet('next', 'current')][string]$PreReleaseBase = 'next',
        [string[]]$ExistingTags = @()
    )

    if ($PreRelease -and $PreRelease -cnotmatch '^[a-z]+$') {
        throw "Invalid -PreRelease label '$PreRelease'; expected lowercase letters only (e.g. 'alpha', 'beta', 'rc')."
    }

    if ($PreRelease -and $PreReleaseBase -eq 'current') {
        $currentMatch = [regex]::Match($Current, '^(\d+)\.(\d+)\.(\d+)')
        if (-not $currentMatch.Success) {
            throw "Current version '$Current' is not valid semver."
        }
        $currentBase = "$($currentMatch.Groups[1].Value).$($currentMatch.Groups[2].Value).$($currentMatch.Groups[3].Value)"
        $n = Get-NervNextPreReleaseNumber -Base $currentBase -Label $PreRelease -ExistingTags $ExistingTags
        return "$currentBase-$PreRelease.$n"
    }

    if ($Bump -eq 'none') {
        return $null
    }

    $baseMatch = [regex]::Match($Current, '^(\d+)\.(\d+)\.(\d+)')
    if (-not $baseMatch.Success) {
        throw "Current version '$Current' is not valid semver."
    }

    $major = [int]$baseMatch.Groups[1].Value
    $minor = [int]$baseMatch.Groups[2].Value
    $patch = [int]$baseMatch.Groups[3].Value

    switch ($Bump) {
        'major' { $major++; $minor = 0; $patch = 0 }
        'minor' { $minor++; $patch = 0 }
        'patch' { $patch++ }
    }

    $bumpedBase = "$major.$minor.$patch"

    if ($PreRelease) {
        $n = Get-NervNextPreReleaseNumber -Base $bumpedBase -Label $PreRelease -ExistingTags $ExistingTags
        return "$bumpedBase-$PreRelease.$n"
    }

    return $bumpedBase
}

function Set-NervPluginVersion {
    <#
    .SYNOPSIS
        Rewrites only the `"version": "..."` value inside a plugin.json
        file, in place, via a targeted regex substitution -- never a
        JSON parse/reserialize -- so every other byte of the file
        (indentation, key order, EOL style, absence of a BOM) survives
        untouched.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Version
    )

    $raw = [System.IO.File]::ReadAllText($Path)
    $pattern = [regex]::new('("version"\s*:\s*")([^"]*)(")')
    $match = $pattern.Match($raw)
    if (-not $match.Success) {
        throw "No `"version`" field found in '$Path'."
    }

    $replacement = '${1}' + $Version + '${3}'
    $newText = $pattern.Replace($raw, $replacement, 1)

    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $newText, $utf8NoBom)
}

function New-NervChangelogSection {
    <#
    .SYNOPSIS
        Builds one Keep-a-Changelog section for a version from its
        commits: `## [X.Y.Z] - YYYY-MM-DD`, then only the non-empty
        groups in order `### Breaking`, `### Added` (feat), `### Changed`
        (refactor, perf), `### Fixed` (fix). Each line is
        `- <scope: ><description> (<shortSha>)`. Commits whose type is
        `docs`/`chore`/`test`/`ci`/`build`/`style`/`revert`, or that are
        not Conventional Commits, are omitted (unless breaking, in which
        case they land in `### Breaking` regardless of type).
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Version,
        [Parameter(Mandatory)][string]$Date,
        [AllowNull()][object[]]$Commits
    )

    $groups = [ordered]@{
        'Breaking' = [System.Collections.Generic.List[string]]::new()
        'Added'    = [System.Collections.Generic.List[string]]::new()
        'Changed'  = [System.Collections.Generic.List[string]]::new()
        'Fixed'    = [System.Collections.Generic.List[string]]::new()
    }

    # See the same-shaped comment in Get-NervBumpKind about why a plain
    # `@($Commits)` is unsafe when $Commits is $null.
    foreach ($c in @($Commits | Where-Object { $_ })) {
        $info = Get-NervConventionalCommitInfo -Subject $c.Subject -Body $c.Body
        $line = if ($info.Scope) { "- $($info.Scope): $($info.Description) ($($c.ShortSha))" } else { "- $($info.Description) ($($c.ShortSha))" }

        if ($info.IsBreaking) {
            $groups['Breaking'].Add($line)
            continue
        }
        if (-not $info.IsConventional) { continue }

        switch ($info.Type) {
            'feat' { $groups['Added'].Add($line) }
            'refactor' { $groups['Changed'].Add($line) }
            'perf' { $groups['Changed'].Add($line) }
            'fix' { $groups['Fixed'].Add($line) }
            default { }
        }
    }

    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.Add("## [$Version] - $Date")

    foreach ($groupName in @('Breaking', 'Added', 'Changed', 'Fixed')) {
        if ($groups[$groupName].Count -gt 0) {
            $lines.Add('')
            $lines.Add("### $groupName")
            foreach ($l in $groups[$groupName]) { $lines.Add($l) }
        }
    }

    return (($lines -join "`n") + "`n")
}

function Update-NervChangelog {
    <#
    .SYNOPSIS
        Inserts $Section (as produced by New-NervChangelogSection) right
        after the `## [Unreleased]` heading block of CHANGELOG.md at
        $Path -- so newest-applied sections stay on top, right under
        Unreleased, above any older version sections. Creates the file
        with a standard Keep-a-Changelog header (and an empty
        `## [Unreleased]` section) when it does not exist yet. Returns
        $false without writing anything when a section for the same
        version already exists (idempotent).
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Section
    )

    $versionMatch = [regex]::Match($Section, '^## \[(?<v>[^\]]+)\]')
    if (-not $versionMatch.Success) {
        throw "Section text must start with a '## [X.Y.Z]' heading."
    }
    $version = $versionMatch.Groups['v'].Value
    $escapedVersion = [regex]::Escape($version)

    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)

    if (-not (Test-Path -LiteralPath $Path)) {
        $standardHeader = "# Changelog`n`n" +
        "All notable changes to this project will be documented in this file.`n`n" +
        "The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),`n" +
        "and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).`n`n" +
        "## [Unreleased]`n"
        [System.IO.File]::WriteAllText($Path, $standardHeader, $utf8NoBom)
    }

    $text = [System.IO.File]::ReadAllText($Path)

    if ($text -match "(?m)^## \[$escapedVersion\](\s|$)") {
        return $false
    }

    $unreleasedMatch = [regex]::Match($text, '(?m)^## \[Unreleased\][^\r\n]*')
    if (-not $unreleasedMatch.Success) {
        throw "No '## [Unreleased]' heading found in '$Path'."
    }

    $afterHeadingIndex = $unreleasedMatch.Index + $unreleasedMatch.Length
    $rest = $text.Substring($afterHeadingIndex)
    $nextHeadingMatch = [regex]::Match($rest, '(?m)^## \[')
    $insertIndex = if ($nextHeadingMatch.Success) { $afterHeadingIndex + $nextHeadingMatch.Index } else { $text.Length }

    $before = $text.Substring(0, $insertIndex).TrimEnd("`n")
    $after = $text.Substring($insertIndex).TrimStart("`n")

    $sectionBlock = $Section.Trim("`n")

    $newText = $before + "`n`n" + $sectionBlock + "`n`n" + $after
    if (-not $newText.EndsWith("`n")) { $newText += "`n" }

    [System.IO.File]::WriteAllText($Path, $newText, $utf8NoBom)
    return $true
}

# =============================================================================
# CLI body. Guarded so dot-sourcing (tests) only defines the functions above
# and never runs the CLI -- same pattern as tools/configure.ps1.
# =============================================================================
if ($MyInvocation.InvocationName -ne '.') {

    $ErrorActionPreference = "Stop"

    try {
        if ($PreReleaseBase -ne 'next' -and -not $PreRelease) {
            Write-Host "-PreReleaseBase requires -PreRelease; it has no meaning on its own."
            exit 1
        }

        if ($PreRelease -and $PreRelease -cnotmatch '^[a-z]+$') {
            Write-Host "Invalid -PreRelease label '$PreRelease'; expected lowercase letters only (e.g. 'alpha', 'beta', 'rc')."
            exit 1
        }

        if ($PreRelease -and $Apply) {
            Write-Host "-Apply cannot be combined with -PreRelease: pre-releases are tag-only and never modify plugin.json or CHANGELOG.md."
            exit 1
        }

        if (-not (Test-Path -LiteralPath $RepoPath)) {
            Write-Host "Repository path not found: '$RepoPath'."
            exit 2
        }
        $repoPathResolved = (Resolve-Path -LiteralPath $RepoPath).ProviderPath

        $pluginJsonPath = Join-Path $repoPathResolved 'plugin/.claude-plugin/plugin.json'
        $changelogPath = Join-Path $repoPathResolved 'CHANGELOG.md'

        if (-not (Test-Path -LiteralPath $pluginJsonPath)) {
            Write-Host "plugin.json not found at '$pluginJsonPath'."
            exit 2
        }

        $currentVersion = Get-NervPluginVersion -Path $pluginJsonPath

        $lastTag = Get-NervLastReleaseTag -RepoPath $repoPathResolved
        $commits = @(Get-NervCommitsSince -RepoPath $repoPathResolved -Tag $lastTag)
        $bumpKind = Get-NervBumpKind -Commits $commits

        $nextVersion = $null

        if ($Version) {
            if ($Version -notmatch '^\d+\.\d+\.\d+$') {
                Write-Host "Invalid -Version '$Version'; expected semver X.Y.Z."
                exit 1
            }

            $curParts = @($currentVersion -split '\.' | ForEach-Object { [int]$_ })
            $newParts = @($Version -split '\.' | ForEach-Object { [int]$_ })
            $isGreater = $false
            for ($i = 0; $i -lt 3; $i++) {
                if ($newParts[$i] -gt $curParts[$i]) { $isGreater = $true; break }
                if ($newParts[$i] -lt $curParts[$i]) { $isGreater = $false; break }
            }
            if (-not $isGreater) {
                Write-Host "-Version '$Version' must be strictly greater than the current version '$currentVersion'."
                exit 1
            }

            $nextVersion = $Version
        }
        elseif ($PreRelease) {
            $global:LASTEXITCODE = 0
            $existingTags = & git -C $repoPathResolved tag -l 'v*' 2>$null
            if ($LASTEXITCODE -ne 0) {
                throw "git tag failed in '$repoPathResolved' (exit $LASTEXITCODE)."
            }
            $nextVersion = Get-NervNextVersion -Current $currentVersion -Bump $bumpKind -PreRelease $PreRelease -PreReleaseBase $PreReleaseBase -ExistingTags @($existingTags)
        }
        else {
            $nextVersion = Get-NervNextVersion -Current $currentVersion -Bump $bumpKind
        }

        $dateStr = (Get-Date).ToString('yyyy-MM-dd')
        $section = if ($nextVersion) { New-NervChangelogSection -Version $nextVersion -Date $dateStr -Commits $commits } else { $null }

        $commitsPayload = @($commits | ForEach-Object {
                [ordered]@{ sha = $_.Sha; short_sha = $_.ShortSha; subject = $_.Subject; body = $_.Body }
            })

        $tagValue = if ($nextVersion) { "v$nextVersion" } else { $null }
        $baseVersion = if ($nextVersion) { ([regex]::Match($nextVersion, '^(\d+\.\d+\.\d+)')).Groups[1].Value } else { $null }

        $result = [ordered]@{
            current    = $currentVersion
            last_tag   = $lastTag
            bump       = $bumpKind
            next       = $nextVersion
            tag        = $tagValue
            base       = $baseVersion
            prerelease = if ($PreRelease) { $PreRelease } else { $null }
            commits    = $commitsPayload
            section    = $section
            applied    = $false
        }

        if (-not $nextVersion) {
            if ($Json) {
                ConvertTo-Json -InputObject $result -Depth 10
            }
            else {
                $sinceDescription = if ($lastTag) { $lastTag } else { "the repository root" }
                Write-Host "Nothing to release: $($commits.Count) commit(s) inspected since $sinceDescription, none releasable."
            }
            exit 1
        }

        if ($Apply) {
            Set-NervPluginVersion -Path $pluginJsonPath -Version $nextVersion
            Update-NervChangelog -Path $changelogPath -Section $section | Out-Null
            $result.applied = $true
            $result.written = @($pluginJsonPath, $changelogPath)
        }

        if ($Json) {
            ConvertTo-Json -InputObject $result -Depth 10
        }
        else {
            Write-Host "Current version : $currentVersion"
            Write-Host "Last tag        : $(if ($lastTag) { $lastTag } else { '(none)' })"
            Write-Host "Bump            : $bumpKind"
            Write-Host "Next version    : $nextVersion"
            Write-Host "Tag             : $(if ($tagValue) { $tagValue } else { '(none)' })"
            if ($PreRelease) { Write-Host "Pre-release     : $PreRelease" }
            Write-Host ""
            Write-Host $section
            if ($Apply) {
                Write-Host "Applied: wrote $pluginJsonPath and $changelogPath"
            }
        }

        exit 0
    }
    catch {
        Write-Host "Unexpected error: $_"
        exit 2
    }
}
