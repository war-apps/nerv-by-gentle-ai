#requires -Version 7
<#
.SYNOPSIS
    Assertions for tools/release.ps1: the pure functions
    (Get-NervPluginVersion, Get-NervLastReleaseTag, Get-NervCommitsSince,
    Get-NervConventionalCommitInfo, Get-NervBumpKind, Get-NervNextVersion,
    Set-NervPluginVersion, New-NervChangelogSection, Update-NervChangelog)
    dot-sourced behind the same interactive-guard pattern as
    tools/configure.ps1, plus end-to-end child-process runs of the CLI
    (-Preview, -Apply, -Version, -PreRelease, -Json) against real
    temporary git repositories. No Pester -- prints PASS/FAIL lines and
    exits 1 on any failure, matching tests/configure.test.ps1's style.

    Fixtures are real git repositories built with `git init`, `git
    commit`, and `git tag` -- git is never mocked.

.EXAMPLE
    pwsh -NoProfile -File tests/release.test.ps1
#>

$ErrorActionPreference = "Stop"

$selfDir = $PSScriptRoot
$repoRoot = Split-Path -Parent $selfDir
$scriptPath = Join-Path $repoRoot "tools/release.ps1"

$script:passCount = 0
$script:failCount = 0

function Report {
    param([string]$CaseName, [bool]$Ok, [string]$Detail = "")
    if ($Ok) {
        Write-Host "PASS $CaseName"
        $script:passCount++
    }
    else {
        $suffix = if ($Detail) { " - $Detail" } else { "" }
        Write-Host "FAIL $CaseName$suffix"
        $script:failCount++
    }
}

# ---------------------------------------------------------------------------
# Dot-source the script. Its CLI body is guarded by
# `if ($MyInvocation.InvocationName -ne '.')` (same pattern as
# tools/configure.ps1), so dot-sourcing only defines functions and never
# writes, tags, or exits for real.
# ---------------------------------------------------------------------------
if (-not (Test-Path -LiteralPath $scriptPath)) {
    Report "release-script-exists" $false "tools/release.ps1 not found (not implemented yet)"
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    exit 1
}
Report "release-script-exists" $true

try {
    . $scriptPath -RepoPath $repoRoot 2>$null
}
catch {
    # Expected during RED (before the CLI body is guarded); the real signal
    # is whether the functions got defined below.
}

$requiredFunctions = @(
    'Get-NervPluginVersion',
    'Get-NervLastReleaseTag',
    'Get-NervCommitsSince',
    'Get-NervConventionalCommitInfo',
    'Get-NervBumpKind',
    'Get-NervNextVersion',
    'Set-NervPluginVersion',
    'New-NervChangelogSection',
    'Update-NervChangelog'
)
$allDefined = $true
foreach ($fn in $requiredFunctions) {
    if (-not (Get-Command $fn -ErrorAction SilentlyContinue)) { $allDefined = $false }
}

if (-not $allDefined) {
    Report "functions-defined-after-dot-source" $false "one or more pure functions not found (tools/release.ps1 not implemented yet)"
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    exit 1
}
Report "functions-defined-after-dot-source" $true

$pwshExe = (Get-Process -Id $PID).Path
if (-not $pwshExe -or -not (Test-Path -LiteralPath $pwshExe)) { $pwshExe = 'pwsh' }
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)

# ---------------------------------------------------------------------------
# Fixture helpers -- real temporary git repositories, never mocked.
# ---------------------------------------------------------------------------
function New-ReleaseFixtureRepo {
    param(
        [string]$Version = '0.1.0',
        [switch]$WithChangelog,
        [switch]$SkipGitInit
    )
    $root = Join-Path ([IO.Path]::GetTempPath()) ("nerv-release-test-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $root -Force | Out-Null

    if (-not $SkipGitInit) {
        & git init -q -b main $root 2>$null | Out-Null
    }

    $pluginDir = Join-Path $root "plugin/.claude-plugin"
    New-Item -ItemType Directory -Path $pluginDir -Force | Out-Null
    $pluginJsonPath = Join-Path $pluginDir "plugin.json"
    $pluginJsonContent = "{`n  `"name`": `"nerv-release-fixture`",`n  `"version`": `"$Version`",`n  `"description`": `"Fixture plugin for release.ps1 tests`",`n  `"author`": { `"name`": `"Test`" }`n}`n"
    [System.IO.File]::WriteAllText($pluginJsonPath, $pluginJsonContent, $utf8NoBom)

    if ($WithChangelog) {
        $changelogContent = "# Changelog`n`nAll notable changes to this project will be documented in this file.`n`n## [Unreleased]`n"
        [System.IO.File]::WriteAllText((Join-Path $root "CHANGELOG.md"), $changelogContent, $utf8NoBom)
    }

    if (-not $SkipGitInit) {
        & git -C $root add -A 2>$null | Out-Null
        & git -C $root -c user.name=t -c user.email=t@t commit -q -m "chore: initial fixture commit" 2>$null | Out-Null
    }

    return $root
}

function Add-ReleaseFixtureCommit {
    param([Parameter(Mandatory)][string]$RepoPath, [Parameter(Mandatory)][string]$Message, [switch]$AllowEmpty = $true)
    $msgFile = [System.IO.Path]::GetTempFileName()
    try {
        [System.IO.File]::WriteAllText($msgFile, $Message, $utf8NoBom)
        & git -C $RepoPath -c user.name=t -c user.email=t@t commit -q --allow-empty -F $msgFile 2>$null | Out-Null
    }
    finally {
        Remove-Item -LiteralPath $msgFile -Force -ErrorAction SilentlyContinue
    }
    (& git -C $RepoPath rev-parse HEAD).Trim()
}

