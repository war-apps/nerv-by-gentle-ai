#requires -Version 7
<#
.SYNOPSIS
    Assertions for tools/install.ps1's -ApplyModels logic: Resolve-NervModelAssignments
    and Set-NervAgentFrontmatter. No Pester — prints PASS/FAIL lines and exits 1 on any
    failure, matching the style of tests/hook-session-start.test.sh for bash.

.EXAMPLE
    pwsh -NoProfile -File tests/install-apply-models.test.ps1
#>

$ErrorActionPreference = "Stop"

$selfDir = $PSScriptRoot
$repoRoot = Split-Path -Parent $selfDir
$installerPath = Join-Path $repoRoot "plugin/tools/install.ps1"
$rootForwarderPath = Join-Path $repoRoot "tools/install.ps1"
$agentsSourceDir = Join-Path $repoRoot "plugin/agents"

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

function Get-FrontmatterBody {
    # Returns everything after the closing "---" delimiter, newline-normalized,
    # so body comparisons ignore CRLF/LF differences and focus on content.
    param([string]$Content)
    $lines = $Content -split "`r`n|`n"
    $closeIdx = -1
    for ($i = 1; $i -lt $lines.Count; $i++) {
        if ($lines[$i].Trim() -eq '---') { $closeIdx = $i; break }
    }
    if ($closeIdx -lt 0) { return $null }
    if ($closeIdx -ge ($lines.Count - 1)) { return "" }
    return ($lines[($closeIdx + 1)..($lines.Count - 1)] -join "`n")
}

# ---------------------------------------------------------------------------
# Dot-source the installer. A bogus, nonexistent -SettingsPath guarantees that
# IF the main script body were to run (pre-guard / RED phase, before
# install.ps1 wraps its side-effecting body behind a dot-source guard), it
# throws immediately at its own "settings.json not found" check — before
# touching any real settings.json, calling gentle-ai, or calling engram. Once
# install.ps1 guards its main body behind
# `if ($MyInvocation.InvocationName -ne '.')`, this call only defines
# functions and never executes that body at all, regardless of the path.
# ---------------------------------------------------------------------------
$bogusSettingsPath = Join-Path ([System.IO.Path]::GetTempPath()) "nerv-install-test-nonexistent-settings.json"
try {
    . $installerPath -SettingsPath $bogusSettingsPath -RepoPath $repoRoot 2>$null
}
catch {
    # Expected during RED (the unguarded script throws "settings.json not
    # found"); the real signal is whether the functions got defined below.
}

$resolveAvailable = [bool](Get-Command Resolve-NervModelAssignments -ErrorAction SilentlyContinue)
$setAvailable = [bool](Get-Command Set-NervAgentFrontmatter -ErrorAction SilentlyContinue)

if (-not $resolveAvailable -or -not $setAvailable) {
    Report "functions-defined-after-dot-source" $false "Resolve-NervModelAssignments/Set-NervAgentFrontmatter not found (install.ps1 not implemented yet)"
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    exit 1
}
Report "functions-defined-after-dot-source" $true

# ---------------------------------------------------------------------------
# Fixture setup
# ---------------------------------------------------------------------------
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-apply-models-test-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null

$agentsDirA = Join-Path $tempRoot "agents-a"
New-Item -ItemType Directory -Path $agentsDirA -Force | Out-Null
foreach ($name in @("aoba.md", "misato.md", "rei.md")) {
    Copy-Item -LiteralPath (Join-Path $agentsSourceDir $name) -Destination (Join-Path $agentsDirA $name) -Force
}

$configPath = Join-Path $tempRoot "nerv.yaml"
@'
skills: {}
models:                             # per-role model and effort; project overrides user, key by key
  aoba: { model: haiku, effort: low }
# balthasar: { model: sonnet, effort: medium }   # column-0 comment, as /nerv:init writes them
#   casper: { model: sonnet, effort: medium }
  rei: { from: jd-judge-b }
  misato: { effort: bogus }
  ghost: { model: sonnet, effort: low }
critical_paths: [auth/, payments/, migrations/, infra/]
'@ | Set-Content -LiteralPath $configPath -Encoding UTF8 -NoNewline

$statePath = Join-Path $tempRoot "state.json"
@'
{
  "claude_phase_assignments": {
    "jd-judge-b": { "model": "opus", "effort": "xhigh" }
  }
}
'@ | Set-Content -LiteralPath $statePath -Encoding UTF8 -NoNewline

# Snapshot originals for later "unchanged" comparisons.
$misatoOriginalBytes = [System.IO.File]::ReadAllBytes((Join-Path $agentsDirA "misato.md"))
$aobaOriginalContent = [System.IO.File]::ReadAllText((Join-Path $agentsDirA "aoba.md"))
$reiOriginalContent = [System.IO.File]::ReadAllText((Join-Path $agentsDirA "rei.md"))
$aobaOriginalBody = Get-FrontmatterBody $aobaOriginalContent
$reiOriginalBody = Get-FrontmatterBody $reiOriginalContent

