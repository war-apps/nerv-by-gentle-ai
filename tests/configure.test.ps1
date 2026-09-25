#requires -Version 7
<#
.SYNOPSIS
    Assertions for tools/configure.ps1: the pure YAML/formatting functions
    (Set-NervYamlBlock, Set-NervYamlScalar, Get-NervYamlBlock,
    Get-NervYamlSubBlock, Read-NervScalar, Format-NervGitBlock,
    Format-NervTasksBlock, Format-NervSkillsBlock,
    Format-NervCriticalPathsLine, Format-NervArtifactsBlock,
    Format-NervProjectFile) plus end-to-end child-process runs of the
    interactive wizard driven by -AnswersFile. No Pester — prints PASS/FAIL
    (and SKIP) lines and exits 1 on any failure, matching
    tests/configure-models.test.ps1's style.

.EXAMPLE
    pwsh -NoProfile -File tests/configure.test.ps1
#>

$ErrorActionPreference = "Stop"

$selfDir = $PSScriptRoot
$repoRoot = Split-Path -Parent $selfDir
$wizardPath = Join-Path $repoRoot "tools/configure.ps1"

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

function ReportSkip {
    param([string]$CaseName, [string]$Detail = "")
    $suffix = if ($Detail) { " - $Detail" } else { "" }
    Write-Host "SKIP $CaseName$suffix"
}

# ---------------------------------------------------------------------------
# Dot-source the wizard. Its interactive body is guarded by
# `if ($MyInvocation.InvocationName -ne '.')` (same pattern as
# tools/install.ps1 and tools/configure-models.ps1), so dot-sourcing only
# defines functions and never prompts, writes, or applies anything for real.
# ---------------------------------------------------------------------------
if (-not (Test-Path -LiteralPath $wizardPath)) {
    Report "wizard-script-exists" $false "tools/configure.ps1 not found (not implemented yet)"
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    exit 1
}
Report "wizard-script-exists" $true

$bogusConfigPath = Join-Path ([System.IO.Path]::GetTempPath()) "nerv-configure-test-nonexistent.yaml"
$bogusHomeDir = Join-Path ([System.IO.Path]::GetTempPath()) "nerv-configure-test-nonexistent-home"
try {
    . $wizardPath -RepoPath $repoRoot -HomeDir $bogusHomeDir -ConfigPath $bogusConfigPath -SkipSkills -SkipModels -SkipRepos -SkipCommands -NoRefresh -AnswersFile $bogusConfigPath 2>$null
}
catch {
    # Expected during RED (before the interactive body is guarded); the real
    # signal is whether the functions got defined below.
}

$requiredFunctions = @(
    'Set-NervYamlBlock',
    'Set-NervYamlScalar',
    'Get-NervYamlBlock',
    'Get-NervYamlSubBlock',
    'Read-NervScalar',
    'Format-NervGitBlock',
    'Format-NervTasksBlock',
    'Format-NervSkillsBlock',
    'Format-NervCriticalPathsLine',
    'Format-NervArtifactsBlock',
    'Format-NervProjectFile'
)
$allDefined = $true
foreach ($fn in $requiredFunctions) {
    if (-not (Get-Command $fn -ErrorAction SilentlyContinue)) { $allDefined = $false }
}

if (-not $allDefined) {
    Report "functions-defined-after-dot-source" $false "one or more pure functions not found (tools/configure.ps1 not implemented yet, or Set-NervYamlScalar missing)"
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    exit 1
}
Report "functions-defined-after-dot-source" $true

# ---------------------------------------------------------------------------
# Fixture mirroring the real nerv.yaml shape: models, skills, critical_paths,
# artifacts, git, and tasks: containing providers.teamwork (task_ref_prefix,
# assignee_id, default_project_id, default_tasklist_id, stages, and a
# known_projects: list of maps with note: strings the wizard has no field
# for), providers.github-projects/jira, a sources: list, and a
# sources_howto: literal block NESTED UNDER tasks: (not top-level) — plus
# trailing comments on several scalar lines throughout. Deliberately out of
# the documented key order, to exercise the scan-based (not order-dependent)
# block functions. Section 1 must never regenerate an existing block, so
# known_projects/sources/sources_howto (fields the wizard has no prompt for)
# must always survive byte for byte.
# ---------------------------------------------------------------------------
$fixtureLf = (
    "enabled: true`n" +
    "skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md`n" +
    "  testing: [tdd, playwright-best-practices]                        # ritsuko, kaworu, maya`n" +
    "  code: [dotnet-best-practices, typescript-best-practices]         # pilots`n" +
    "  best-practices: [best-practices, solid-principles, clean-code-guard]  # balthasar`n" +
    "  architecture: [hexagonal-architecture, c4-architecture]          # melchor`n" +
    "  audit: [security-review, clean-code-guard]                       # kaji passes`n" +
    "models:                             # per-role model and effort; project overrides user, key by key`n" +
    "  misato: { model: fable, effort: high }`n" +
    "  melchor: { from: jd-judge-b }     # inherit gentle-ai's assignment for that phase (state.json)`n" +
    "critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical`n" +
    "artifacts:`n" +
    "  commit: at-close                  # with-change | at-close | never (default: at-close)`n" +
    "git:`n" +
    "  base_branch: develop              # default base for the worktree offer`n" +
    "  worktree: ask                     # ask | always | never`n" +
    "  branch_pattern: `"feature/{prefix}-{id}-{slug}`"   # prefix comes from the provider (tw, gh, jira)`n" +
    "  commit_ref_pattern: `"({PREFIX}-{id})`"`n" +
    "tasks:`n" +
    "  provider: teamwork                # teamwork | github-projects | jira | none ; `"ask`" when absent`n" +
    "  ask_when_missing: true            # preflight asks task + worktree + branch if no active task`n" +
    "  subtasks_per_wave: false`n" +
    "  timer_store: ~/.claude/work/timers.json`n" +
    "  rounding_minutes: 15`n" +
    "  providers:                        # one block per provider, only the enabled one is required`n" +
    "    teamwork:`n" +
    "      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern`n" +
    "      assignee_id: 686035           # user scope`n" +
    "      default_project_id: 1271726`n" +
    "      default_tasklist_id: 3951970`n" +
    "      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }`n" +
    "      known_projects:                 # extra Teamwork projects seen before, for quick lookup`n" +
    "        - id: 1271726`n" +
    "          name: ERP Proveedores`n" +
    "          note: `"primary project for this team`"`n" +
    "        - id: 1300000`n" +
    "          name: Infra`n" +
    "          note: `"shared infra tasks, rarely used`"`n" +
    "        - id: 1400000`n" +
    "          name: Docs`n" +
    "          note: `"documentation backlog, low priority`"`n" +
    "        - id: 1500000`n" +
    "          name: Legacy`n" +
    "          note: `"read-only archive, do not assign`"`n" +
    "    github-projects: { task_ref_prefix: gh, owner: `"`", project_number: 0 }    # later`n" +
    "    jira: { task_ref_prefix: jira, site: `"`", project_key: `"`" }               # later`n" +
    "  sources:                          # extra work sources for listings (replaces ~/.claude/work/sources.md)`n" +
    "    - name: erp-proveedores`n" +
    "      type: google-sheets`n" +
    "      sheet_id: abc123`n" +
    "    - name: another-source`n" +
    "      type: csv`n" +
    "      path: /data/x.csv`n" +
    "  sources_howto: |`n" +
    "    How to add a new source:`n" +
    "    1. Pick a name.`n" +
    "    2. Pick a type.`n" +
    "    3. Fill in the fields.`n"
)