function Add-ReleaseFixtureTag {
    param([Parameter(Mandatory)][string]$RepoPath, [Parameter(Mandatory)][string]$Tag)
    & git -C $RepoPath tag $Tag 2>$null | Out-Null
}

function Remove-FixtureRepo {
    param([string]$RepoPath)
    if ($RepoPath -and (Test-Path -LiteralPath $RepoPath)) {
        Remove-Item -LiteralPath $RepoPath -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ---------------------------------------------------------------------------
# Case group A: Get-NervLastReleaseTag
# ---------------------------------------------------------------------------
$repoA = New-ReleaseFixtureRepo
try {
    Report "lastreleasetag-none-returns-null" ($null -eq (Get-NervLastReleaseTag -RepoPath $repoA))

    Add-ReleaseFixtureTag -RepoPath $repoA -Tag 'v0.9.0'
    Add-ReleaseFixtureTag -RepoPath $repoA -Tag 'v0.10.0'
    Report "lastreleasetag-semver-order-not-lexical" ((Get-NervLastReleaseTag -RepoPath $repoA) -eq 'v0.10.0')

    Add-ReleaseFixtureTag -RepoPath $repoA -Tag 'v1.0.0-rc.1'
    Report "lastreleasetag-prerelease-ignored-by-default" ((Get-NervLastReleaseTag -RepoPath $repoA) -eq 'v0.10.0')
    Report "lastreleasetag-prerelease-included-when-requested" ((Get-NervLastReleaseTag -RepoPath $repoA -IncludePreRelease) -eq 'v1.0.0-rc.1')

    Add-ReleaseFixtureTag -RepoPath $repoA -Tag 'v1.0.0'
    Report "lastreleasetag-stable-outranks-prerelease-of-same-base" ((Get-NervLastReleaseTag -RepoPath $repoA -IncludePreRelease) -eq 'v1.0.0')
}
finally {
    Remove-FixtureRepo $repoA
}

# ---------------------------------------------------------------------------
# Case group B: Get-NervCommitsSince
# ---------------------------------------------------------------------------
$repoB = New-ReleaseFixtureRepo
try {
    Add-ReleaseFixtureCommit -RepoPath $repoB -Message "feat: first feature" | Out-Null
    Add-ReleaseFixtureCommit -RepoPath $repoB -Message "fix: first fix" | Out-Null
    $tagShaB = (& git -C $repoB rev-parse HEAD).Trim()
    Add-ReleaseFixtureTag -RepoPath $repoB -Tag 'v0.2.0'
    Add-ReleaseFixtureCommit -RepoPath $repoB -Message "feat: second feature" | Out-Null
    Add-ReleaseFixtureCommit -RepoPath $repoB -Message "fix: second fix" | Out-Null

    $allCommits = @(Get-NervCommitsSince -RepoPath $repoB -Tag $null)
    Report "commitssince-whole-history-count" ($allCommits.Count -eq 5) "got $($allCommits.Count)"
    Report "commitssince-oldest-first" ($allCommits[0].Subject -eq 'chore: initial fixture commit' -and $allCommits[-1].Subject -eq 'fix: second fix')

    $sinceTagCommits = @(Get-NervCommitsSince -RepoPath $repoB -Tag 'v0.2.0')
    Report "commitssince-tag-range-count" ($sinceTagCommits.Count -eq 2) "got $($sinceTagCommits.Count)"
    Report "commitssince-tag-range-subjects" (
        $sinceTagCommits.Count -eq 2 -and
        $sinceTagCommits[0].Subject -eq 'feat: second feature' -and
        $sinceTagCommits[1].Subject -eq 'fix: second fix'
    )
    Report "commitssince-has-sha-and-shortsha" (
        $sinceTagCommits.Count -ge 1 -and
        $sinceTagCommits[0].Sha.Length -eq 40 -and
        $sinceTagCommits[0].ShortSha.Length -ge 7 -and
        $sinceTagCommits[0].Sha.StartsWith($sinceTagCommits[0].ShortSha)
    )

    # merge-commit exclusion
    & git -C $repoB checkout -q -b feature-branch 2>$null | Out-Null
    Add-ReleaseFixtureCommit -RepoPath $repoB -Message "feat: branch work" | Out-Null
    & git -C $repoB checkout -q main 2>$null | Out-Null
    Add-ReleaseFixtureCommit -RepoPath $repoB -Message "fix: main work" | Out-Null
    & git -C $repoB -c user.name=t -c user.email=t@t merge -q --no-ff -m "merge: bring in feature-branch" feature-branch 2>$null | Out-Null

    $afterMergeCommits = @(Get-NervCommitsSince -RepoPath $repoB -Tag 'v0.2.0')
    $mergeSubjectPresent = @($afterMergeCommits | Where-Object { $_.Subject -eq 'merge: bring in feature-branch' }).Count -gt 0
    Report "commitssince-excludes-merge-commits" (-not $mergeSubjectPresent)
    Report "commitssince-includes-non-merge-commits-from-both-branches" (
        (@($afterMergeCommits | Where-Object { $_.Subject -eq 'feat: branch work' }).Count -eq 1) -and
        (@($afterMergeCommits | Where-Object { $_.Subject -eq 'fix: main work' }).Count -eq 1)
    )
}
finally {
    Remove-FixtureRepo $repoB
}

# ---------------------------------------------------------------------------
# Case group C: Get-NervBumpKind
# ---------------------------------------------------------------------------
function C([string]$Subject, [string]$Body = '') {
    return [PSCustomObject]@{ Sha = 'deadbeef'; ShortSha = 'deadbee'; Subject = $Subject; Body = $Body }
}

Report "bumpkind-feat-is-minor" ((Get-NervBumpKind -Commits @((C 'feat: add thing'))) -eq 'minor')
Report "bumpkind-fix-is-patch" ((Get-NervBumpKind -Commits @((C 'fix: fix thing'))) -eq 'patch')
Report "bumpkind-perf-is-patch" ((Get-NervBumpKind -Commits @((C 'perf: speed up thing'))) -eq 'patch')
Report "bumpkind-feat-bang-is-major" ((Get-NervBumpKind -Commits @((C 'feat!: drop old api'))) -eq 'major')
Report "bumpkind-fix-scope-bang-is-major" ((Get-NervBumpKind -Commits @((C 'fix(parser)!: change contract'))) -eq 'major')
Report "bumpkind-breaking-change-footer-is-major" ((Get-NervBumpKind -Commits @((C 'fix: adjust parsing' "Body text.`n`nBREAKING CHANGE: input format changed"))) -eq 'major')

foreach ($t in @('docs', 'chore', 'test', 'refactor', 'ci', 'build', 'style', 'revert')) {
    Report "bumpkind-$t-is-none" ((Get-NervBumpKind -Commits @((C "${t}: some change"))) -eq 'none')
}

Report "bumpkind-non-conventional-is-none" ((Get-NervBumpKind -Commits @((C 'update the readme quickly'))) -eq 'none')
Report "bumpkind-empty-commits-is-none" ((Get-NervBumpKind -Commits @()) -eq 'none')
Report "bumpkind-highest-wins-feat-and-fix" ((Get-NervBumpKind -Commits @((C 'feat: a'), (C 'fix: b'))) -eq 'minor')
Report "bumpkind-highest-wins-feat-and-breaking" ((Get-NervBumpKind -Commits @((C 'feat: a'), (C 'feat!: b'))) -eq 'major')
Report "bumpkind-highest-wins-docs-and-fix" ((Get-NervBumpKind -Commits @((C 'docs: a'), (C 'fix: b'))) -eq 'patch')

# ---------------------------------------------------------------------------
# Case group D: Get-NervNextVersion
# ---------------------------------------------------------------------------
Report "nextversion-major-bump" ((Get-NervNextVersion -Current '1.2.3' -Bump 'major') -eq '2.0.0')
Report "nextversion-minor-bump" ((Get-NervNextVersion -Current '1.2.3' -Bump 'minor') -eq '1.3.0')
Report "nextversion-patch-bump" ((Get-NervNextVersion -Current '1.2.3' -Bump 'patch') -eq '1.2.4')
Report "nextversion-none-bump-is-null" ($null -eq (Get-NervNextVersion -Current '1.2.3' -Bump 'none'))
Report "nextversion-prerelease-first-number" ((Get-NervNextVersion -Current '1.2.3' -Bump 'minor' -PreRelease 'rc' -ExistingTags @()) -eq '1.3.0-rc.1')
Report "nextversion-prerelease-next-number" (
    (Get-NervNextVersion -Current '1.2.3' -Bump 'minor' -PreRelease 'rc' -ExistingTags @('v1.3.0-rc.1', 'v1.3.0-rc.2', 'v1.2.0-rc.5')) -eq '1.3.0-rc.3'
)

# ---------------------------------------------------------------------------
# Case group E: Set-NervPluginVersion
# ---------------------------------------------------------------------------
$repoE = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    $pluginJsonPathE = Join-Path $repoE "plugin/.claude-plugin/plugin.json"
    $beforeLines = @([System.IO.File]::ReadAllLines($pluginJsonPathE))
    $beforeRaw = [System.IO.File]::ReadAllText($pluginJsonPathE)

    Set-NervPluginVersion -Path $pluginJsonPathE -Version '0.2.0'

    $afterLines = @([System.IO.File]::ReadAllLines($pluginJsonPathE))
    $afterRaw = [System.IO.File]::ReadAllText($pluginJsonPathE)

    Report "setpluginversion-same-line-count" ($beforeLines.Count -eq $afterLines.Count)

    $diffIndexes = @()
    for ($i = 0; $i -lt $beforeLines.Count; $i++) {
        if ($beforeLines[$i] -cne $afterLines[$i]) { $diffIndexes += $i }
    }
    Report "setpluginversion-exactly-one-line-differs" ($diffIndexes.Count -eq 1) "diff indexes: $($diffIndexes -join ',')"
    Report "setpluginversion-changed-line-has-new-version" (
        $diffIndexes.Count -eq 1 -and $afterLines[$diffIndexes[0]] -match '"version":\s*"0\.2\.0"'
    )

    Report "setpluginversion-no-bom" (
        [System.IO.File]::ReadAllBytes($pluginJsonPathE)[0] -ne 0xEF
    )
    Report "setpluginversion-lf-only" (-not $afterRaw.Contains("`r"))
    Report "setpluginversion-ends-with-newline" ($afterRaw.EndsWith("`n"))

    # rest of the document byte-identical: rejoin all lines except the
    # differing one and compare against the original with the same line
    # excluded.
    $beforeMinus = @($beforeLines | Where-Object { $beforeLines.IndexOf($_) -ne $diffIndexes[0] })
    $restIdentical = $true
    for ($i = 0; $i -lt $beforeLines.Count; $i++) {
        if ($i -eq $diffIndexes[0]) { continue }
        if ($beforeLines[$i] -cne $afterLines[$i]) { $restIdentical = $false }
    }
    Report "setpluginversion-rest-of-file-byte-identical" $restIdentical
}
finally {
    Remove-FixtureRepo $repoE
}

# ---------------------------------------------------------------------------
# Case group F: New-NervChangelogSection
# ---------------------------------------------------------------------------
$fCommits = @(
    (C 'feat(cli): add --json flag'),
    (C 'fix: handle null path'),
    (C 'docs: update readme'),
    (C 'refactor: simplify parser'),
    (C 'perf: speed up load'),
    (C 'feat!: drop legacy mode')
)
$sectionF = New-NervChangelogSection -Version '1.2.0' -Date '2026-09-28' -Commits $fCommits

Report "changelogsection-starts-with-heading" ($sectionF.StartsWith("## [1.2.0] - 2026-09-28"))
Report "changelogsection-group-order" (
    $sectionF.IndexOf('### Breaking') -lt $sectionF.IndexOf('### Added') -and
    $sectionF.IndexOf('### Added') -lt $sectionF.IndexOf('### Changed') -and
    $sectionF.IndexOf('### Changed') -lt $sectionF.IndexOf('### Fixed')
)
Report "changelogsection-breaking-has-drop-legacy" ($sectionF -match [regex]::Escape('- drop legacy mode (deadbee)'))
Report "changelogsection-added-has-scope-prefix" ($sectionF -match [regex]::Escape('- cli: add --json flag (deadbee)'))
Report "changelogsection-fixed-has-no-scope-prefix" ($sectionF -match [regex]::Escape('- handle null path (deadbee)'))
Report "changelogsection-changed-has-refactor-and-perf" (
    $sectionF -match [regex]::Escape('- simplify parser (deadbee)') -and
    $sectionF -match [regex]::Escape('- speed up load (deadbee)')
)
Report "changelogsection-omits-docs" ($sectionF -notmatch [regex]::Escape('update readme'))

$onlyDocsSection = New-NervChangelogSection -Version '1.2.1' -Date '2026-09-28' -Commits @((C 'docs: only docs change'))
Report "changelogsection-only-empty-groups-omitted-entirely" (
    $onlyDocsSection.StartsWith("## [1.2.1] - 2026-09-28") -and
    $onlyDocsSection -notmatch '###'
)

# ---------------------------------------------------------------------------
# Case group G: Update-NervChangelog
# ---------------------------------------------------------------------------
$tempRootG = Join-Path ([IO.Path]::GetTempPath()) ("nerv-release-test-changelog-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRootG -Force | Out-Null
try {
    $changelogPathG = Join-Path $tempRootG "CHANGELOG.md"

    $sectionG1 = New-NervChangelogSection -Version '1.0.0' -Date '2026-09-28' -Commits @((C 'feat: first release feature'))
    $resultG1 = Update-NervChangelog -Path $changelogPathG -Section $sectionG1
    Report "updatechangelog-creates-missing-file" (Test-Path -LiteralPath $changelogPathG)
    Report "updatechangelog-first-insert-returns-true" ($resultG1 -eq $true)

    $textG1 = [System.IO.File]::ReadAllText($changelogPathG)
    Report "updatechangelog-has-unreleased-heading" ($textG1 -match '(?m)^## \[Unreleased\]')
    Report "updatechangelog-section-inserted-after-unreleased" (
        $textG1.IndexOf('## [Unreleased]') -lt $textG1.IndexOf('## [1.0.0]')
    )
    Report "updatechangelog-no-bom-created-file" ([System.IO.File]::ReadAllBytes($changelogPathG)[0] -ne 0xEF)

    $sectionG2 = New-NervChangelogSection -Version '1.1.0' -Date '2026-09-29' -Commits @((C 'feat: second release feature'))
    $resultG2 = Update-NervChangelog -Path $changelogPathG -Section $sectionG2
    Report "updatechangelog-second-insert-returns-true" ($resultG2 -eq $true)

    $textG2 = [System.IO.File]::ReadAllText($changelogPathG)
    Report "updatechangelog-newest-on-top-under-unreleased" (
        $textG2.IndexOf('## [Unreleased]') -lt $textG2.IndexOf('## [1.1.0]') -and
        $textG2.IndexOf('## [1.1.0]') -lt $textG2.IndexOf('## [1.0.0]')
    )
    Report "updatechangelog-older-section-preserved" ($textG2 -match [regex]::Escape('first release feature'))

    # idempotence: inserting the same version twice is a no-op
    $textBeforeIdempotent = [System.IO.File]::ReadAllText($changelogPathG)
    $resultG3 = Update-NervChangelog -Path $changelogPathG -Section $sectionG2
    Report "updatechangelog-duplicate-version-returns-false" ($resultG3 -eq $false)
    $textAfterIdempotent = [System.IO.File]::ReadAllText($changelogPathG)
    Report "updatechangelog-duplicate-version-file-unchanged" ($textBeforeIdempotent -ceq $textAfterIdempotent)
}
finally {
    Remove-FixtureRepo $tempRootG
}

# ---------------------------------------------------------------------------
# Case group H: CLI end-to-end (child process), real temp git repos.
# ---------------------------------------------------------------------------

# -- H1: -Preview default text output on a repo with a feat commit --
$repoH1 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoH1 -Message "feat: add cool feature" | Out-Null

    $previewOutput = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH1 -Preview 2>&1
    $previewExit = $LASTEXITCODE
    $previewText = ($previewOutput -join "`n")

    Report "cli-preview-exit-zero" ($previewExit -eq 0) "exit $previewExit; output: $previewText"
    Report "cli-preview-shows-current-version" ($previewText -match [regex]::Escape('0.1.0'))
    Report "cli-preview-shows-next-version" ($previewText -match [regex]::Escape('0.2.0'))
    Report "cli-preview-shows-bump-minor" ($previewText -match 'Bump\s*:\s*minor')

    $pluginJsonAfterPreview = [System.IO.File]::ReadAllText((Join-Path $repoH1 "plugin/.claude-plugin/plugin.json"))
    Report "cli-preview-does-not-write-plugin-json" ($pluginJsonAfterPreview -match '"version":\s*"0\.1\.0"')
    Report "cli-preview-does-not-create-changelog" (-not (Test-Path -LiteralPath (Join-Path $repoH1 "CHANGELOG.md")))
}
finally {
    Remove-FixtureRepo $repoH1
}

# -- H2: -Preview -Json shape --
$repoH2 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoH2 -Message "fix: patch something" | Out-Null

    $jsonOutput = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH2 -Preview -Json
    $jsonExit = $LASTEXITCODE
    $parsed = $null
    try { $parsed = ($jsonOutput -join "`n") | ConvertFrom-Json } catch { $parsed = $null }

    Report "cli-preview-json-exit-zero" ($jsonExit -eq 0) "exit $jsonExit"
    Report "cli-preview-json-parses" ($null -ne $parsed)
    if ($parsed) {
        Report "cli-preview-json-current" ($parsed.current -eq '0.1.0')
        Report "cli-preview-json-next" ($parsed.next -eq '0.1.1')
        Report "cli-preview-json-bump" ($parsed.bump -eq 'patch')
        Report "cli-preview-json-applied-false" ($parsed.applied -eq $false)
        Report "cli-preview-json-has-section" (-not [string]::IsNullOrEmpty($parsed.section))
        Report "cli-preview-json-has-commits-array" (@($parsed.commits).Count -ge 1)
        Report "cli-preview-json-prerelease-null" ($null -eq $parsed.prerelease)
    }
    else {
        Report "cli-preview-json-current" $false "no parsed JSON"
        Report "cli-preview-json-next" $false "no parsed JSON"
        Report "cli-preview-json-bump" $false "no parsed JSON"
        Report "cli-preview-json-applied-false" $false "no parsed JSON"
        Report "cli-preview-json-has-section" $false "no parsed JSON"
        Report "cli-preview-json-has-commits-array" $false "no parsed JSON"
        Report "cli-preview-json-prerelease-null" $false "no parsed JSON"
    }
}
finally {
    Remove-FixtureRepo $repoH2
}

