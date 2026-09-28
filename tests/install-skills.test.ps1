#requires -Version 7
<#
.SYNOPSIS
    Assertions for tools/install-skills.ps1: Read-NervSkillsManifest,
    Test-NervSkillInstalled, Get-NervSkillsStatus, and New-NervSkillInstallArgs,
    plus an end-to-end -DryRun child run. No Pester — prints PASS/FAIL lines and
    exits 1 on any failure, matching the style of
    tests/install-apply-models.test.ps1.

.EXAMPLE
    pwsh -NoProfile -File tests/install-skills.test.ps1
#>

$ErrorActionPreference = "Stop"

$selfDir = $PSScriptRoot
$repoRoot = Split-Path -Parent $selfDir
$scriptPath = Join-Path $repoRoot "plugin/tools/install-skills.ps1"
$realManifestPath = Join-Path $repoRoot "plugin/tools/skills-manifest.json"
$rootForwarderPath = Join-Path $repoRoot "tools/install-skills.ps1"

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

$expectedExternal = @(
    "tdd",
    "playwright-best-practices",
    "dotnet-best-practices",
    "typescript-best-practices",
    "best-practices",
    "solid-principles",
    "clean-code-guard",
    "hexagonal-architecture",
    "c4-architecture"
)

# ---------------------------------------------------------------------------
# Dot-source the script. Body must be guarded behind
# `if ($MyInvocation.InvocationName -ne '.')` so dot-sourcing only defines
# functions and never runs npx or writes anything.
# ---------------------------------------------------------------------------
try {
    . $scriptPath -SkillsDir (Join-Path ([System.IO.Path]::GetTempPath()) "nerv-install-skills-test-nonexistent") 2>$null
}
catch {
    # Expected during RED (script missing, or unguarded body throwing).
}

$readAvailable = [bool](Get-Command Read-NervSkillsManifest -ErrorAction SilentlyContinue)
$testInstalledAvailable = [bool](Get-Command Test-NervSkillInstalled -ErrorAction SilentlyContinue)
$statusAvailable = [bool](Get-Command Get-NervSkillsStatus -ErrorAction SilentlyContinue)
$argsAvailable = [bool](Get-Command New-NervSkillInstallArgs -ErrorAction SilentlyContinue)

if (-not $readAvailable -or -not $testInstalledAvailable -or -not $statusAvailable -or -not $argsAvailable) {
    Report "functions-defined-after-dot-source" $false "one or more of Read-NervSkillsManifest/Test-NervSkillInstalled/Get-NervSkillsStatus/New-NervSkillInstallArgs not found (plugin/tools/install-skills.ps1 not implemented yet)"
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    exit 1
}
Report "functions-defined-after-dot-source" $true

# ---------------------------------------------------------------------------
# Fixture setup
# ---------------------------------------------------------------------------
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-install-skills-test-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null

# ---------------------------------------------------------------------------
# Case group A: Read-NervSkillsManifest
# ---------------------------------------------------------------------------
$manifest = Read-NervSkillsManifest -Path $realManifestPath
Report "manifest-has-at-least-15-entries" ($manifest.Count -ge 15) "found $($manifest.Count)"

$manifestNames = @($manifest | ForEach-Object { $_.name })
$missingExternal = @($expectedExternal | Where-Object { $manifestNames -notcontains $_ })
Report "manifest-has-all-nine-external-skills" ($missingExternal.Count -eq 0) ("missing: " + ($missingExternal -join ', '))

$externalCount = @($manifest | Where-Object { $_.kind -eq 'external' }).Count
Report "manifest-external-count-is-nine" ($externalCount -eq 9) "found $externalCount"

$malformedPath = Join-Path $tempRoot "malformed-manifest.json"
'{ "schema": "wrong-schema/v1", "skills": [] }' | Set-Content -LiteralPath $malformedPath -Encoding UTF8 -NoNewline
$threw = $false
try {
    Read-NervSkillsManifest -Path $malformedPath | Out-Null
}
catch {
    $threw = $true
}
Report "malformed-manifest-throws" $threw

$missingKeyPath = Join-Path $tempRoot "missing-key-manifest.json"
'{ "schema": "nerv.skills-manifest/v1", "skills": [ { "name": "x" } ] }' | Set-Content -LiteralPath $missingKeyPath -Encoding UTF8 -NoNewline
$threw2 = $false
try {
    Read-NervSkillsManifest -Path $missingKeyPath | Out-Null
}
catch {
    $threw2 = $true
}
Report "manifest-entry-missing-kind-throws" $threw2

