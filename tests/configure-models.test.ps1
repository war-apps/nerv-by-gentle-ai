#requires -Version 7
<#
.SYNOPSIS
    Assertions for tools/configure-models.ps1: the pure functions
    (Format-NervModelsBlock, Set-NervYamlModelsBlock, Read-NervModelsOverrides,
    Get-NervModelTable) plus one end-to-end child-process run of the
    interactive wizard driven by -AnswersFile. No Pester — prints PASS/FAIL
    lines and exits 1 on any failure, matching
    tests/install-apply-models.test.ps1's style.

.EXAMPLE
    pwsh -NoProfile -File tests/configure-models.test.ps1
#>

$ErrorActionPreference = "Stop"

$selfDir = $PSScriptRoot
$repoRoot = Split-Path -Parent $selfDir
$wizardPath = Join-Path $repoRoot "plugin/tools/configure-models.ps1"
$rootForwarderPath = Join-Path $repoRoot "tools/configure-models.ps1"

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
# Dot-source the wizard. Its interactive body is guarded by
# `if ($MyInvocation.InvocationName -ne '.')` (same pattern as
# tools/install.ps1), so dot-sourcing only defines functions and never
# prompts, writes, or applies anything for real.
# ---------------------------------------------------------------------------
if (-not (Test-Path -LiteralPath $wizardPath)) {
    Report "wizard-script-exists" $false "plugin/tools/configure-models.ps1 not found (not implemented yet)"
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    exit 1
}
Report "wizard-script-exists" $true

$bogusConfigPath = Join-Path ([System.IO.Path]::GetTempPath()) "nerv-configure-models-test-nonexistent.yaml"
$bogusStatePath = Join-Path ([System.IO.Path]::GetTempPath()) "nerv-configure-models-test-nonexistent-state.json"
try {
    . $wizardPath -ConfigPath $bogusConfigPath -StatePath $bogusStatePath -RepoPath $repoRoot -NoApply -AnswersFile $bogusConfigPath 2>$null
}
catch {
    # Expected during RED (before the interactive body is guarded); the real
    # signal is whether the functions got defined below.
}

$requiredFunctions = @(
    'Format-NervModelsBlock',
    'Set-NervYamlModelsBlock',
    'Read-NervModelsOverrides',
    'Get-NervModelTable'
)
$allDefined = $true
foreach ($fn in $requiredFunctions) {
    if (-not (Get-Command $fn -ErrorAction SilentlyContinue)) { $allDefined = $false }
}

if (-not $allDefined) {
    Report "functions-defined-after-dot-source" $false "one or more pure functions not found (tools/configure-models.ps1 not implemented yet)"
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    exit 1
}
Report "functions-defined-after-dot-source" $true

# ---------------------------------------------------------------------------
# Case group A: Format-NervModelsBlock
# ---------------------------------------------------------------------------
$formatOverrides = @{
    'misato' = @{ Model = 'fable'; Effort = 'high' }
    'aoba'   = @{ From = 'jd-judge-b' }
}
$formatBlock = Format-NervModelsBlock -Overrides $formatOverrides
$formatLines = $formatBlock -split "`n"

Report "format-header-line" ($formatLines[0] -eq 'models:                             # per-role model and effort (written by tools/configure-models.ps1)')
Report "format-sorted-aoba-first" ($formatLines[1] -match '^\s*aoba: \{ from: jd-judge-b \}$')
Report "format-misato-explicit-syntax" ($formatLines[2] -match '^\s*misato: \{ model: fable, effort: high \}$')
Report "format-no-trailing-blank-line" (-not $formatBlock.EndsWith("`n") -and $formatLines.Count -eq 3)
Report "format-uses-lf" (-not $formatBlock.Contains("`r`n"))

# ---------------------------------------------------------------------------
# Case group B: Set-NervYamlModelsBlock
# ---------------------------------------------------------------------------
$yamlWithBlockLf = (
    "skills: {}`n" +
    "models:                             # old header`n" +
    "  aoba: { model: haiku, effort: low }`n" +
    "# balthasar: { model: sonnet, effort: medium }`n" +
    "#   casper: { model: sonnet, effort: medium }`n" +
    "  rei: { from: jd-judge-b }`n" +
    "critical_paths: [auth/, payments/, migrations/, infra/]"
)
$newBlock = "models:                             # new header`n  misato: { model: fable, effort: high }"