# ---------------------------------------------------------------------------
# Case group A: Set-NervYamlBlock (generalized Set-NervYamlModelsBlock)
# ---------------------------------------------------------------------------
$newGitBlock = "git:`n  base_branch: main`n  worktree: always"
$replacedGit = Set-NervYamlBlock -YamlText $fixtureLf -Key 'git' -BlockText $newGitBlock
Report "block-replace-contains-new" ($replacedGit -match [regex]::Escape($newGitBlock))
Report "block-replace-drops-old" ((Get-NervYamlBlock -YamlText $replacedGit -Key 'git') -notmatch 'commit_ref_pattern')
Report "block-replace-preserves-unrelated" (($replacedGit -match 'sources_howto: \|') -and ($replacedGit -match 'known_projects:'))
Report "block-replace-ends-with-eol" ($replacedGit.EndsWith("`n"))

$noKeyYaml = "skills: {}`ncritical_paths: [auth/]"
$appendedBlock = Set-NervYamlBlock -YamlText $noKeyYaml -Key 'git' -BlockText $newGitBlock
$expectedAppended = $noKeyYaml + "`n`n" + $newGitBlock + "`n"
Report "block-append-when-absent" ($appendedBlock -eq $expectedAppended)

$removedBlock = Set-NervYamlBlock -YamlText $fixtureLf -Key 'artifacts' -BlockText ''
Report "block-remove-when-empty" ($removedBlock -notmatch '(?m)^artifacts:')
Report "block-remove-preserves-rest" (($removedBlock -match 'git:') -and ($removedBlock -match 'tasks:'))

$fixtureCrlf = $fixtureLf -replace "`n", "`r`n"
$replacedCrlf = Set-NervYamlBlock -YamlText $fixtureCrlf -Key 'git' -BlockText $newGitBlock
$crlfBytes = [System.Text.Encoding]::UTF8.GetBytes($replacedCrlf)
$hasBareLf = $false
for ($i = 0; $i -lt $crlfBytes.Length; $i++) {
    if ($crlfBytes[$i] -eq 10 -and ($i -eq 0 -or $crlfBytes[$i - 1] -ne 13)) { $hasBareLf = $true; break }
}
Report "block-crlf-preserved" (-not $hasBareLf)

# models: handling must delegate to the existing Set-NervYamlModelsBlock
$newModelsBlock = "models:`n  aoba: { model: haiku, effort: low }"
$replacedModelsViaGeneric = Set-NervYamlBlock -YamlText $fixtureLf -Key 'models' -BlockText $newModelsBlock
$replacedModelsViaOriginal = Set-NervYamlModelsBlock -YamlText $fixtureLf -BlockText $newModelsBlock
Report "block-models-delegates-to-existing-function" ($replacedModelsViaGeneric -eq $replacedModelsViaOriginal)

# ---------------------------------------------------------------------------
# Case group B: Get-NervYamlBlock / Get-NervYamlSubBlock
# ---------------------------------------------------------------------------
$gitBlock = Get-NervYamlBlock -YamlText $fixtureLf -Key 'git'
Report "get-block-git" ($null -ne $gitBlock -and $gitBlock -match 'base_branch: develop' -and $gitBlock -notmatch 'tasks:')

$tasksBlock = Get-NervYamlBlock -YamlText $fixtureLf -Key 'tasks'
Report "get-block-tasks-includes-nested-content" (
    $null -ne $tasksBlock -and
    $tasksBlock -match 'assignee_id: 686035' -and
    $tasksBlock -match 'erp-proveedores' -and
    $tasksBlock -match 'known_projects:' -and
    $tasksBlock -match 'sources_howto: \|'
)

Report "get-block-sources-howto-not-top-level" ($null -eq (Get-NervYamlBlock -YamlText $fixtureLf -Key 'sources_howto'))

$missingBlock = Get-NervYamlBlock -YamlText $fixtureLf -Key 'does_not_exist'
Report "get-block-missing-returns-null" ($null -eq $missingBlock)

$sourcesSubBlock = Get-NervYamlSubBlock -BlockText $tasksBlock -Key 'sources' -Indent 2
Report "get-subblock-sources-present" (
    $null -ne $sourcesSubBlock -and
    $sourcesSubBlock -match '- name: erp-proveedores' -and
    $sourcesSubBlock -match '- name: another-source' -and
    $sourcesSubBlock -notmatch 'providers:'
)

$sourcesHowtoSubBlock = Get-NervYamlSubBlock -BlockText $tasksBlock -Key 'sources_howto' -Indent 2
Report "get-subblock-sources-howto-nested" ($null -ne $sourcesHowtoSubBlock -and $sourcesHowtoSubBlock -match 'Pick a name')