# ---------------------------------------------------------------------------
# Case group B: Test-NervSkillInstalled
# ---------------------------------------------------------------------------
$skillsDirB = Join-Path $tempRoot "skills-b"
New-Item -ItemType Directory -Path (Join-Path $skillsDirB "present-skill") -Force | Out-Null
"# present" | Set-Content -LiteralPath (Join-Path $skillsDirB "present-skill/SKILL.md") -Encoding UTF8
New-Item -ItemType Directory -Path (Join-Path $skillsDirB "empty-skill") -Force | Out-Null

Report "test-installed-true-when-skill-md-present" (Test-NervSkillInstalled -SkillsDir $skillsDirB -Name "present-skill")
Report "test-installed-false-when-skill-md-absent" (-not (Test-NervSkillInstalled -SkillsDir $skillsDirB -Name "empty-skill"))
Report "test-installed-false-when-dir-absent" (-not (Test-NervSkillInstalled -SkillsDir $skillsDirB -Name "nonexistent-skill"))

# ---------------------------------------------------------------------------
# Case group C: Get-NervSkillsStatus
# ---------------------------------------------------------------------------
$skillsDirC = Join-Path $tempRoot "skills-c"
foreach ($n in @("tdd", "solid-principles")) {
    New-Item -ItemType Directory -Path (Join-Path $skillsDirC $n) -Force | Out-Null
    "# $n" | Set-Content -LiteralPath (Join-Path $skillsDirC "$n/SKILL.md") -Encoding UTF8
}

$miniManifest = @(
    [pscustomobject]@{ name = "tdd"; kind = "external"; repo = "mattpocock/skills"; skill = "tdd"; used_by = @("testing") },
    [pscustomobject]@{ name = "solid-principles"; kind = "external"; repo = "thebushidocollective/han"; skill = "solid-principles"; used_by = @("best-practices") },
    [pscustomobject]@{ name = "c4-architecture"; kind = "external"; repo = "softaworks/agent-toolkit"; skill = "c4-architecture"; used_by = @("architecture") },
    [pscustomobject]@{ name = "work-unit-commits"; kind = "gentle-ai"; used_by = @("delivery") },
    [pscustomobject]@{ name = "security-review"; kind = "builtin"; used_by = @("audit") }
)

$status = Get-NervSkillsStatus -Manifest $miniManifest -SkillsDir $skillsDirC
$byName = @{}
foreach ($s in $status) { $byName[$s.name] = $s }

Report "status-installed-external-action-none" ($byName['tdd'].installed -eq $true -and $byName['tdd'].action -eq 'none')
Report "status-installed-external-2-action-none" ($byName['solid-principles'].installed -eq $true -and $byName['solid-principles'].action -eq 'none')
Report "status-missing-external-action-install" ($byName['c4-architecture'].installed -eq $false -and $byName['c4-architecture'].action -eq 'install')
Report "status-missing-gentle-ai-action-verify" ($byName['work-unit-commits'].installed -eq $false -and $byName['work-unit-commits'].action -eq 'verify-gentle-ai')
Report "status-builtin-action-none" ($byName['security-review'].action -eq 'none')

# ---------------------------------------------------------------------------
# Case group D: New-NervSkillInstallArgs
# ---------------------------------------------------------------------------
$entryWithSkill = [pscustomobject]@{ name = "tdd"; kind = "external"; repo = "mattpocock/skills"; skill = "tdd" }
$argsWithSkill = New-NervSkillInstallArgs -Entry $entryWithSkill
$expectedWithSkill = @("skills", "add", "mattpocock/skills", "--skill", "tdd", "-g", "-a", "claude-code", "-y")
Report "install-args-with-skill" ((($argsWithSkill -join '|') -eq ($expectedWithSkill -join '|')))

$entryWithoutSkill = [pscustomobject]@{ name = "playwright-best-practices"; kind = "external"; repo = "currents-dev/playwright-best-practices-skill" }
$argsWithoutSkill = New-NervSkillInstallArgs -Entry $entryWithoutSkill
$expectedWithoutSkill = @("skills", "add", "currents-dev/playwright-best-practices-skill", "-g", "-a", "claude-code", "-y")
Report "install-args-without-skill" ((($argsWithoutSkill -join '|') -eq ($expectedWithoutSkill -join '|')))

# ---------------------------------------------------------------------------
# Case group E: end-to-end child run, -DryRun only (never invokes real npx)
# ---------------------------------------------------------------------------
$skillsDirE = Join-Path $tempRoot "skills-e"
New-Item -ItemType Directory -Path $skillsDirE -Force | Out-Null

$outputE = & pwsh -NoProfile -File $scriptPath -ManifestPath $realManifestPath -SkillsDir $skillsDirE -DryRun 2>&1
$exitE = $LASTEXITCODE
$installLinesE = @($outputE | Where-Object { $_ -match 'npx skills add' })
Report "dryrun-fresh-dir-prints-nine-install-lines" ($installLinesE.Count -eq 9) "found $($installLinesE.Count)"
Report "dryrun-fresh-dir-exit-zero" ($exitE -eq 0) "exit $exitE"