# -- H3: -Apply writes plugin.json (exactly one line) and CHANGELOG.md --
$repoH3 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoH3 -Message "feat: add applied feature" | Out-Null

    $pluginJsonPathH3 = Join-Path $repoH3 "plugin/.claude-plugin/plugin.json"
    $beforeApplyLines = @([System.IO.File]::ReadAllLines($pluginJsonPathH3))

    $applyOutput = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH3 -Apply -Json
    $applyExit = $LASTEXITCODE
    $applyParsed = $null
    try { $applyParsed = ($applyOutput -join "`n") | ConvertFrom-Json } catch { $applyParsed = $null }

    Report "cli-apply-exit-zero" ($applyExit -eq 0) "exit $applyExit"
    Report "cli-apply-json-applied-true" ($null -ne $applyParsed -and $applyParsed.applied -eq $true)
    Report "cli-apply-json-has-written" ($null -ne $applyParsed -and @($applyParsed.written).Count -eq 2)

    $afterApplyLines = @([System.IO.File]::ReadAllLines($pluginJsonPathH3))
    Report "cli-apply-same-line-count" ($beforeApplyLines.Count -eq $afterApplyLines.Count)
    $applyDiffCount = 0
    for ($i = 0; $i -lt $beforeApplyLines.Count; $i++) {
        if ($beforeApplyLines[$i] -cne $afterApplyLines[$i]) { $applyDiffCount++ }
    }
    Report "cli-apply-exactly-one-line-changed" ($applyDiffCount -eq 1) "diff count $applyDiffCount"

    $changelogPathH3 = Join-Path $repoH3 "CHANGELOG.md"
    Report "cli-apply-creates-changelog" (Test-Path -LiteralPath $changelogPathH3)
    $changelogTextH3 = if (Test-Path -LiteralPath $changelogPathH3) { [System.IO.File]::ReadAllText($changelogPathH3) } else { '' }
    Report "cli-apply-changelog-has-new-section" ($changelogTextH3 -match [regex]::Escape('## [0.2.0]'))
    Report "cli-apply-changelog-has-commit-line" ($changelogTextH3 -match [regex]::Escape('add applied feature'))

    # -- H3b: tag the applied version (as CI would after merging to main),
    #    then run -Apply again with no new commits -> nothing to release.
    #    (This is the realistic idempotence scenario: tools/release.ps1
    #    never creates the tag itself -- see the deviation note in the
    #    task report about the "no-op" acceptance wording.) --
    Add-ReleaseFixtureTag -RepoPath $repoH3 -Tag 'v0.2.0'

    $afterTagLinesBefore = @([System.IO.File]::ReadAllLines($pluginJsonPathH3))
    $changelogBeforeSecondApply = [System.IO.File]::ReadAllText($changelogPathH3)

    $secondApplyOutput = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH3 -Apply -Json 2>&1
    $secondApplyExit = $LASTEXITCODE

    Report "cli-apply-again-after-tag-nothing-to-release-exit-one" ($secondApplyExit -eq 1) "exit $secondApplyExit"

    $afterTagLinesAfter = @([System.IO.File]::ReadAllLines($pluginJsonPathH3))
    $changelogAfterSecondApply = [System.IO.File]::ReadAllText($changelogPathH3)
    Report "cli-apply-again-after-tag-plugin-json-unchanged" (($afterTagLinesBefore -join "`n") -ceq ($afterTagLinesAfter -join "`n"))
    Report "cli-apply-again-after-tag-changelog-unchanged" ($changelogBeforeSecondApply -ceq $changelogAfterSecondApply)
}
finally {
    Remove-FixtureRepo $repoH3
}