$providersSubBlock = Get-NervYamlSubBlock -BlockText $tasksBlock -Key 'providers' -Indent 2
$teamworkSubBlock = if ($providersSubBlock) { Get-NervYamlSubBlock -BlockText $providersSubBlock -Key 'teamwork' -Indent 4 } else { $null }
Report "get-subblock-teamwork-nested" ($null -ne $teamworkSubBlock -and $teamworkSubBlock -match 'assignee_id: 686035')

$knownProjectsSubBlock = if ($teamworkSubBlock) { Get-NervYamlSubBlock -BlockText $teamworkSubBlock -Key 'known_projects' -Indent 6 } else { $null }
Report "get-subblock-known-projects-nested" ($null -ne $knownProjectsSubBlock -and $knownProjectsSubBlock -match 'ERP Proveedores' -and $knownProjectsSubBlock -match 'Legacy')

$missingSubBlock = Get-NervYamlSubBlock -BlockText $tasksBlock -Key 'nope' -Indent 2
Report "get-subblock-missing-returns-null" ($null -eq $missingSubBlock)

# ---------------------------------------------------------------------------
# Case group C: Read-NervScalar
# ---------------------------------------------------------------------------
Report "scalar-git-base-branch" ((Read-NervScalar -YamlText $fixtureLf -Path 'git.base_branch') -eq 'develop')
Report "scalar-nested-assignee-id" ((Read-NervScalar -YamlText $fixtureLf -Path 'tasks.providers.teamwork.assignee_id') -eq '686035')
Report "scalar-missing-path" ($null -eq (Read-NervScalar -YamlText $fixtureLf -Path 'tasks.providers.teamwork.nonexistent'))
Report "scalar-missing-top-key" ($null -eq (Read-NervScalar -YamlText $fixtureLf -Path 'does.not.exist'))

# ---------------------------------------------------------------------------
# Case group D: Format-* functions
# ---------------------------------------------------------------------------
$gitFormatted = Format-NervGitBlock -Values @{
    base_branch        = 'develop'
    worktree            = 'ask'
    branch_pattern      = 'feature/{prefix}-{id}-{slug}'
    commit_ref_pattern  = '({PREFIX}-{id})'
}
Report "format-git-header" ($gitFormatted -match '(?m)^git:')
Report "format-git-base-branch" ($gitFormatted -match 'base_branch: develop')
Report "format-git-worktree" ($gitFormatted -match 'worktree: ask')
Report "format-git-branch-pattern-quoted" ($gitFormatted -match 'branch_pattern: "feature/\{prefix\}-\{id\}-\{slug\}"')

$sourcesRawForFormat = "  sources:`n    - name: erp-proveedores`n      type: google-sheets"
$tasksFormatted = Format-NervTasksBlock -Values @{
    provider                     = 'teamwork'
    ask_when_missing             = 'true'
    subtasks_per_wave            = 'false'
    timer_store                  = '~/.claude/work/timers.json'
    rounding_minutes             = '15'
    teamwork_task_ref_prefix     = 'tw'
    teamwork_assignee_id         = '686035'
    teamwork_default_project_id  = '1271726'
    teamwork_default_tasklist_id = '3951970'
    teamwork_stage_inDev         = 'DESARROLLO'
    teamwork_stage_testing       = 'TESTING'
    teamwork_stage_implemented   = 'IMPLEMENTA'
    teamwork_stage_blocked       = 'BLOQUEA'
    teamwork_stage_canceled      = 'CANCEL'
    teamwork_stage_pending       = 'PENDIENTE'
    teamwork_stage_analysis      = 'ANALISIS'
} -SourcesRaw $sourcesRawForFormat
Report "format-tasks-header" ($tasksFormatted -match '(?m)^tasks:')
Report "format-tasks-provider" ($tasksFormatted -match 'provider: teamwork')
Report "format-tasks-teamwork-block" ($tasksFormatted -match 'task_ref_prefix: tw' -and $tasksFormatted -match 'assignee_id: 686035')
Report "format-tasks-stages" ($tasksFormatted -match 'inDev: DESARROLLO' -and $tasksFormatted -match 'analysis: ANALISIS')
Report "format-tasks-other-providers-present" ($tasksFormatted -match 'github-projects:' -and $tasksFormatted -match 'jira:')
Report "format-tasks-carries-sources-verbatim" ($tasksFormatted.Contains($sourcesRawForFormat))

$skillsFormatted = Format-NervSkillsBlock -Values @{
    testing           = 'tdd, playwright-best-practices'
    code              = 'dotnet-best-practices, typescript-best-practices'
    'best-practices'  = 'best-practices, solid-principles, clean-code-guard'
    architecture      = 'hexagonal-architecture, c4-architecture'
    audit             = 'security-review, clean-code-guard'
}
Report "format-skills-header" ($skillsFormatted -match '(?m)^skills:')
Report "format-skills-testing" ($skillsFormatted -match '\[tdd, playwright-best-practices\]')
Report "format-skills-audit" ($skillsFormatted -match 'audit: \[security-review, clean-code-guard\]')

$criticalPathsLine = Format-NervCriticalPathsLine -Paths @('auth/', 'payments/', 'migrations/', 'infra/')
Report "format-critical-paths" ($criticalPathsLine -match '^critical_paths: \[auth/, payments/, migrations/, infra/\]')

$artifactsFormatted = Format-NervArtifactsBlock -Commit 'at-close'
Report "format-artifacts" ($artifactsFormatted -match '(?m)^artifacts:' -and $artifactsFormatted -match 'commit: at-close')

$projectFile = Format-NervProjectFile -Values @{
    base_branch = 'develop2'
    provider    = 'teamwork'
    project_id  = '111'
    tasklist_id = '222'
    commit      = 'at-close'
}
Report "format-project-enabled" ($projectFile -match '(?m)^enabled: true')
Report "format-project-base-branch" ($projectFile -match 'base_branch: develop2')
Report "format-project-provider" ($projectFile -match 'provider: teamwork')
Report "format-project-project-id" ($projectFile -match 'project_id: 111')
Report "format-project-tasklist-id" ($projectFile -match 'tasklist_id: 222')
Report "format-project-models-hint" ($projectFile -match '# ?models:')