$replaced = Set-NervYamlModelsBlock -YamlText $yamlWithBlockLf -BlockText $newBlock
$expectedReplaced = "skills: {}`n" + $newBlock + "`ncritical_paths: [auth/, payments/, migrations/, infra/]`n"
Report "set-replace-exact" ($replaced -eq $expectedReplaced)

$noBlockYaml = "skills: {}`ncritical_paths: [auth/]"
$appended = Set-NervYamlModelsBlock -YamlText $noBlockYaml -BlockText $newBlock
Report "append-ends-with-eol" ($appended.EndsWith("`n"))
$replacedEol = Set-NervYamlModelsBlock -YamlText $yamlWithBlockLf -BlockText $newBlock
Report "replace-ends-with-eol" ($replacedEol.EndsWith("`n"))
$expectedAppended = $noBlockYaml + "`n`n" + $newBlock + "`n"
Report "set-append-when-absent" ($appended -eq $expectedAppended)

$removed = Set-NervYamlModelsBlock -YamlText $yamlWithBlockLf -BlockText ''
$expectedRemoved = "skills: {}`ncritical_paths: [auth/, payments/, migrations/, infra/]`n"
Report "set-remove-when-empty" ($removed -eq $expectedRemoved)

$yamlWithBlockCrlf = $yamlWithBlockLf -replace "`n", "`r`n"
$replacedCrlf = Set-NervYamlModelsBlock -YamlText $yamlWithBlockCrlf -BlockText $newBlock
$crlfBytes = [System.Text.Encoding]::UTF8.GetBytes($replacedCrlf)
$hasBareLf = $false
for ($i = 0; $i -lt $crlfBytes.Length; $i++) {
    if ($crlfBytes[$i] -eq 10 -and ($i -eq 0 -or $crlfBytes[$i - 1] -ne 13)) { $hasBareLf = $true; break }
}
Report "set-crlf-preserved" (-not $hasBareLf)
Report "set-crlf-content-matches-lf-result" (($replacedCrlf -replace "`r`n", "`n") -eq $expectedReplaced)

# ---------------------------------------------------------------------------
# Case group C: Get-NervModelTable
# ---------------------------------------------------------------------------
$tableDefaults = @{
    aoba   = @{ Model = 'sonnet'; Effort = 'low' }
    rei    = @{ Model = 'sonnet'; Effort = 'medium' }
    misato = @{ Model = 'fable'; Effort = 'high' }
}
$tableOverrides = @{
    aoba = @{ Model = 'haiku'; Effort = 'low' }
    rei  = @{ From = 'jd-judge-b' }
}
$fakeState = [pscustomobject]@{
    claude_phase_assignments = [pscustomobject]@{
        'jd-judge-b' = [pscustomobject]@{ model = 'opus'; effort = 'xhigh' }
    }
}

$table = Get-NervModelTable -Defaults $tableDefaults -Overrides $tableOverrides -State $fakeState
$aobaRow = $table | Where-Object { $_.role -eq 'aoba' }
$reiRow = $table | Where-Object { $_.role -eq 'rei' }
$misatoRow = $table | Where-Object { $_.role -eq 'misato' }

Report "table-override-source" ($null -ne $aobaRow -and $aobaRow.model -eq 'haiku' -and $aobaRow.effort -eq 'low' -and $aobaRow.source -eq 'override')
Report "table-gentle-ai-from-source" ($null -ne $reiRow -and $reiRow.model -eq 'opus' -and $reiRow.effort -eq 'xhigh' -and $reiRow.source -eq 'gentle-ai:jd-judge-b')
Report "table-default-source" ($null -ne $misatoRow -and $misatoRow.model -eq 'fable' -and $misatoRow.effort -eq 'high' -and $misatoRow.source -eq 'default')

# ---------------------------------------------------------------------------
# Case group D: end-to-end child-process run driven by -AnswersFile
# ---------------------------------------------------------------------------
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-configure-models-test-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null

$e2eConfigPath = Join-Path $tempRoot "nerv.yaml"
@'
enabled: true
critical_paths: [auth/, payments/, migrations/, infra/]
'@ | Set-Content -LiteralPath $e2eConfigPath -Encoding UTF8 -NoNewline

$e2eStatePath = Join-Path $tempRoot "state.json"
@'
{
  "claude_phase_assignments": {
    "jd-judge-b": { "model": "opus", "effort": "xhigh" }
  }
}
'@ | Set-Content -LiteralPath $e2eStatePath -Encoding UTF8 -NoNewline