# -- H4: -Version override succeeds even when bump would be none --
$repoH4 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoH4 -Message "docs: update readme only" | Out-Null

    $versionOutput = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH4 -Version '1.0.0' -Json
    $versionExit = $LASTEXITCODE
    $versionParsed = $null
    try { $versionParsed = ($versionOutput -join "`n") | ConvertFrom-Json } catch { $versionParsed = $null }

    Report "cli-version-override-exit-zero" ($versionExit -eq 0) "exit $versionExit"
    Report "cli-version-override-next-is-explicit" ($null -ne $versionParsed -and $versionParsed.next -eq '1.0.0')
    Report "cli-version-override-bump-still-reported" ($null -ne $versionParsed -and $versionParsed.bump -eq 'none')
}
finally {
    Remove-FixtureRepo $repoH4
}

# -- H5: -Version invalid semver --
$repoH5 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH5 -Version 'not-a-version' 2>&1 | Out-Null
    Report "cli-version-invalid-semver-exit-one" ($LASTEXITCODE -eq 1) "exit $LASTEXITCODE"
}
finally {
    Remove-FixtureRepo $repoH5
}

# -- H6: -Version not strictly greater than current --
$repoH6 = New-ReleaseFixtureRepo -Version '0.5.0'
try {
    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH6 -Version '0.5.0' 2>&1 | Out-Null
    Report "cli-version-not-greater-exit-one" ($LASTEXITCODE -eq 1) "exit $LASTEXITCODE"

    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH6 -Version '0.4.0' 2>&1 | Out-Null
    Report "cli-version-lower-exit-one" ($LASTEXITCODE -eq 1) "exit $LASTEXITCODE"
}
finally {
    Remove-FixtureRepo $repoH6
}

