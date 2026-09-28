#requires -Version 7
<#
.SYNOPSIS
    Assertions for tools/release-guard.ps1: the pure/dot-sourceable
    functions (Get-NervChangelogSectionBody, Test-NervReleaseReadiness)
    plus end-to-end child-process runs of the CLI (-RepoPath, -NotesPath,
    -Json) against real temporary git repositories. No Pester -- prints
    PASS/FAIL lines and exits 1 on any failure, matching
    tests/release.test.ps1's style.

    Fixtures are real git repositories built with `git init`, `git
    commit`, and `git tag` -- git is never mocked.

.EXAMPLE
    pwsh -NoProfile -File tests/release-guard.test.ps1
#>

$ErrorActionPreference = "Stop"

$selfDir = $PSScriptRoot
$repoRoot = Split-Path -Parent $selfDir
$scriptPath = Join-Path $repoRoot "tools/release-guard.ps1"

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
# tools/release.ps1), so dot-sourcing only defines functions and never
# writes, tags, or exits for real.
# ---------------------------------------------------------------------------
if (-not (Test-Path -LiteralPath $scriptPath)) {
    Report "release-guard-script-exists" $false "tools/release-guard.ps1 not found (not implemented yet)"
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    exit 1
}
Report "release-guard-script-exists" $true

try {
    . $scriptPath -RepoPath $repoRoot 2>$null
}
catch {
    # Expected during RED (before the CLI body is guarded); the real signal
    # is whether the functions got defined below.
}

$requiredFunctions = @(
    'Get-NervPluginVersion',
    'Get-NervChangelogSectionBody',
    'Test-NervReleaseReadiness'
)
$allDefined = $true
foreach ($fn in $requiredFunctions) {
    if (-not (Get-Command $fn -ErrorAction SilentlyContinue)) { $allDefined = $false }
}