# ---------------------------------------------------------------------------
# Case group A: Resolve-NervModelAssignments
# ---------------------------------------------------------------------------
$resolveWarnings = @()
$assignments = Resolve-NervModelAssignments -ConfigPath $configPath -StatePath $statePath -WarningVariable resolveWarnings -WarningAction SilentlyContinue
$resolveWarningsText = ($resolveWarnings | ForEach-Object { $_.ToString() }) -join ' | '

Report "resolve-aoba-explicit-override" ($assignments.ContainsKey('aoba') -and $assignments['aoba']['Model'] -eq 'haiku' -and $assignments['aoba']['Effort'] -eq 'low')
Report "resolve-rei-from-state-json" ($assignments.ContainsKey('rei') -and $assignments['rei']['Model'] -eq 'opus' -and $assignments['rei']['Effort'] -eq 'xhigh')
Report "resolve-misato-invalid-effort-skipped" (-not $assignments.ContainsKey('misato'))
Report "resolve-warns-on-invalid-effort" ($resolveWarningsText -match 'misato')
Report "resolve-ghost-present-despite-no-file" ($assignments.ContainsKey('ghost') -and $assignments['ghost']['Model'] -eq 'sonnet' -and $assignments['ghost']['Effort'] -eq 'low')

# ---------------------------------------------------------------------------
# Case group B: Set-NervAgentFrontmatter (Case A fixtures)
# ---------------------------------------------------------------------------
$applyWarnings = @()
Set-NervAgentFrontmatter -AgentsDir $agentsDirA -Assignments $assignments -WarningVariable applyWarnings -WarningAction SilentlyContinue | Out-Null
$applyWarningsText = ($applyWarnings | ForEach-Object { $_.ToString() }) -join ' | '

$aobaContent = [System.IO.File]::ReadAllText((Join-Path $agentsDirA "aoba.md"))
$reiContent = [System.IO.File]::ReadAllText((Join-Path $agentsDirA "rei.md"))
$misatoBytes = [System.IO.File]::ReadAllBytes((Join-Path $agentsDirA "misato.md"))

Report "aoba-model-updated" ($aobaContent -match '(?m)^model:\s*haiku\s*$')
Report "aoba-effort-updated" ($aobaContent -match '(?m)^effort:\s*low\s*$')
Report "rei-model-updated" ($reiContent -match '(?m)^model:\s*opus\s*$')
Report "rei-effort-updated" ($reiContent -match '(?m)^effort:\s*xhigh\s*$')
Report "misato-untouched" ([System.Linq.Enumerable]::SequenceEqual([byte[]]$misatoOriginalBytes, [byte[]]$misatoBytes))
Report "ghost-warns-missing-file" ($applyWarningsText -match 'ghost')

$aobaBody = Get-FrontmatterBody $aobaContent
$reiBody = Get-FrontmatterBody $reiContent
Report "aoba-body-unchanged" ($aobaBody -eq $aobaOriginalBody)
Report "rei-body-unchanged" ($reiBody -eq $reiOriginalBody)

# ---------------------------------------------------------------------------
# Case group C: trailing-comment preservation + CRLF preservation (isolated
# fixture — misato's model line carries a long trailing "# ..." comment)
# ---------------------------------------------------------------------------
$agentsDirC = Join-Path $tempRoot "agents-c"
New-Item -ItemType Directory -Path $agentsDirC -Force | Out-Null
$misatoSourceText = [System.IO.File]::ReadAllText((Join-Path $agentsSourceDir "misato.md"))
# Force CRLF regardless of the source file's own line endings, so this case
# genuinely exercises CRLF preservation end to end.
$misatoCrlfText = ($misatoSourceText -split "`r`n|`n") -join "`r`n"
$misatoCrlfPath = Join-Path $agentsDirC "misato.md"
[System.IO.File]::WriteAllText($misatoCrlfPath, $misatoCrlfText, (New-Object System.Text.UTF8Encoding($false)))

$commentLineBefore = ($misatoCrlfText -split "`r`n") | Where-Object { $_ -match '^model:\s*fable' } | Select-Object -First 1
$commentText = $null
if ($commentLineBefore -match '(#.*)$') { $commentText = $Matches[1] }

Set-NervAgentFrontmatter -AgentsDir $agentsDirC -Assignments @{ misato = @{ Model = 'opus' } } -WarningAction SilentlyContinue | Out-Null

$misatoCrlfBytes = [System.IO.File]::ReadAllBytes($misatoCrlfPath)
$misatoCrlfAfterText = [System.IO.File]::ReadAllText($misatoCrlfPath)

Report "misato-model-changed-to-opus" ($misatoCrlfAfterText -match '(?m)^model:\s*opus')
Report "misato-comment-preserved" ($null -ne $commentText -and $misatoCrlfAfterText.Contains($commentText))