# -- H7: nothing releasable (only docs/chore commits, no tag, no -Version) --
$repoH7 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoH7 -Message "docs: tweak docs" | Out-Null
    Add-ReleaseFixtureCommit -RepoPath $repoH7 -Message "chore: tidy up" | Out-Null

    $nothingOutput = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH7 2>&1
    $nothingExit = $LASTEXITCODE
    $nothingText = ($nothingOutput -join "`n")

    Report "cli-nothing-to-release-exit-one" ($nothingExit -eq 1) "exit $nothingExit"
    Report "cli-nothing-to-release-message" ($nothingText -match 'nothing to release' -or $nothingText -match 'Nothing to release')
    Report "cli-nothing-to-release-mentions-commit-count" ($nothingText -match '\b3\b')

    $nothingJsonOutput = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH7 -Json
    $nothingJsonParsed = $null
    try { $nothingJsonParsed = ($nothingJsonOutput -join "`n") | ConvertFrom-Json } catch { $nothingJsonParsed = $null }
    Report "cli-nothing-to-release-json-still-emitted" ($null -ne $nothingJsonParsed -and $nothingJsonParsed.bump -eq 'none')
    Report "cli-nothing-to-release-json-next-null" ($null -ne $nothingJsonParsed -and $null -eq $nothingJsonParsed.next)
}
finally {
    Remove-FixtureRepo $repoH7
}