$projectFileMinimal = Format-NervProjectFile -Values @{}
Report "format-project-minimal-enabled-only" ($projectFileMinimal -match '(?m)^enabled: true' -and $projectFileMinimal -notmatch 'project_id')

# ---------------------------------------------------------------------------
# Case group E: Set-NervYamlScalar
# ---------------------------------------------------------------------------

# (e1) existing key with a trailing comment: value replaced, comment kept verbatim.
$scalarReplaced = Set-NervYamlScalar -YamlText $fixtureLf -Path 'git.base_branch' -Value 'develop2'
Report "scalar-set-existing-value-replaced" ($scalarReplaced -match '(?m)^  base_branch: develop2\s')
Report "scalar-set-existing-comment-kept" ($scalarReplaced -match '(?m)^  base_branch: develop2\s+# default base for the worktree offer$')
Report "scalar-set-existing-only-one-line-changed" (
    (@($fixtureLf -split "`n") | Where-Object { $_ -ne '' }).Count -eq (@($scalarReplaced -split "`n") | Where-Object { $_ -ne '' }).Count
)

# -Comment is ignored when the line already exists — the original comment survives.
$scalarReplacedIgnoresComment = Set-NervYamlScalar -YamlText $fixtureLf -Path 'git.base_branch' -Value 'develop3' -Comment 'ignored'
Report "scalar-set-existing-comment-param-ignored" (
    $scalarReplacedIgnoresComment -match '# default base for the worktree offer' -and
    $scalarReplacedIgnoresComment -notmatch 'ignored'
)

# nested existing key, no comment on the line.
$scalarNestedReplaced = Set-NervYamlScalar -YamlText $fixtureLf -Path 'tasks.providers.teamwork.assignee_id' -Value '999999'
Report "scalar-set-nested-existing-value-replaced" ($scalarNestedReplaced -match '(?m)^      assignee_id: 999999\s+# user scope$')
Report "scalar-set-nested-preserves-known-projects" ($scalarNestedReplaced -match 'ERP Proveedores' -and $scalarNestedReplaced -match 'Legacy')
Report "scalar-set-nested-preserves-sources-howto" ($scalarNestedReplaced -match 'Pick a name')

# (e2) missing leaf appended at the end of its parent block, before the next top-level key.
$minimalWithArtifacts = "artifacts:`n  commit: at-close`ngit:`n  base_branch: develop`n"
$leafAppended = Set-NervYamlScalar -YamlText $minimalWithArtifacts -Path 'artifacts.new_field' -Value 'hello'
Report "scalar-set-missing-leaf-appended" ($leafAppended -match '(?m)^  new_field: hello$')
Report "scalar-set-missing-leaf-before-next-top-key" (
    $leafAppended -match '(?ms)commit: at-close\r?\n  new_field: hello\r?\ngit:'
)
Report "scalar-set-missing-leaf-does-not-disturb-git" ($leafAppended -match '(?m)^  base_branch: develop$')

# (e3) missing parent chain created (providers: and teamwork: both absent under tasks:).
$minimalTasks = "tasks:`n  provider: teamwork`n"
$chainCreated = Set-NervYamlScalar -YamlText $minimalTasks -Path 'tasks.providers.teamwork.new_field' -Value 'x'
Report "scalar-set-chain-creates-providers" ($chainCreated -match '(?m)^  providers:$')
Report "scalar-set-chain-creates-teamwork" ($chainCreated -match '(?m)^    teamwork:$')
Report "scalar-set-chain-creates-leaf" ($chainCreated -match '(?m)^      new_field: x$')
$chainTasksBlock = Get-NervYamlBlock -YamlText $chainCreated -Key 'tasks'
$chainProvidersSub = Get-NervYamlSubBlock -BlockText $chainTasksBlock -Key 'providers' -Indent 2
$chainTeamworkSub = if ($chainProvidersSub) { Get-NervYamlSubBlock -BlockText $chainProvidersSub -Key 'teamwork' -Indent 4 } else { $null }
Report "scalar-set-chain-nested-correctly" ($null -ne $chainTeamworkSub -and $chainTeamworkSub -match 'new_field: x')

# (e4) quoting rules.
$minimalGit = "git:`n  base_branch: develop`n"
$quotedSpecial = Set-NervYamlScalar -YamlText $minimalGit -Path 'git.new_pattern' -Value 'feature/{prefix}'
Report "scalar-set-quotes-special-chars" ($quotedSpecial -match '(?m)^  new_pattern: "feature/\{prefix\}"$')

$bareBool = Set-NervYamlScalar -YamlText $minimalGit -Path 'git.new_flag' -Value 'true'
Report "scalar-set-bare-boolean" ($bareBool -match '(?m)^  new_flag: true$')

$bareInt = Set-NervYamlScalar -YamlText $minimalGit -Path 'git.new_count' -Value '42'
Report "scalar-set-bare-integer" ($bareInt -match '(?m)^  new_count: 42$')

$bareNull = Set-NervYamlScalar -YamlText $minimalGit -Path 'git.new_null' -Value 'null'
Report "scalar-set-bare-null" ($bareNull -match '(?m)^  new_null: null$')

$rawUnquoted = Set-NervYamlScalar -YamlText $minimalGit -Path 'git.new_list' -Value '[a, b]' -Raw
Report "scalar-set-raw-bypasses-quoting" ($rawUnquoted -match '(?m)^  new_list: \[a, b\]$')

# -Comment applies only to a genuinely new (appended) line.
$newWithComment = Set-NervYamlScalar -YamlText $minimalGit -Path 'git.new_with_comment' -Value 'x' -Comment 'hello world'
Report "scalar-set-new-line-comment-applied" ($newWithComment -match '(?m)^  new_with_comment: x  # hello world$')

# ---------------------------------------------------------------------------
# Case group F: end-to-end child-process run with an EMPTY answers file
# (keep everything, Write confirm defaults to Y) — the written user config
# must be BYTE-IDENTICAL to the fixture. This is the regression test for the
# real-world bug report: section 1 used to regenerate the managed blocks
# from a fixed field list on every run, silently dropping unknown content
# (known_projects, sources_howto, ...) and every trailing comment.
# ---------------------------------------------------------------------------
$tempRootEmpty = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-configure-test-empty-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRootEmpty -Force | Out-Null
$tempHomeEmpty = Join-Path $tempRootEmpty "home"
New-Item -ItemType Directory -Path $tempHomeEmpty -Force | Out-Null