$outputOnly = & pwsh -NoProfile -File $scriptPath -ManifestPath $realManifestPath -SkillsDir $skillsDirE -Only "tdd" -DryRun 2>&1
$exitOnly = $LASTEXITCODE
$installLinesOnly = @($outputOnly | Where-Object { $_ -match 'npx skills add' })
Report "dryrun-only-tdd-prints-exactly-one-line" ($installLinesOnly.Count -eq 1) "found $($installLinesOnly.Count)"
Report "dryrun-only-tdd-exit-zero" ($exitOnly -eq 0) "exit $exitOnly"

# --- -Only with a name absent from the manifest: clear message, exit 1, no binding error ---
$outputOnlyBad = & pwsh -NoProfile -File $scriptPath -ManifestPath $realManifestPath -SkillsDir $skillsDirE -Only "no-such-skill" -DryRun 2>&1
$exitOnlyBad = $LASTEXITCODE
$joinedBad = ($outputOnlyBad | ForEach-Object { [string]$_ }) -join "`n"
Report "only-unknown-name-exits-1" ($exitOnlyBad -eq 1) "exit $exitOnlyBad"
Report "only-unknown-name-names-the-skill" ($joinedBad -match 'no-such-skill') "output: $($joinedBad.Substring(0, [Math]::Min(200, $joinedBad.Length)))"
Report "only-unknown-name-no-binding-error" ($joinedBad -notmatch 'Cannot bind argument|ParameterBindingValidationException') "output: $($joinedBad.Substring(0, [Math]::Min(200, $joinedBad.Length)))"
# --- -Json shape is always an array, even for one entry ---
$jsonOne = (& pwsh -NoProfile -File $scriptPath -ManifestPath $realManifestPath -SkillsDir $skillsDirE -Only "tdd" -Json 2>&1 | Out-String)
$jsonOneTrim = $jsonOne.Trim()
Report "json-single-entry-is-array" ($jsonOneTrim.StartsWith('[')) "starts with: $($jsonOneTrim.Substring(0, [Math]::Min(20, $jsonOneTrim.Length)))"

$outputJson = & pwsh -NoProfile -File $scriptPath -ManifestPath $realManifestPath -SkillsDir $skillsDirE -Json -DryRun 2>&1
$jsonText = ($outputJson -join "`n")
$parsedOk = $true
$parsed = $null
try {
    $parsed = $jsonText | ConvertFrom-Json
}
catch {
    $parsedOk = $false
}
Report "json-output-parses" $parsedOk
if ($parsedOk) {
    Report "json-output-has-at-least-15-entries" (@($parsed).Count -ge 15) "found $(@($parsed).Count)"
}
else {
    Report "json-output-has-at-least-15-entries" $false "JSON did not parse"
}

# ---------------------------------------------------------------------------
# Case group F: root tools/install-skills.ps1 is a thin forwarder to
# plugin/tools/install-skills.ps1 — same args, same output, same exit code.
# ---------------------------------------------------------------------------
Report "root-forwarder-exists" (Test-Path -LiteralPath $rootForwarderPath)

if (Test-Path -LiteralPath $rootForwarderPath) {
    $skillsDirF = Join-Path $tempRoot "skills-f"
    New-Item -ItemType Directory -Path $skillsDirF -Force | Out-Null

    $outputForwarder = & pwsh -NoProfile -File $rootForwarderPath -ManifestPath $realManifestPath -SkillsDir $skillsDirF -Only "tdd" -Json -DryRun 2>&1
    $exitForwarder = $LASTEXITCODE
    $outputDirect = & pwsh -NoProfile -File $scriptPath -ManifestPath $realManifestPath -SkillsDir $skillsDirF -Only "tdd" -Json -DryRun 2>&1
    $exitDirect = $LASTEXITCODE

    $forwarderJoined = ($outputForwarder -join "`n")
    $directJoined = ($outputDirect -join "`n")

    Report "root-forwarder-output-matches-plugin-script" ($forwarderJoined -eq $directJoined) "forwarder exit $exitForwarder, direct exit $exitDirect"
    Report "root-forwarder-exit-matches" ($exitForwarder -eq $exitDirect) "forwarder $exitForwarder vs direct $exitDirect"
}
else {
    Report "root-forwarder-output-matches-plugin-script" $false "tools/install-skills.ps1 not found"
    Report "root-forwarder-exit-matches" $false "tools/install-skills.ps1 not found"
}

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "Results: $script:passCount passed, $script:failCount failed"
if ($script:failCount -ne 0) { exit 1 }
exit 0