# -- H8: missing plugin.json -> exit 2 --
$tempRootH8 = Join-Path ([IO.Path]::GetTempPath()) ("nerv-release-test-nopluginjson-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRootH8 -Force | Out-Null
try {
    & git init -q -b main $tempRootH8 2>$null | Out-Null
    & git -C $tempRootH8 -c user.name=t -c user.email=t@t commit -q --allow-empty -m "chore: empty" 2>$null | Out-Null

    & $pwshExe -NoProfile -File $scriptPath -RepoPath $tempRootH8 2>&1 | Out-Null
    Report "cli-missing-plugin-json-exit-two" ($LASTEXITCODE -eq 2) "exit $LASTEXITCODE"
}
finally {
    Remove-FixtureRepo $tempRootH8
}

# -- H9: not a git repository -> exit 2 --
$repoH9 = New-ReleaseFixtureRepo -Version '0.1.0' -SkipGitInit
try {
    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH9 2>&1 | Out-Null
    Report "cli-not-a-git-repo-exit-two" ($LASTEXITCODE -eq 2) "exit $LASTEXITCODE"
}
finally {
    Remove-FixtureRepo $repoH9
}

# -- H10: -PreRelease produces an rc version end to end --
$repoH10 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoH10 -Message "feat: prerelease-worthy feature" | Out-Null

    $preOutput = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoH10 -PreRelease 'rc' -Json
    $preExit = $LASTEXITCODE
    $preParsed = $null
    try { $preParsed = ($preOutput -join "`n") | ConvertFrom-Json } catch { $preParsed = $null }

    Report "cli-prerelease-exit-zero" ($preExit -eq 0) "exit $preExit"
    Report "cli-prerelease-next-has-rc-suffix" ($null -ne $preParsed -and $preParsed.next -eq '0.2.0-rc.1')
    Report "cli-prerelease-field-set" ($null -ne $preParsed -and $preParsed.prerelease -eq 'rc')
}
finally {
    Remove-FixtureRepo $repoH10
}