$e2eEmptyConfigPath = Join-Path $tempRootEmpty "nerv.yaml"
[System.IO.File]::WriteAllText($e2eEmptyConfigPath, $fixtureLf, (New-Object System.Text.UTF8Encoding($false)))

$emptyAnswersPath = Join-Path $tempRootEmpty "answers.txt"
Set-Content -LiteralPath $emptyAnswersPath -Value @() -Encoding UTF8

$pwshExe = (Get-Process -Id $PID).Path
if (-not $pwshExe -or -not (Test-Path -LiteralPath $pwshExe)) { $pwshExe = 'pwsh' }

& $pwshExe -NoProfile -File $wizardPath -HomeDir $tempHomeEmpty -ConfigPath $e2eEmptyConfigPath -SkipSkills -SkipModels -SkipRepos -SkipCommands -NoRefresh -AnswersFile $emptyAnswersPath | Out-Null
$e2eEmptyExit = $LASTEXITCODE
Report "e2e-empty-exit-zero" ($e2eEmptyExit -eq 0)

$configAfterEmpty = if (Test-Path -LiteralPath $e2eEmptyConfigPath) { Get-Content -LiteralPath $e2eEmptyConfigPath -Raw -Encoding UTF8 } else { $null }
Report "e2e-empty-answers-byte-identical" ($null -ne $configAfterEmpty -and $configAfterEmpty -ceq $fixtureLf)

Remove-Item -LiteralPath $tempRootEmpty -Recurse -Force -ErrorAction SilentlyContinue

# ---------------------------------------------------------------------------
# Case group G: end-to-end child-process run changing ONLY git.base_branch —
# the resulting user config must differ from the fixture in exactly one
# line — plus repo init and slash-commands install, driven by -AnswersFile.
# ---------------------------------------------------------------------------
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-configure-test-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null

$tempHome = Join-Path $tempRoot "home"
New-Item -ItemType Directory -Path $tempHome -Force | Out-Null

$e2eConfigPath = Join-Path $tempRoot "nerv.yaml"
[System.IO.File]::WriteAllText($e2eConfigPath, $fixtureLf, (New-Object System.Text.UTF8Encoding($false)))

$tempGitRepo = Join-Path $tempRoot "repo"
New-Item -ItemType Directory -Path $tempGitRepo -Force | Out-Null
& git init -q $tempGitRepo 2>$null | Out-Null

$answers = @()
$answers += 'develop2'      # git.base_branch
$answers += ''               # git.worktree
$answers += ''               # git.branch_pattern
$answers += ''               # git.commit_ref_pattern
$answers += ''               # tasks.provider
$answers += ''               # tasks.ask_when_missing
$answers += ''               # tasks.subtasks_per_wave
$answers += ''               # tasks.timer_store
$answers += ''               # tasks.rounding_minutes
$answers += ''               # tasks.providers.teamwork.task_ref_prefix
$answers += ''               # tasks.providers.teamwork.assignee_id
$answers += ''               # tasks.providers.teamwork.default_project_id
$answers += ''               # tasks.providers.teamwork.default_tasklist_id
$answers += ''               # stage: inDev
$answers += ''               # stage: testing
$answers += ''               # stage: implemented
$answers += ''               # stage: blocked
$answers += ''               # stage: canceled
$answers += ''               # stage: pending
$answers += ''               # stage: analysis
$answers += ''               # skills.testing
$answers += ''               # skills.code
$answers += ''               # skills.best-practices
$answers += ''               # skills.architecture
$answers += ''               # skills.audit
$answers += ''               # critical_paths
$answers += ''               # artifacts.commit
$answers += ''               # write confirm [Y/n]
$answers += $tempGitRepo      # repo path
$answers += ''               # repo base branch (keep)
$answers += ''               # repo task provider (keep teamwork)
$answers += '111'            # repo project_id
$answers += '222'            # repo tasklist_id
$answers += ''               # repo path again -> skip
$answers += 'y'               # install slash commands

$answersPath = Join-Path $tempRoot "answers.txt"
Set-Content -LiteralPath $answersPath -Value $answers -Encoding UTF8

& $pwshExe -NoProfile -File $wizardPath -HomeDir $tempHome -ConfigPath $e2eConfigPath -SkipSkills -SkipModels -NoRefresh -AnswersFile $answersPath | Out-Null
$e2eExit = $LASTEXITCODE

Report "e2e-exit-zero" ($e2eExit -eq 0)

$configAfter = if (Test-Path -LiteralPath $e2eConfigPath) { Get-Content -LiteralPath $e2eConfigPath -Raw -Encoding UTF8 } else { $null }
Report "e2e-base-branch-changed" ($null -ne $configAfter -and $configAfter -match 'base_branch: develop2')

if ($null -ne $configAfter) {
    $fixtureLines = @($fixtureLf -split "`n")
    $afterLines = @($configAfter -split "`n")
    $maxLen = [Math]::Max($fixtureLines.Count, $afterLines.Count)
    $diffCount = 0
    for ($i = 0; $i -lt $maxLen; $i++) {
        $a = if ($i -lt $fixtureLines.Count) { $fixtureLines[$i] } else { $null }
        $b = if ($i -lt $afterLines.Count) { $afterLines[$i] } else { $null }
        if ($a -cne $b) { $diffCount++ }
    }
    Report "e2e-change-diff-exactly-one-line" ($diffCount -eq 1)
}
else {
    Report "e2e-change-diff-exactly-one-line" $false "config file missing after run"
}

$fixtureSourcesBlock = Get-NervYamlSubBlock -BlockText (Get-NervYamlBlock -YamlText $fixtureLf -Key 'tasks') -Key 'sources' -Indent 2
$afterSourcesBlock = if ($configAfter) { Get-NervYamlSubBlock -BlockText (Get-NervYamlBlock -YamlText $configAfter -Key 'tasks') -Key 'sources' -Indent 2 } else { $null }
Report "e2e-sources-block-byte-identical" ($null -ne $afterSourcesBlock -and $afterSourcesBlock -ceq $fixtureSourcesBlock)