$hasBareLf = $false
for ($i = 0; $i -lt $misatoCrlfBytes.Length; $i++) {
    if ($misatoCrlfBytes[$i] -eq 10 -and ($i -eq 0 -or $misatoCrlfBytes[$i - 1] -ne 13)) {
        $hasBareLf = $true
        break
    }
}
Report "misato-crlf-preserved" (-not $hasBareLf)

# ---------------------------------------------------------------------------
# Case group D: plugin defaults restore a role whose override was removed
# ---------------------------------------------------------------------------
$agentsDirD = Join-Path $tempRoot "agents-d"
New-Item -ItemType Directory -Path $agentsDirD | Out-Null
$aobaDPath = Join-Path $agentsDirD "aoba.md"
Copy-Item -LiteralPath (Join-Path $agentsSourceDir "aoba.md") -Destination $aobaDPath
# Simulate a cache that still carries a previous override (haiku/low).
Set-NervAgentFrontmatter -AgentsDir $agentsDirD -Assignments @{ aoba = @{ Model = 'haiku'; Effort = 'low' } } -WarningAction SilentlyContinue | Out-Null
$defaultsAvailable = [bool](Get-Command Get-NervPluginDefaults -ErrorAction SilentlyContinue)
Report "get-plugin-defaults-defined" $defaultsAvailable
if ($defaultsAvailable) {
    $defaults = Get-NervPluginDefaults -AgentsDir $agentsSourceDir
    Report "plugin-defaults-aoba-sonnet-low" ($defaults.ContainsKey('aoba') -and $defaults['aoba']['Model'] -eq 'sonnet' -and $defaults['aoba']['Effort'] -eq 'low')
    Report "plugin-defaults-count-18" ($defaults.Count -eq 18)
    $merged = Merge-NervModelAssignments -Defaults $defaults -Overrides @{}
    Set-NervAgentFrontmatter -AgentsDir $agentsDirD -Assignments $merged -WarningAction SilentlyContinue | Out-Null
    $aobaDText = [System.IO.File]::ReadAllText($aobaDPath)
    Report "removed-override-restores-default" (($aobaDText -match '(?m)^model:\s*sonnet') -and ($aobaDText -match '(?m)^effort:\s*low'))
    $merged2 = Merge-NervModelAssignments -Defaults $defaults -Overrides @{ aoba = @{ Model = 'haiku' } }
    Report "override-merges-over-default" ($merged2['aoba']['Model'] -eq 'haiku' -and $merged2['aoba']['Effort'] -eq 'low')
}
else {
    Report "plugin-defaults-aoba-sonnet-low" $false "Get-NervPluginDefaults missing"
    Report "plugin-defaults-count-18" $false "Get-NervPluginDefaults missing"
    Report "removed-override-restores-default" $false "Merge-NervModelAssignments missing"
    Report "override-merges-over-default" $false "Merge-NervModelAssignments missing"
}

# ---------------------------------------------------------------------------
# Case group E: root tools/install.ps1 is a thin forwarder to
# plugin/tools/install.ps1 — a bogus, nonexistent -SettingsPath makes the
# real script throw its own "settings.json not found" error before touching
# any real file, gentle-ai, or engram; the forwarder must produce the exact
# same error text (proving -SettingsPath/-RepoPath were forwarded) and the
# same non-zero exit code.
# ---------------------------------------------------------------------------
Report "root-forwarder-exists" (Test-Path -LiteralPath $rootForwarderPath)

if (Test-Path -LiteralPath $rootForwarderPath) {
    $bogusSettingsPathE = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-install-forwarder-test-nonexistent-" + [Guid]::NewGuid().ToString("N") + ".json")

    $outputForwarder = & pwsh -NoProfile -File $rootForwarderPath -SettingsPath $bogusSettingsPathE -RepoPath $repoRoot 2>&1
    $exitForwarder = $LASTEXITCODE
    $outputDirect = & pwsh -NoProfile -File $installerPath -SettingsPath $bogusSettingsPathE -RepoPath $repoRoot 2>&1
    $exitDirect = $LASTEXITCODE

    $forwarderText = ($outputForwarder | ForEach-Object { [string]$_ }) -join "`n"
    $directText = ($outputDirect | ForEach-Object { [string]$_ }) -join "`n"

    Report "root-forwarder-exit-matches" ($exitForwarder -eq $exitDirect -and $exitForwarder -ne 0) "forwarder $exitForwarder vs direct $exitDirect"
    Report "root-forwarder-error-names-bogus-path" ($forwarderText -match [regex]::Escape($bogusSettingsPathE)) "output: $($forwarderText.Substring(0, [Math]::Min(200, $forwarderText.Length)))"
    Report "root-forwarder-output-matches-plugin-script" ($forwarderText -eq $directText)
}
else {
    Report "root-forwarder-exit-matches" $false "tools/install.ps1 not found"
    Report "root-forwarder-error-names-bogus-path" $false "tools/install.ps1 not found"
    Report "root-forwarder-output-matches-plugin-script" $false "tools/install.ps1 not found"
}

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "Results: $script:passCount passed, $script:failCount failed"
if ($script:failCount -ne 0) { exit 1 }
exit 0