# ---------------------------------------------------------------------------
# Case group D2: Get-NervNextVersion -- PreReleaseBase and label validation
# ---------------------------------------------------------------------------
Report "nextversion-prerelease-base-default-is-next" (
    (Get-NervNextVersion -Current '0.2.0' -Bump 'minor' -PreRelease 'rc' -ExistingTags @()) -eq '0.3.0-rc.1'
)
Report "nextversion-prerelease-base-current-uses-current-as-base" (
    (Get-NervNextVersion -Current '0.2.0' -Bump 'none' -PreRelease 'rc' -PreReleaseBase 'current' -ExistingTags @('v0.1.0')) -eq '0.2.0-rc.1'
)
Report "nextversion-prerelease-base-current-numbers-from-existing-rc-tag" (
    (Get-NervNextVersion -Current '0.2.0' -Bump 'none' -PreRelease 'rc' -PreReleaseBase 'current' -ExistingTags @('v0.1.0', 'v0.2.0-rc.1')) -eq '0.2.0-rc.2'
)
Report "nextversion-prerelease-base-current-other-labels-do-not-interfere" (
    (Get-NervNextVersion -Current '0.2.0' -Bump 'none' -PreRelease 'rc' -PreReleaseBase 'current' -ExistingTags @('v0.2.0-alpha.3')) -eq '0.2.0-rc.1'
)
Report "nextversion-prerelease-base-next-other-labels-do-not-interfere" (
    (Get-NervNextVersion -Current '0.1.0' -Bump 'minor' -PreRelease 'rc' -ExistingTags @('v0.2.0-alpha.3')) -eq '0.2.0-rc.1'
)

$labelThrew = $false
try { Get-NervNextVersion -Current '1.0.0' -Bump 'minor' -PreRelease 'RC' -ExistingTags @() | Out-Null }
catch { $labelThrew = $true }
Report "nextversion-invalid-label-uppercase-throws" $labelThrew

$labelThrew2 = $false
try { Get-NervNextVersion -Current '1.0.0' -Bump 'minor' -PreRelease 'rc1' -ExistingTags @() | Out-Null }
catch { $labelThrew2 = $true }
Report "nextversion-invalid-label-digits-throws" $labelThrew2

$labelThrew3 = $false
try { Get-NervNextVersion -Current '1.0.0' -Bump 'minor' -PreRelease 'release-candidate' -ExistingTags @() | Out-Null }
catch { $labelThrew3 = $true }
Report "nextversion-invalid-label-hyphen-throws" $labelThrew3

# ---------------------------------------------------------------------------
# Case group I: -PreRelease / -PreReleaseBase CLI behaviour and JSON tag/base
# ---------------------------------------------------------------------------

# -- I1: JSON gains tag and base fields on a plain (non-prerelease) run --
$repoI1 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoI1 -Message "feat: add cool feature" | Out-Null

    $i1Output = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoI1 -Preview -Json
    $i1Parsed = $null
    try { $i1Parsed = ($i1Output -join "`n") | ConvertFrom-Json } catch { $i1Parsed = $null }

    Report "cli-json-has-tag-field" ($null -ne $i1Parsed -and $i1Parsed.tag -eq 'v0.2.0')
    Report "cli-json-has-base-field" ($null -ne $i1Parsed -and $i1Parsed.base -eq '0.2.0')
}
finally {
    Remove-FixtureRepo $repoI1
}

# -- I2: JSON tag/base on a -PreRelease run (base next) --
$repoI2 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoI2 -Message "feat: prerelease-worthy feature" | Out-Null

    $i2Output = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoI2 -PreRelease 'alpha' -Json
    $i2Parsed = $null
    try { $i2Parsed = ($i2Output -join "`n") | ConvertFrom-Json } catch { $i2Parsed = $null }

    Report "cli-prerelease-next-json-tag" ($null -ne $i2Parsed -and $i2Parsed.tag -eq 'v0.2.0-alpha.1')
    Report "cli-prerelease-next-json-base" ($null -ne $i2Parsed -and $i2Parsed.base -eq '0.2.0')
}
finally {
    Remove-FixtureRepo $repoI2
}

# -- I3: nothing-to-release still reports tag=null, base=null --
$repoI3 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoI3 -Message "docs: tweak docs only" | Out-Null

    $i3Output = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoI3 -Json
    $i3Parsed = $null
    try { $i3Parsed = ($i3Output -join "`n") | ConvertFrom-Json } catch { $i3Parsed = $null }

    Report "cli-nothing-to-release-json-tag-null" ($null -ne $i3Parsed -and $null -eq $i3Parsed.tag)
    Report "cli-nothing-to-release-json-base-null" ($null -ne $i3Parsed -and $null -eq $i3Parsed.base)
}
finally {
    Remove-FixtureRepo $repoI3
}