if (-not $allDefined) {
    Report "functions-defined-after-dot-source" $false "one or more functions not found (tools/release-guard.ps1 not implemented yet)"
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
function New-GuardFixtureRepo {
    param(
        [string]$Version = '0.2.0',
        [switch]$WithChangelog,
        [string]$ChangelogText,
        [switch]$SkipPluginJson,
        [switch]$SkipGitInit
    )
    $root = Join-Path ([IO.Path]::GetTempPath()) ("nerv-guard-test-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $root -Force | Out-Null

    if (-not $SkipGitInit) {
        & git init -q -b main $root 2>$null | Out-Null
    }

    if (-not $SkipPluginJson) {
        $pluginDir = Join-Path $root "plugin/.claude-plugin"
        New-Item -ItemType Directory -Path $pluginDir -Force | Out-Null
        $pluginJsonPath = Join-Path $pluginDir "plugin.json"
        $pluginJsonContent = "{`n  `"name`": `"nerv-guard-fixture`",`n  `"version`": `"$Version`",`n  `"description`": `"Fixture plugin for release-guard.ps1 tests`",`n  `"author`": { `"name`": `"Test`" }`n}`n"
        [System.IO.File]::WriteAllText($pluginJsonPath, $pluginJsonContent, $utf8NoBom)
    }

    if ($WithChangelog) {
        $text = if ($ChangelogText) { $ChangelogText } else {
            "# Changelog`n`nAll notable changes to this project will be documented in this file.`n`n## [Unreleased]`n"
        }
        [System.IO.File]::WriteAllText((Join-Path $root "CHANGELOG.md"), $text, $utf8NoBom)
    }

    if (-not $SkipGitInit) {
        & git -C $root add -A 2>$null | Out-Null
        & git -C $root -c user.name=t -c user.email=t@t commit -q -m "chore: initial fixture commit" 2>$null | Out-Null
    }

    return $root
}

function Add-GuardFixtureTag {
    param([Parameter(Mandatory)][string]$RepoPath, [Parameter(Mandatory)][string]$Tag)
    & git -C $RepoPath tag $Tag 2>$null | Out-Null
}

function Remove-GuardFixtureRepo {
    param([string]$RepoPath)
    if ($RepoPath -and (Test-Path -LiteralPath $RepoPath)) {
        Remove-Item -LiteralPath $RepoPath -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function New-FullChangelogText {
    # A changelog with two real sections, shaped exactly like
    # New-NervChangelogSection's output (tools/release.ps1), so extraction
    # is tested against realistic content, not a hand-simplified stand-in.
    return "# Changelog`n`n" +
    "All notable changes to this project will be documented in this file.`n`n" +
    "The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),`n" +
    "and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).`n`n" +
    "## [Unreleased]`n`n" +
    "## [0.2.0] - 2026-09-28`n`n" +
    "### Added`n" +
    "- cli: add --json flag (abc1234)`n`n" +
    "### Fixed`n" +
    "- handle null path (def5678)`n`n" +
    "## [0.1.0] - 2026-09-01`n`n" +
    "### Added`n" +
    "- initial release (0000001)`n"
}

# ---------------------------------------------------------------------------
# Case group A: Get-NervChangelogSectionBody (pure)
# ---------------------------------------------------------------------------
$fullText = New-FullChangelogText

$bodyMiddle = Get-NervChangelogSectionBody -Text $fullText -Version '0.2.0'
Report "sectionbody-middle-not-null" ($null -ne $bodyMiddle)
Report "sectionbody-middle-starts-with-added" ($bodyMiddle.StartsWith("### Added"))
Report "sectionbody-middle-has-both-groups" (
    $bodyMiddle -match [regex]::Escape('cli: add --json flag (abc1234)') -and
    $bodyMiddle -match [regex]::Escape('handle null path (def5678)')
)
Report "sectionbody-middle-excludes-older-section" ($bodyMiddle -notmatch [regex]::Escape('initial release'))
Report "sectionbody-middle-no-leading-blank-line" (-not $bodyMiddle.StartsWith("`n"))
Report "sectionbody-middle-no-trailing-blank-line" (-not $bodyMiddle.EndsWith("`n") -and -not $bodyMiddle.EndsWith("`r"))

$bodyLast = Get-NervChangelogSectionBody -Text $fullText -Version '0.1.0'
Report "sectionbody-last-section-to-eof" (
    $null -ne $bodyLast -and
    $bodyLast.StartsWith("### Added") -and
    ($bodyLast -match [regex]::Escape('initial release (0000001)'))
)
Report "sectionbody-last-no-trailing-blank-line" (-not $bodyLast.EndsWith("`n") -and -not $bodyLast.EndsWith("`r"))

$bodyMissing = Get-NervChangelogSectionBody -Text $fullText -Version '9.9.9'
Report "sectionbody-missing-version-returns-null" ($null -eq $bodyMissing)

# ---------------------------------------------------------------------------
# Case group B: Test-NervReleaseReadiness (dot-sourced, real fixture repos)
# ---------------------------------------------------------------------------

# B1: ok path
$repoB1 = New-GuardFixtureRepo -Version '0.2.0' -WithChangelog -ChangelogText $fullText
try {
    $readyB1 = Test-NervReleaseReadiness -RepoPath $repoB1
    Report "readiness-ok-true-on-clean-repo" ($readyB1.Ok -eq $true)
    Report "readiness-ok-version-matches" ($readyB1.Version -eq '0.2.0')
    Report "readiness-ok-tag-is-v-prefixed" ($readyB1.Tag -eq 'v0.2.0')
    Report "readiness-ok-tag-exists-false" ($readyB1.TagExists -eq $false)
    Report "readiness-ok-changelog-section-true" ($readyB1.ChangelogSection -eq $true)
    Report "readiness-ok-body-set" (-not [string]::IsNullOrEmpty($readyB1.Body))
}
finally {
    Remove-GuardFixtureRepo $repoB1
}

# B2: tag already exists
$repoB2 = New-GuardFixtureRepo -Version '0.2.0' -WithChangelog -ChangelogText $fullText
try {
    Add-GuardFixtureTag -RepoPath $repoB2 -Tag 'v0.2.0'
    $readyB2 = Test-NervReleaseReadiness -RepoPath $repoB2
    Report "readiness-tag-exists-ok-false" ($readyB2.Ok -eq $false)
    Report "readiness-tag-exists-true" ($readyB2.TagExists -eq $true)
    Report "readiness-tag-exists-reason-mentions-tag" ($readyB2.Reason -match [regex]::Escape('v0.2.0'))
}
finally {
    Remove-GuardFixtureRepo $repoB2
}

# B3: missing CHANGELOG.md
$repoB3 = New-GuardFixtureRepo -Version '0.2.0'
try {
    $readyB3 = Test-NervReleaseReadiness -RepoPath $repoB3
    Report "readiness-missing-changelog-ok-false" ($readyB3.Ok -eq $false)
    Report "readiness-missing-changelog-section-false" ($readyB3.ChangelogSection -eq $false)
    Report "readiness-missing-changelog-reason-mentions-file" ($readyB3.Reason -match 'CHANGELOG\.md')
}
finally {
    Remove-GuardFixtureRepo $repoB3
}

# B4: CHANGELOG.md exists, no matching section
$repoB4 = New-GuardFixtureRepo -Version '0.3.0' -WithChangelog -ChangelogText $fullText
try {
    $readyB4 = Test-NervReleaseReadiness -RepoPath $repoB4
    Report "readiness-missing-section-ok-false" ($readyB4.Ok -eq $false)
    Report "readiness-missing-section-false" ($readyB4.ChangelogSection -eq $false)
    Report "readiness-missing-section-tag-exists-false" ($readyB4.TagExists -eq $false)
    Report "readiness-missing-section-reason-mentions-version" ($readyB4.Reason -match [regex]::Escape('0.3.0'))
}
finally {
    Remove-GuardFixtureRepo $repoB4
}

# B5: git failure (not a git repository) -> throws
$repoB5 = New-GuardFixtureRepo -Version '0.2.0' -WithChangelog -ChangelogText $fullText -SkipGitInit
try {
    $threw = $false
    try {
        Test-NervReleaseReadiness -RepoPath $repoB5 | Out-Null
    }
    catch {
        $threw = $true
    }
    Report "readiness-not-a-git-repo-throws" $threw
}
finally {
    Remove-GuardFixtureRepo $repoB5
}

# ---------------------------------------------------------------------------
# Case group C: CLI end-to-end (child process), real temp git repos.
# ---------------------------------------------------------------------------

# C1: ok path with -NotesPath -> exit 0, prints version, notes file has exact body
$repoC1 = New-GuardFixtureRepo -Version '0.2.0' -WithChangelog -ChangelogText $fullText
try {
    $notesPathC1 = Join-Path ([IO.Path]::GetTempPath()) ("nerv-guard-notes-" + [Guid]::NewGuid().ToString("N") + ".md")
    $outC1 = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoC1 -NotesPath $notesPathC1 2>&1
    $exitC1 = $LASTEXITCODE
    $textC1 = ($outC1 -join "`n")

    Report "cli-ok-exit-zero" ($exitC1 -eq 0) "exit $exitC1; output: $textC1"
    Report "cli-ok-prints-version" ($textC1 -match [regex]::Escape('0.2.0'))
    Report "cli-ok-notes-file-created" (Test-Path -LiteralPath $notesPathC1)

    if (Test-Path -LiteralPath $notesPathC1) {
        $expectedBody = Get-NervChangelogSectionBody -Text $fullText -Version '0.2.0'
        $notesBytes = [System.IO.File]::ReadAllBytes($notesPathC1)
        $notesText = [System.IO.File]::ReadAllText($notesPathC1)
        Report "cli-ok-notes-no-bom" ($notesBytes.Length -eq 0 -or $notesBytes[0] -ne 0xEF)
        Report "cli-ok-notes-lf-only" (-not $notesText.Contains("`r"))
        Report "cli-ok-notes-exact-body" ($notesText.TrimEnd("`n") -ceq $expectedBody)
        Report "cli-ok-notes-no-trailing-blank-line" (-not $notesText.EndsWith("`n`n"))
    }
    else {
        Report "cli-ok-notes-no-bom" $false "notes file missing"
        Report "cli-ok-notes-lf-only" $false "notes file missing"
        Report "cli-ok-notes-exact-body" $false "notes file missing"
        Report "cli-ok-notes-no-trailing-blank-line" $false "notes file missing"
    }
}
finally {
    Remove-GuardFixtureRepo $repoC1
    if ($notesPathC1 -and (Test-Path -LiteralPath $notesPathC1)) { Remove-Item -LiteralPath $notesPathC1 -Force -ErrorAction SilentlyContinue }
}

# C2: -Json shape on success
$repoC2 = New-GuardFixtureRepo -Version '0.2.0' -WithChangelog -ChangelogText $fullText
try {
    $jsonOutC2 = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoC2 -Json
    $exitC2 = $LASTEXITCODE
    $parsedC2 = $null
    try { $parsedC2 = ($jsonOutC2 -join "`n") | ConvertFrom-Json } catch { $parsedC2 = $null }

    Report "cli-json-ok-exit-zero" ($exitC2 -eq 0) "exit $exitC2"
    Report "cli-json-ok-parses" ($null -ne $parsedC2)
    if ($parsedC2) {
        Report "cli-json-ok-version" ($parsedC2.version -eq '0.2.0')
        Report "cli-json-ok-tag" ($parsedC2.tag -eq 'v0.2.0')
        Report "cli-json-ok-changelog-section-true" ($parsedC2.changelog_section -eq $true)
        Report "cli-json-ok-tag-exists-false" ($parsedC2.tag_exists -eq $false)
        Report "cli-json-ok-notes-path-null" ($null -eq $parsedC2.notes_path)
        Report "cli-json-ok-ok-true" ($parsedC2.ok -eq $true)
    }
    else {
        foreach ($n in @('version', 'tag', 'changelog-section-true', 'tag-exists-false', 'notes-path-null', 'ok-true')) {
            Report "cli-json-ok-$n" $false "no parsed JSON"
        }
    }
}
finally {
    Remove-GuardFixtureRepo $repoC2
}

# C3: tag already exists -> exit 1, -Json shows tag_exists true, ok false
$repoC3 = New-GuardFixtureRepo -Version '0.2.0' -WithChangelog -ChangelogText $fullText
try {
    Add-GuardFixtureTag -RepoPath $repoC3 -Tag 'v0.2.0'

    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoC3 2>&1 | Out-Null
    Report "cli-tag-exists-exit-one" ($LASTEXITCODE -eq 1) "exit $LASTEXITCODE"

    $jsonOutC3 = & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoC3 -Json
    $exitC3Json = $LASTEXITCODE
    $parsedC3 = $null
    try { $parsedC3 = ($jsonOutC3 -join "`n") | ConvertFrom-Json } catch { $parsedC3 = $null }

    Report "cli-tag-exists-json-exit-one" ($exitC3Json -eq 1) "exit $exitC3Json"
    Report "cli-tag-exists-json-parses" ($null -ne $parsedC3)
    if ($parsedC3) {
        Report "cli-tag-exists-json-ok-false" ($parsedC3.ok -eq $false)
        Report "cli-tag-exists-json-tag-exists-true" ($parsedC3.tag_exists -eq $true)
    }
    else {
        Report "cli-tag-exists-json-ok-false" $false "no parsed JSON"
        Report "cli-tag-exists-json-tag-exists-true" $false "no parsed JSON"
    }
}
finally {
    Remove-GuardFixtureRepo $repoC3
}

# C4: missing CHANGELOG.md -> exit 1
$repoC4 = New-GuardFixtureRepo -Version '0.2.0'
try {
    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoC4 2>&1 | Out-Null
    Report "cli-missing-changelog-exit-one" ($LASTEXITCODE -eq 1) "exit $LASTEXITCODE"
}
finally {
    Remove-GuardFixtureRepo $repoC4
}

# C5: CHANGELOG.md exists, no matching section -> exit 1
$repoC5 = New-GuardFixtureRepo -Version '0.3.0' -WithChangelog -ChangelogText $fullText
try {
    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoC5 2>&1 | Out-Null
    Report "cli-missing-section-exit-one" ($LASTEXITCODE -eq 1) "exit $LASTEXITCODE"
}
finally {
    Remove-GuardFixtureRepo $repoC5
}

# C6: missing plugin.json -> exit 2
$repoC6 = New-GuardFixtureRepo -Version '0.2.0' -WithChangelog -ChangelogText $fullText -SkipPluginJson
try {
    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoC6 2>&1 | Out-Null
    Report "cli-missing-plugin-json-exit-two" ($LASTEXITCODE -eq 2) "exit $LASTEXITCODE"
}
finally {
    Remove-GuardFixtureRepo $repoC6
}

# C7: not a git repository -> exit 2
$repoC7 = New-GuardFixtureRepo -Version '0.2.0' -WithChangelog -ChangelogText $fullText -SkipGitInit
try {
    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoC7 2>&1 | Out-Null
    Report "cli-not-a-git-repo-exit-two" ($LASTEXITCODE -eq 2) "exit $LASTEXITCODE"
}
finally {
    Remove-GuardFixtureRepo $repoC7
}

# C8: repo path does not exist at all -> exit 2
$missingRepoPath = Join-Path ([IO.Path]::GetTempPath()) ("nerv-guard-does-not-exist-" + [Guid]::NewGuid().ToString("N"))
& $pwshExe -NoProfile -File $scriptPath -RepoPath $missingRepoPath 2>&1 | Out-Null
Report "cli-repo-path-not-found-exit-two" ($LASTEXITCODE -eq 2) "exit $LASTEXITCODE"

# C9: failure path with -NotesPath given -> notes file must NOT be written
$repoC9 = New-GuardFixtureRepo -Version '0.2.0'
try {
    $notesPathC9 = Join-Path ([IO.Path]::GetTempPath()) ("nerv-guard-notes-fail-" + [Guid]::NewGuid().ToString("N") + ".md")
    & $pwshExe -NoProfile -File $scriptPath -RepoPath $repoC9 -NotesPath $notesPathC9 2>&1 | Out-Null
    Report "cli-failure-does-not-write-notes" (-not (Test-Path -LiteralPath $notesPathC9))
}
finally {
    Remove-GuardFixtureRepo $repoC9
    if ($notesPathC9 -and (Test-Path -LiteralPath $notesPathC9)) { Remove-Item -LiteralPath $notesPathC9 -Force -ErrorAction SilentlyContinue }
}

Write-Host ""
Write-Host "Results: $script:passCount passed, $script:failCount failed"
if ($script:failCount -ne 0) { exit 1 }
exit 0