$answersPath1 = Join-Path $tempRoot "answers-set.txt"
Set-Content -LiteralPath $answersPath1 -Value @('aoba', '3', '1', 'done', 'Y') -Encoding UTF8

$pwshExe = (Get-Process -Id $PID).Path
if (-not $pwshExe -or -not (Test-Path -LiteralPath $pwshExe)) { $pwshExe = 'pwsh' }

& $pwshExe -NoProfile -File $wizardPath -ConfigPath $e2eConfigPath -StatePath $e2eStatePath -RepoPath $repoRoot -NoApply -AnswersFile $answersPath1 | Out-Null
$e2eExit1 = $LASTEXITCODE

$configAfterSet = if (Test-Path -LiteralPath $e2eConfigPath) { Get-Content -LiteralPath $e2eConfigPath -Raw -Encoding UTF8 } else { $null }
$backupFiles = @(Get-ChildItem -LiteralPath $tempRoot -Filter "nerv.yaml.bak-models-*" -File -ErrorAction SilentlyContinue)

Report "e2e-set-exit-zero" ($e2eExit1 -eq 0)
Report "e2e-set-writes-override" ($null -ne $configAfterSet -and $configAfterSet -match '(?m)^\s*aoba: \{ model: haiku, effort: low \}\s*$')
Report "e2e-set-creates-backup" ($backupFiles.Count -ge 1)

$answersPath2 = Join-Path $tempRoot "answers-reset.txt"
Set-Content -LiteralPath $answersPath2 -Value @('reset', 'aoba', 'done', 'Y') -Encoding UTF8

& $pwshExe -NoProfile -File $wizardPath -ConfigPath $e2eConfigPath -StatePath $e2eStatePath -RepoPath $repoRoot -NoApply -AnswersFile $answersPath2 | Out-Null
$e2eExit2 = $LASTEXITCODE

$configAfterReset = if (Test-Path -LiteralPath $e2eConfigPath) { Get-Content -LiteralPath $e2eConfigPath -Raw -Encoding UTF8 } else { $null }

Report "e2e-reset-exit-zero" ($e2eExit2 -eq 0)
Report "e2e-reset-removes-override" ($null -ne $configAfterReset -and $configAfterReset -notmatch 'haiku')

# ---------------------------------------------------------------------------
# Case group E: root tools/configure-models.ps1 is a thin forwarder to
# plugin/tools/configure-models.ps1 — the same -Set/-AnswersFile run through
# the root forwarder must write the exact same override line as running the
# plugin script directly.
# ---------------------------------------------------------------------------
Report "root-forwarder-exists" (Test-Path -LiteralPath $rootForwarderPath)

if (Test-Path -LiteralPath $rootForwarderPath) {
    $e2eForwarderConfigPath = Join-Path $tempRoot "nerv-forwarder.yaml"
    @'
enabled: true
critical_paths: [auth/, payments/, migrations/, infra/]
'@ | Set-Content -LiteralPath $e2eForwarderConfigPath -Encoding UTF8 -NoNewline

    $answersPathForwarder = Join-Path $tempRoot "answers-forwarder.txt"
    Set-Content -LiteralPath $answersPathForwarder -Value @('aoba', '3', '1', 'done', 'Y') -Encoding UTF8

    & $pwshExe -NoProfile -File $rootForwarderPath -ConfigPath $e2eForwarderConfigPath -StatePath $e2eStatePath -RepoPath $repoRoot -NoApply -AnswersFile $answersPathForwarder | Out-Null
    $forwarderExit = $LASTEXITCODE

    $configAfterForwarder = if (Test-Path -LiteralPath $e2eForwarderConfigPath) { Get-Content -LiteralPath $e2eForwarderConfigPath -Raw -Encoding UTF8 } else { $null }

    Report "root-forwarder-exit-zero" ($forwarderExit -eq 0) "exit $forwarderExit"
    Report "root-forwarder-output-matches-plugin-script" ($null -ne $configAfterForwarder -and $configAfterForwarder -match '(?m)^\s*aoba: \{ model: haiku, effort: low \}\s*$')
}
else {
    Report "root-forwarder-exit-zero" $false "tools/configure-models.ps1 not found"
    Report "root-forwarder-output-matches-plugin-script" $false "tools/configure-models.ps1 not found"
}

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "Results: $script:passCount passed, $script:failCount failed"
if ($script:failCount -ne 0) { exit 1 }
exit 0