# -- I4: invalid -PreRelease label exits 1 (CLI) --
$repoI4 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoI4 -Message "feat: add cool feature" | Out-Null

    $i4Output = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoI4 -PreRelease 'RC' 2>&1
    Report "cli-prerelease-invalid-label-exit-one" ($LASTEXITCODE -eq 1) "exit $LASTEXITCODE"
    Report "cli-prerelease-invalid-label-message" (($i4Output -join "`n") -match '(?i)invalid')
}
finally {
    Remove-FixtureRepo $repoI4
}

# -- I5: -PreReleaseBase without -PreRelease exits 1 (meaningless) --
$repoI5 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoI5 -PreReleaseBase 'current' 2>&1 | Out-Null
    Report "cli-prereleasebase-without-prerelease-exit-one" ($LASTEXITCODE -eq 1) "exit $LASTEXITCODE"
}
finally {
    Remove-FixtureRepo $repoI5
}

# -- I6: -Apply combined with -PreRelease exits 1, writes nothing (tag-only) --
$repoI6 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoI6 -Message "feat: add cool feature" | Out-Null
    $pluginJsonPathI6 = Join-Path $repoI6 "plugin/.claude-plugin/plugin.json"
    $beforeI6 = [System.IO.File]::ReadAllText($pluginJsonPathI6)

    $i6Output = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoI6 -PreRelease 'alpha' -Apply 2>&1
    Report "cli-apply-with-prerelease-exit-one" ($LASTEXITCODE -eq 1) "exit $LASTEXITCODE"
    Report "cli-apply-with-prerelease-message" (($i6Output -join "`n") -match '(?i)tag-only')

    $afterI6 = [System.IO.File]::ReadAllText($pluginJsonPathI6)
    Report "cli-apply-with-prerelease-plugin-json-unchanged" ($beforeI6 -ceq $afterI6)
    Report "cli-apply-with-prerelease-no-changelog" (-not (Test-Path -LiteralPath (Join-Path $repoI6 "CHANGELOG.md")))
}
finally {
    Remove-FixtureRepo $repoI6
}

# -- I7: -PreReleaseBase current end to end, matching the fixture in the
#    feature document: plugin.json at 0.2.0, tags v0.1.0 and v0.2.0-rc.1 --
$repoI7 = New-ReleaseFixtureRepo -Version '0.2.0'
try {
    Add-ReleaseFixtureTag -RepoPath $repoI7 -Tag 'v0.1.0'
    Add-ReleaseFixtureTag -RepoPath $repoI7 -Tag 'v0.2.0-rc.1'

    $i7Output = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoI7 -PreRelease 'rc' -PreReleaseBase 'current' -Json
    $i7Exit = $LASTEXITCODE
    $i7Parsed = $null
    try { $i7Parsed = ($i7Output -join "`n") | ConvertFrom-Json } catch { $i7Parsed = $null }

    Report "cli-prereleasebase-current-exit-zero" ($i7Exit -eq 0) "exit $i7Exit"
    Report "cli-prereleasebase-current-next" ($null -ne $i7Parsed -and $i7Parsed.next -eq '0.2.0-rc.2')
    Report "cli-prereleasebase-current-tag" ($null -ne $i7Parsed -and $i7Parsed.tag -eq 'v0.2.0-rc.2')
    Report "cli-prereleasebase-current-base" ($null -ne $i7Parsed -and $i7Parsed.base -eq '0.2.0')
}
finally {
    Remove-FixtureRepo $repoI7
}

# -- I8: -PreReleaseBase current with no rc tag yet -> rc.1, and it does not
#    require a releasable commit (no feat/fix since v0.1.0) --
$repoI8 = New-ReleaseFixtureRepo -Version '0.2.0'
try {
    Add-ReleaseFixtureTag -RepoPath $repoI8 -Tag 'v0.1.0'
    Add-ReleaseFixtureCommit -RepoPath $repoI8 -Message "docs: only docs since last stable tag" | Out-Null

    $i8Output = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoI8 -PreRelease 'rc' -PreReleaseBase 'current' -Json
    $i8Exit = $LASTEXITCODE
    $i8Parsed = $null
    try { $i8Parsed = ($i8Output -join "`n") | ConvertFrom-Json } catch { $i8Parsed = $null }

    Report "cli-prereleasebase-current-no-releasable-commit-required-exit-zero" ($i8Exit -eq 0) "exit $i8Exit"
    Report "cli-prereleasebase-current-first-rc" ($null -ne $i8Parsed -and $i8Parsed.next -eq '0.2.0-rc.1')
}
finally {
    Remove-FixtureRepo $repoI8
}

# -- I9: text output prints the tag --
$repoI9 = New-ReleaseFixtureRepo -Version '0.1.0'
try {
    Add-ReleaseFixtureCommit -RepoPath $repoI9 -Message "feat: add cool feature" | Out-Null

    $i9Output = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoI9 -Preview 2>&1
    $i9Text = ($i9Output -join "`n")
    Report "cli-text-output-prints-tag" ($i9Text -match [regex]::Escape('v0.2.0'))
}
finally {
    Remove-FixtureRepo $repoI9
}

Write-Host ""
Write-Host "Results: $script:passCount passed, $script:failCount failed"
if ($script:failCount -ne 0) { exit 1 }
exit 0