$fixtureHowtoBlock = Get-NervYamlSubBlock -BlockText (Get-NervYamlBlock -YamlText $fixtureLf -Key 'tasks') -Key 'sources_howto' -Indent 2
$afterHowtoBlock = if ($configAfter) { Get-NervYamlSubBlock -BlockText (Get-NervYamlBlock -YamlText $configAfter -Key 'tasks') -Key 'sources_howto' -Indent 2 } else { $null }
Report "e2e-sources-howto-byte-identical" ($null -ne $afterHowtoBlock -and $afterHowtoBlock -ceq $fixtureHowtoBlock)

$fixtureTeamworkBlock = $null
$fixtureTasksBlk = Get-NervYamlBlock -YamlText $fixtureLf -Key 'tasks'
$fixtureProvidersSub = Get-NervYamlSubBlock -BlockText $fixtureTasksBlk -Key 'providers' -Indent 2
if ($fixtureProvidersSub) { $fixtureTeamworkBlock = Get-NervYamlSubBlock -BlockText $fixtureProvidersSub -Key 'teamwork' -Indent 4 }
$fixtureKnownProjects = if ($fixtureTeamworkBlock) { Get-NervYamlSubBlock -BlockText $fixtureTeamworkBlock -Key 'known_projects' -Indent 6 } else { $null }

$afterTeamworkBlock = $null
if ($configAfter) {
    $afterTasksBlk = Get-NervYamlBlock -YamlText $configAfter -Key 'tasks'
    $afterProvidersSub = Get-NervYamlSubBlock -BlockText $afterTasksBlk -Key 'providers' -Indent 2
    if ($afterProvidersSub) { $afterTeamworkBlock = Get-NervYamlSubBlock -BlockText $afterProvidersSub -Key 'teamwork' -Indent 4 }
}
$afterKnownProjects = if ($afterTeamworkBlock) { Get-NervYamlSubBlock -BlockText $afterTeamworkBlock -Key 'known_projects' -Indent 6 } else { $null }
Report "e2e-known-projects-byte-identical" ($null -ne $afterKnownProjects -and $afterKnownProjects -ceq $fixtureKnownProjects)

$backupFiles = @(Get-ChildItem -LiteralPath $tempRoot -Filter "nerv.yaml.bak-configure-*" -File -ErrorAction SilentlyContinue)
Report "e2e-backup-created" ($backupFiles.Count -ge 1)

$repoConfigPath = Join-Path $tempGitRepo ".nerv/nerv.yaml"
$repoConfigContent = if (Test-Path -LiteralPath $repoConfigPath) { Get-Content -LiteralPath $repoConfigPath -Raw -Encoding UTF8 } else { $null }
Report "e2e-repo-config-enabled" ($null -ne $repoConfigContent -and $repoConfigContent -match '(?m)^enabled: true')
Report "e2e-repo-config-project-id" ($null -ne $repoConfigContent -and $repoConfigContent -match 'project_id: 111')
Report "e2e-repo-config-tasklist-id" ($null -ne $repoConfigContent -and $repoConfigContent -match 'tasklist_id: 222')

$proceduresDir = Join-Path $repoRoot "plugin/skills/nerv-tasks/providers/teamwork/procedures"
if (Test-Path -LiteralPath $proceduresDir) {
    $destCommandsDir = Join-Path $tempHome ".claude/commands/task"
    $copiedFiles = @(Get-ChildItem -LiteralPath $destCommandsDir -Filter '*.md' -File -ErrorAction SilentlyContinue)
    Report "e2e-slash-commands-copied" ($copiedFiles.Count -ge 1)
}
else {
    ReportSkip "e2e-slash-commands-copied" "plugin/skills/nerv-tasks/providers/teamwork/procedures not present yet (other writer in progress)"
}

# ---------------------------------------------------------------------------
# Case group H: inline top-level values (skills: as one-line inline map, no
# artifacts: block, critical_paths: already inline) — regression coverage
# for the real-world bug: an inline-form key was treated as "block missing"
# and a duplicate block got appended next to it, and a missing block/key
# used to be created with built-in defaults even when the user typed
# nothing.
# ---------------------------------------------------------------------------
$fixtureInlineLf = (
    "enabled: true`n" +
    "skills: { testing: [tdd, playwright-best-practices], code: [dotnet-best-practices, typescript-best-practices], best-practices: [best-practices, solid-principles, clean-code-guard], architecture: [hexagonal-architecture, c4-architecture], audit: [security-review, clean-code-guard] }   # stacks per consuming role`n" +
    "models:                             # per-role model and effort; project overrides user, key by key`n" +
    "  misato: { model: fable, effort: high }`n" +
    "critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical`n" +
    "git:`n" +
    "  base_branch: develop              # default base for the worktree offer`n" +
    "  worktree: ask                     # ask | always | never`n" +
    "  branch_pattern: `"feature/{prefix}-{id}-{slug}`"   # prefix comes from the provider (tw, gh, jira)`n" +
    "  commit_ref_pattern: `"({PREFIX}-{id})`"`n" +
    "tasks:`n" +
    "  provider: teamwork                # teamwork | github-projects | jira | none ; `"ask`" when absent`n" +
    "  ask_when_missing: true            # preflight asks task + worktree + branch if no active task`n" +
    "  subtasks_per_wave: false`n" +
    "  timer_store: ~/.claude/work/timers.json`n" +
    "  rounding_minutes: 15`n" +
    "  providers:                        # one block per provider, only the enabled one is required`n" +
    "    teamwork:`n" +
    "      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern`n" +
    "      assignee_id: 686035           # user scope`n" +
    "      default_project_id: 1271726`n" +
    "      default_tasklist_id: 3951970`n" +
    "      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }`n" +
    "    github-projects: { task_ref_prefix: gh, owner: `"`", project_number: 0 }    # later`n" +
    "    jira: { task_ref_prefix: jira, site: `"`", project_key: `"`" }               # later`n"
)

# (h1) unit: existence/inline detection for inline-form top-level keys.
Report "key-exists-inline-skills" (Test-NervYamlKeyExists -YamlText $fixtureInlineLf -Key 'skills')
Report "key-is-inline-skills" (Test-NervYamlKeyIsInline -YamlText $fixtureInlineLf -Key 'skills')
Report "key-exists-inline-critical-paths" (Test-NervYamlKeyExists -YamlText $fixtureInlineLf -Key 'critical_paths')
Report "key-is-inline-critical-paths" (Test-NervYamlKeyIsInline -YamlText $fixtureInlineLf -Key 'critical_paths')
Report "key-not-exists-artifacts-in-inline-fixture" (-not (Test-NervYamlKeyExists -YamlText $fixtureInlineLf -Key 'artifacts'))
Report "key-exists-but-not-inline-git-block-form" (
    (Test-NervYamlKeyExists -YamlText $fixtureInlineLf -Key 'git') -and
    -not (Test-NervYamlKeyIsInline -YamlText $fixtureInlineLf -Key 'git')
)
Report "get-block-returns-null-for-inline-skills" ($null -eq (Get-NervYamlBlock -YamlText $fixtureInlineLf -Key 'skills'))

# (h2) E2E: empty answers on the inline fixture -> byte-identical, no backup,
#      exactly one "skills:" key (no duplicate block appended next to it).
$tempRootInlineEmpty = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-configure-test-inline-empty-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRootInlineEmpty -Force | Out-Null
$tempHomeInlineEmpty = Join-Path $tempRootInlineEmpty "home"
New-Item -ItemType Directory -Path $tempHomeInlineEmpty -Force | Out-Null

$e2eInlineEmptyConfigPath = Join-Path $tempRootInlineEmpty "nerv.yaml"
[System.IO.File]::WriteAllText($e2eInlineEmptyConfigPath, $fixtureInlineLf, (New-Object System.Text.UTF8Encoding($false)))
$inlineEmptyAnswersPath = Join-Path $tempRootInlineEmpty "answers.txt"
Set-Content -LiteralPath $inlineEmptyAnswersPath -Value @() -Encoding UTF8

& $pwshExe -NoProfile -File $wizardPath -HomeDir $tempHomeInlineEmpty -ConfigPath $e2eInlineEmptyConfigPath -SkipSkills -SkipModels -SkipRepos -SkipCommands -NoRefresh -AnswersFile $inlineEmptyAnswersPath | Out-Null
$e2eInlineEmptyExit = $LASTEXITCODE
Report "e2e-inline-empty-exit-zero" ($e2eInlineEmptyExit -eq 0)

$configAfterInlineEmpty = if (Test-Path -LiteralPath $e2eInlineEmptyConfigPath) { Get-Content -LiteralPath $e2eInlineEmptyConfigPath -Raw -Encoding UTF8 } else { $null }
Report "e2e-inline-empty-byte-identical" ($null -ne $configAfterInlineEmpty -and $configAfterInlineEmpty -ceq $fixtureInlineLf)

$inlineEmptyBackups = @(Get-ChildItem -LiteralPath $tempRootInlineEmpty -Filter "nerv.yaml.bak-configure-*" -File -ErrorAction SilentlyContinue)
Report "e2e-inline-empty-no-backup-created" ($inlineEmptyBackups.Count -eq 0)

$skillsLineCount = if ($configAfterInlineEmpty) { ([regex]::Matches($configAfterInlineEmpty, '(?m)^skills:')).Count } else { -1 }
Report "e2e-inline-empty-single-skills-key" ($skillsLineCount -eq 1)

Remove-Item -LiteralPath $tempRootInlineEmpty -Recurse -Force -ErrorAction SilentlyContinue

# (h3) E2E: change only the `code` skills category on the inline fixture ->
#      exactly one differing line, still a single "skills:" key.
$tempRootInlineCode = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-configure-test-inline-code-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRootInlineCode -Force | Out-Null
$tempHomeInlineCode = Join-Path $tempRootInlineCode "home"
New-Item -ItemType Directory -Path $tempHomeInlineCode -Force | Out-Null

$e2eInlineCodeConfigPath = Join-Path $tempRootInlineCode "nerv.yaml"
[System.IO.File]::WriteAllText($e2eInlineCodeConfigPath, $fixtureInlineLf, (New-Object System.Text.UTF8Encoding($false)))

$inlineCodeAnswers = @()
$inlineCodeAnswers += ''   # git.base_branch
$inlineCodeAnswers += ''   # git.worktree
$inlineCodeAnswers += ''   # git.branch_pattern
$inlineCodeAnswers += ''   # git.commit_ref_pattern
$inlineCodeAnswers += ''   # tasks.provider
$inlineCodeAnswers += ''   # tasks.ask_when_missing
$inlineCodeAnswers += ''   # tasks.subtasks_per_wave
$inlineCodeAnswers += ''   # tasks.timer_store
$inlineCodeAnswers += ''   # tasks.rounding_minutes
$inlineCodeAnswers += ''   # teamwork.task_ref_prefix
$inlineCodeAnswers += ''   # teamwork.assignee_id
$inlineCodeAnswers += ''   # teamwork.default_project_id
$inlineCodeAnswers += ''   # teamwork.default_tasklist_id
$inlineCodeAnswers += ''   # stage inDev
$inlineCodeAnswers += ''   # stage testing
$inlineCodeAnswers += ''   # stage implemented
$inlineCodeAnswers += ''   # stage blocked
$inlineCodeAnswers += ''   # stage canceled
$inlineCodeAnswers += ''   # stage pending
$inlineCodeAnswers += ''   # stage analysis
$inlineCodeAnswers += ''   # skills.testing
$inlineCodeAnswers += 'dotnet-best-practices, xunit-testing'   # skills.code (CHANGED)
$inlineCodeAnswers += ''   # skills.best-practices
$inlineCodeAnswers += ''   # skills.architecture
$inlineCodeAnswers += ''   # skills.audit
$inlineCodeAnswers += ''   # critical_paths
$inlineCodeAnswers += ''   # artifacts.commit
$inlineCodeAnswers += ''   # write confirm

$inlineCodeAnswersPath = Join-Path $tempRootInlineCode "answers.txt"
Set-Content -LiteralPath $inlineCodeAnswersPath -Value $inlineCodeAnswers -Encoding UTF8

& $pwshExe -NoProfile -File $wizardPath -HomeDir $tempHomeInlineCode -ConfigPath $e2eInlineCodeConfigPath -SkipSkills -SkipModels -SkipRepos -SkipCommands -NoRefresh -AnswersFile $inlineCodeAnswersPath | Out-Null
$e2eInlineCodeExit = $LASTEXITCODE
Report "e2e-inline-code-exit-zero" ($e2eInlineCodeExit -eq 0)

$configAfterInlineCode = if (Test-Path -LiteralPath $e2eInlineCodeConfigPath) { Get-Content -LiteralPath $e2eInlineCodeConfigPath -Raw -Encoding UTF8 } else { $null }
if ($null -ne $configAfterInlineCode) {
    $fixtureInlineLines = @($fixtureInlineLf -split "`n")
    $afterInlineCodeLines = @($configAfterInlineCode -split "`n")
    $maxLenInlineCode = [Math]::Max($fixtureInlineLines.Count, $afterInlineCodeLines.Count)
    $diffCountInlineCode = 0
    for ($i = 0; $i -lt $maxLenInlineCode; $i++) {
        $a = if ($i -lt $fixtureInlineLines.Count) { $fixtureInlineLines[$i] } else { $null }
        $b = if ($i -lt $afterInlineCodeLines.Count) { $afterInlineCodeLines[$i] } else { $null }
        if ($a -cne $b) { $diffCountInlineCode++ }
    }
    Report "e2e-inline-code-diff-exactly-one-line" ($diffCountInlineCode -eq 1)
}
else {
    Report "e2e-inline-code-diff-exactly-one-line" $false "config file missing after run"
}

$skillsLineCountAfterCode = if ($configAfterInlineCode) { ([regex]::Matches($configAfterInlineCode, '(?m)^skills:')).Count } else { -1 }
Report "e2e-inline-code-single-skills-key" ($skillsLineCountAfterCode -eq 1)
Report "e2e-inline-code-new-value-present" ($null -ne $configAfterInlineCode -and $configAfterInlineCode -match 'xunit-testing')
Report "e2e-inline-code-other-categories-preserved" (
    $null -ne $configAfterInlineCode -and
    $configAfterInlineCode -match 'testing: \[tdd, playwright-best-practices\]' -and
    $configAfterInlineCode -match 'audit: \[security-review, clean-code-guard\]'
)

Remove-Item -LiteralPath $tempRootInlineCode -Recurse -Force -ErrorAction SilentlyContinue

# (h4) E2E: answer "never" for artifacts.commit on the inline fixture (which
#      has no artifacts: block at all) -> the block is appended exactly
#      once with commit: never, and every original line is preserved as is.
$tempRootInlineArtifacts = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-configure-test-inline-artifacts-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRootInlineArtifacts -Force | Out-Null
$tempHomeInlineArtifacts = Join-Path $tempRootInlineArtifacts "home"
New-Item -ItemType Directory -Path $tempHomeInlineArtifacts -Force | Out-Null

$e2eInlineArtifactsConfigPath = Join-Path $tempRootInlineArtifacts "nerv.yaml"
[System.IO.File]::WriteAllText($e2eInlineArtifactsConfigPath, $fixtureInlineLf, (New-Object System.Text.UTF8Encoding($false)))

$inlineArtifactsAnswers = @($inlineCodeAnswers)
$inlineArtifactsAnswers[21] = ''            # skills.code kept this time
$inlineArtifactsAnswers[26] = 'never'       # artifacts.commit (CHANGED) - index 26 = 27th line

$inlineArtifactsAnswersPath = Join-Path $tempRootInlineArtifacts "answers.txt"
Set-Content -LiteralPath $inlineArtifactsAnswersPath -Value $inlineArtifactsAnswers -Encoding UTF8

& $pwshExe -NoProfile -File $wizardPath -HomeDir $tempHomeInlineArtifacts -ConfigPath $e2eInlineArtifactsConfigPath -SkipSkills -SkipModels -SkipRepos -SkipCommands -NoRefresh -AnswersFile $inlineArtifactsAnswersPath | Out-Null
$e2eInlineArtifactsExit = $LASTEXITCODE
Report "e2e-inline-artifacts-exit-zero" ($e2eInlineArtifactsExit -eq 0)

$configAfterInlineArtifacts = if (Test-Path -LiteralPath $e2eInlineArtifactsConfigPath) { Get-Content -LiteralPath $e2eInlineArtifactsConfigPath -Raw -Encoding UTF8 } else { $null }
Report "e2e-inline-artifacts-appended-once" (
    $null -ne $configAfterInlineArtifacts -and
    (([regex]::Matches($configAfterInlineArtifacts, '(?m)^artifacts:')).Count -eq 1)
)
Report "e2e-inline-artifacts-commit-never" ($null -ne $configAfterInlineArtifacts -and $configAfterInlineArtifacts -match 'commit: never')

if ($null -ne $configAfterInlineArtifacts) {
    $fixtureInlineLinesForArtifacts = @($fixtureInlineLf.TrimEnd("`n") -split "`n")
    $afterInlineArtifactsLines = @($configAfterInlineArtifacts -split "`n")
    $prefixPreserved = $true
    for ($i = 0; $i -lt $fixtureInlineLinesForArtifacts.Count; $i++) {
        if ($afterInlineArtifactsLines[$i] -cne $fixtureInlineLinesForArtifacts[$i]) { $prefixPreserved = $false; break }
    }
    Report "e2e-inline-artifacts-everything-else-identical" $prefixPreserved
}
else {
    Report "e2e-inline-artifacts-everything-else-identical" $false "config file missing after run"
}

Remove-Item -LiteralPath $tempRootInlineArtifacts -Recurse -Force -ErrorAction SilentlyContinue

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "Results: $script:passCount passed, $script:failCount failed"
if ($script:failCount -ne 0) { exit 1 }
exit 0
