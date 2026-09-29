#requires -Version 7
<#
.SYNOPSIS
    Assertions for get-nerv.ps1 (the `irm | iex` / `pwsh -File` bootstrap
    installer for Windows and any host with PowerShell 7). Mirrors
    tests/get-nerv.test.sh's fixture design and scenario coverage, and
    tests/release.test.ps1's conventions (dot-source guard check, Report,
    child-process end-to-end runs). No Pester -- prints PASS/FAIL lines and
    exits 1 on any failure.

    Two testing styles are used, matching the seams get-nerv.ps1 exposes:

    - Unit cases: dot-source get-nerv.ps1 (the `$MyInvocation.InvocationName
      -ne '.'` guard skips the CLI body, same pattern as tools/release.ps1),
      then call a function directly. `Invoke-NervApiRequest` (the only
      wrapper around `Invoke-RestMethod`) is redefined afterwards so
      `Resolve-NervRef` can be exercised with canned data and no network
      access -- `Invoke-RestMethod` itself has no file:// support, so this
      in-process override is the seam for unit-level ref-resolution cases.
    - End-to-end cases: run get-nerv.ps1 in a child pwsh process (as a real
      user would, including via `Get-Content -Raw | Invoke-Expression`),
      with PATH pointing at stub `pwsh`/`claude` executables plus real git,
      and `NERV_API_FIXTURE_DIR` pointing at canned latest.json/releases.json
      fixture files -- the end-to-end seam for the API call, since
      Invoke-RestMethod cannot be overridden from outside the process.

    Fixture repo: a local, non-bare git repository with tags v0.1.0,
    v0.2.0-alpha.1, v0.2.0-rc.1, and branches main/develop/release/0.2.0,
    each carrying a dummy plugin/tools/install.ps1 (so the delegation path
    resolves), cloned into a bare repo used as NERV_REPO_URL. Built by this
    file's own helpers; tests/get-nerv.test.sh is not imported or reused.

.EXAMPLE
    pwsh -NoProfile -File tests/get-nerv.test.ps1
#>

$ErrorActionPreference = "Stop"

$selfDir = $PSScriptRoot
$repoRoot = Split-Path -Parent $selfDir
$scriptPath = Join-Path $repoRoot "get-nerv.ps1"

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

function Finish {
    Write-Host ""
    Write-Host "Results: $script:passCount passed, $script:failCount failed"
    if ($script:failCount -ne 0) { exit 1 }
    exit 0
}

# -----------------------------------------------------------------------
# Existence + dot-source guard check (same shape as release.test.ps1).
# -----------------------------------------------------------------------
if (-not (Test-Path -LiteralPath $scriptPath)) {
    Report "get-nerv-script-exists" $false "get-nerv.ps1 not found (not implemented yet)"
    Finish
}
Report "get-nerv-script-exists" $true

try {
    . $scriptPath 2>$null
}
catch {
    # Expected during RED (before the CLI body is guarded); the real signal
    # is whether the functions got defined below.
}

$requiredFunctions = @(
    'Show-NervUsage',
    'Write-NervError',
    'Get-NervHomeDir',
    'Get-NervDefaultDir',
    'Test-NervInputRedirected',
    'Assert-NervPrereqs',
    'Invoke-NervApiRequest',
    'Resolve-NervRef',
    'Invoke-NervCheckout'
)
$allDefined = $true
foreach ($fn in $requiredFunctions) {
    if (-not (Get-Command $fn -ErrorAction SilentlyContinue)) { $allDefined = $false }
}

if (-not $allDefined) {
    Report "functions-defined-after-dot-source" $false "one or more functions not found (get-nerv.ps1 not implemented yet)"
    Finish
}
Report "functions-defined-after-dot-source" $true

$script:pwshExe = (Get-Process -Id $PID).Path
if (-not $script:pwshExe -or -not (Test-Path -LiteralPath $script:pwshExe)) { $script:pwshExe = 'pwsh' }

# =========================================================================
# Fixture helpers -- real temporary git repositories, never mocked.
# =========================================================================
function New-NervTestRepo {
    param([Parameter(Mandatory)][string]$Path)
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
    & git -C $Path init -q 2>$null | Out-Null
    & git -C $Path symbolic-ref HEAD refs/heads/main 2>$null | Out-Null
    & git -C $Path config user.email "nerv-test@example.com" | Out-Null
    & git -C $Path config user.name "NERV Test" | Out-Null
}

function Add-NervTestCommit {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$MarkerText,
        [Parameter(Mandatory)][string]$Message
    )
    $toolsDir = Join-Path $Path "plugin/tools"
    New-Item -ItemType Directory -Path $toolsDir -Force | Out-Null
    Add-Content -LiteralPath (Join-Path $toolsDir "install.ps1") -Value $MarkerText -Encoding UTF8
    & git -C $Path add -A | Out-Null
    & git -C $Path commit -q -m $Message | Out-Null
}

# New-NervSourceRepo DIR -- a non-bare working repo with main/develop/
# release-0.2.0 branches and tags v0.1.0, v0.2.0-alpha.1, v0.2.0-rc.1, each
# containing plugin/tools/install.ps1 (so the delegation path exists).
function New-NervSourceRepo {
    param([Parameter(Mandatory)][string]$Path)
    New-NervTestRepo -Path $Path
    Add-NervTestCommit -Path $Path -MarkerText "dummy install.ps1 fixture" -Message "chore: v0.1.0 fixture"
    & git -C $Path tag v0.1.0 | Out-Null

    & git -C $Path checkout -q -b develop | Out-Null
    Add-NervTestCommit -Path $Path -MarkerText "develop marker" -Message "chore: develop commit"
    & git -C $Path tag v0.2.0-alpha.1 | Out-Null

    & git -C $Path checkout -q -b release/0.2.0 | Out-Null
    Add-NervTestCommit -Path $Path -MarkerText "rc marker" -Message "chore: rc commit"
    & git -C $Path tag v0.2.0-rc.1 | Out-Null

    & git -C $Path checkout -q main | Out-Null
}

function New-NervBareRepo {
    param([Parameter(Mandatory)][string]$SourcePath, [Parameter(Mandatory)][string]$BarePath)
    & git clone --bare -q $SourcePath $BarePath | Out-Null
}

function Write-NervJsonFixture {
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)][string]$Content)
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Content, $utf8NoBom)
}

# =========================================================================
# PATH helpers -- resolve/strip a named executable from a PATH value,
# considering PATHEXT, so a stub can be forced or a real tool can be hidden.
# =========================================================================
function Get-NervCommandDirs {
    param([Parameter(Mandatory)][string]$PathValue, [Parameter(Mandatory)][string]$Name)
    $dirs = [System.Collections.Generic.List[string]]::new()
    $exts = if ($env:PATHEXT) { @('') + ($env:PATHEXT -split ';') } else { @('', '.COM', '.EXE', '.BAT', '.CMD') }
    foreach ($dir in ($PathValue -split [System.IO.Path]::PathSeparator)) {
        if (-not $dir) { continue }
        foreach ($ext in $exts) {
            $candidate = Join-Path $dir ($Name + $ext)
            if (Test-Path -LiteralPath $candidate -PathType Leaf) {
                $dirs.Add($dir)
                break
            }
        }
    }
    return $dirs
}

function Remove-NervCommandFromPath {
    param([Parameter(Mandatory)][string]$PathValue, [Parameter(Mandatory)][string]$Name)
    $dirsToRemove = Get-NervCommandDirs -PathValue $PathValue -Name $Name
    $sep = [System.IO.Path]::PathSeparator
    $kept = ($PathValue -split [regex]::Escape($sep)) | Where-Object { $dirsToRemove -notcontains $_ }
    return ($kept -join $sep)
}

# =========================================================================
# Stub executables -- .cmd files (resolved ahead of any real pwsh/claude
# when their directory is prepended to PATH) that log their arguments and
# exit with a configurable code, read from env vars at run time.
# =========================================================================
function New-NervPwshStub {
    param([Parameter(Mandatory)][string]$Path)
    $content = @'
@echo off
if not defined NERV_STUB_PWSH_EXIT set NERV_STUB_PWSH_EXIT=0
if defined NERV_STUB_PWSH_LOG (>>"%NERV_STUB_PWSH_LOG%" echo %*)
exit /b %NERV_STUB_PWSH_EXIT%
'@
    Write-NervJsonFixture -Path $Path -Content $content
}

function New-NervClaudeStub {
    param([Parameter(Mandatory)][string]$Path)
    $content = @'
@echo off
if not defined NERV_STUB_CLAUDE_EXIT set NERV_STUB_CLAUDE_EXIT=0
exit /b %NERV_STUB_CLAUDE_EXIT%
'@
    Write-NervJsonFixture -Path $Path -Content $content
}

# =========================================================================
# Child-process runner -- manages the get-nerv.ps1 test seam env vars and
# PATH for exactly one invocation, restoring both afterwards.
# =========================================================================
$script:managedEnvNames = @(
    'NERV_CHANNEL', 'NERV_HOME', 'NERV_NO_CONFIGURE', 'NERV_HELP',
    'NERV_REPO_URL', 'NERV_API_BASE', 'NERV_API_FIXTURE_DIR', 'NERV_INPUT_REDIRECTED',
    'NERV_STUB_PWSH_LOG', 'NERV_STUB_PWSH_EXIT', 'NERV_STUB_CLAUDE_EXIT', 'NERV_PWSH_EXE'
)

function Invoke-NervChildProcess {
    param(
        [Parameter(Mandatory)][string[]]$ArgumentList,
        [hashtable]$EnvVars = @{},
        [string]$PathValue
    )

    $savedValues = @{}
    foreach ($name in $script:managedEnvNames) {
        $savedValues[$name] = [System.Environment]::GetEnvironmentVariable($name)
        [System.Environment]::SetEnvironmentVariable($name, $null)
    }
    $savedPath = $env:PATH

    $outFile = [System.IO.Path]::GetTempFileName()
    $errFile = [System.IO.Path]::GetTempFileName()

    try {
        foreach ($key in $EnvVars.Keys) {
            [System.Environment]::SetEnvironmentVariable($key, [string]$EnvVars[$key])
        }
        # pwsh prepends its own installation directory to the child process's
        # PATH on every launch (verified interactively: it wins over anything
        # prepended by the caller), so a bare "pwsh" lookup inside get-nerv.ps1
        # can never resolve a PATH-stubbed pwsh once the outer test runner and
        # get-nerv.ps1 are themselves both pwsh processes. NERV_PWSH_EXE (an
        # absolute path, bypassing PATH lookup entirely) is get-nerv.ps1's
        # seam for this; default every child invocation to the stub unless a
        # case explicitly overrides it.
        if (-not $EnvVars.ContainsKey('NERV_PWSH_EXE') -and $script:stubPwshPath) {
            [System.Environment]::SetEnvironmentVariable('NERV_PWSH_EXE', $script:stubPwshPath)
        }
        if ($PathValue) { $env:PATH = $PathValue }

        & $script:pwshExe -NoProfile @ArgumentList 1>$outFile 2>$errFile
        $exitCode = $LASTEXITCODE

        [pscustomobject]@{
            ExitCode = $exitCode
            StdOut   = [string](Get-Content -LiteralPath $outFile -Raw -ErrorAction SilentlyContinue)
            StdErr   = [string](Get-Content -LiteralPath $errFile -Raw -ErrorAction SilentlyContinue)
        }
    }
    finally {
        Remove-Item -LiteralPath $outFile, $errFile -ErrorAction SilentlyContinue
        foreach ($name in $script:managedEnvNames) {
            [System.Environment]::SetEnvironmentVariable($name, $savedValues[$name])
        }
        $env:PATH = $savedPath
    }
}

function Invoke-NervScriptFile {
    param([string[]]$ScriptArgs = @(), [hashtable]$EnvVars = @{}, [string]$PathValue)
    Invoke-NervChildProcess -ArgumentList (@('-File', $scriptPath) + $ScriptArgs) -EnvVars $EnvVars -PathValue $PathValue
}

function Invoke-NervScriptViaIex {
    param([hashtable]$EnvVars = @{}, [string]$PathValue)
    $iexCommand = "Get-Content -LiteralPath '$scriptPath' -Raw | Invoke-Expression"
    Invoke-NervChildProcess -ArgumentList @('-Command', $iexCommand) -EnvVars $EnvVars -PathValue $PathValue
}

# =========================================================================
# Shared fixtures.
# =========================================================================
$workDir = Join-Path ([System.IO.Path]::GetTempPath()) ("nerv-get-nerv-test-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $workDir -Force | Out-Null

try {
    $srcRepo = Join-Path $workDir "src"
    New-NervSourceRepo -Path $srcRepo
    $bareRepo = Join-Path $workDir "bare.git"
    New-NervBareRepo -SourcePath $srcRepo -BarePath $bareRepo

    $fixturesCommon = Join-Path $workDir "fixtures-common"
    New-Item -ItemType Directory -Path $fixturesCommon -Force | Out-Null
    Write-NervJsonFixture -Path (Join-Path $fixturesCommon "latest.json") -Content @'
{
  "tag_name": "v0.1.0",
  "prerelease": false,
  "name": "v0.1.0"
}
'@
    Write-NervJsonFixture -Path (Join-Path $fixturesCommon "releases.json") -Content @'
[
  {
    "tag_name": "v0.2.0-rc.1",
    "prerelease": true,
    "name": "v0.2.0-rc.1"
  },
  {
    "tag_name": "v0.2.0-alpha.1",
    "prerelease": true,
    "name": "v0.2.0-alpha.1"
  },
  {
    "tag_name": "v0.1.0",
    "prerelease": false,
    "name": "v0.1.0"
  }
]
'@

    $fixturesEmpty = Join-Path $workDir "fixtures-empty"
    New-Item -ItemType Directory -Path $fixturesEmpty -Force | Out-Null
    Write-NervJsonFixture -Path (Join-Path $fixturesEmpty "releases.json") -Content "[]`n"

    $stubDirFull = Join-Path $workDir "stub-full"
    New-Item -ItemType Directory -Path $stubDirFull -Force | Out-Null
    New-NervPwshStub -Path (Join-Path $stubDirFull "pwsh.cmd")
    New-NervClaudeStub -Path (Join-Path $stubDirFull "claude.cmd")
    $script:stubPwshPath = Join-Path $stubDirFull "pwsh.cmd"

    $originalPath = $env:PATH
    $pathNoGit = Remove-NervCommandFromPath -PathValue $originalPath -Name 'git'
    $pathNoClaude = Remove-NervCommandFromPath -PathValue $originalPath -Name 'claude'
    $pathBaseNoTools = Remove-NervCommandFromPath -PathValue (Remove-NervCommandFromPath -PathValue $originalPath -Name 'pwsh') -Name 'claude'
    $pathNormal = "$stubDirFull$([System.IO.Path]::PathSeparator)$pathBaseNoTools"

    # =====================================================================
    # Case: -Help exits 0 and prints usage
    # =====================================================================
    $r = Invoke-NervScriptFile -ScriptArgs @('-Help') -PathValue $originalPath
    Report "help-exit-0" ($r.ExitCode -eq 0 -and $r.StdOut -match '(?m)^Usage: get-nerv\.ps1') "exit=$($r.ExitCode)"

    # =====================================================================
    # Case: NERV_HELP=1 exits 0 and prints usage (env fallback)
    # =====================================================================
    $r = Invoke-NervScriptFile -EnvVars @{ NERV_HELP = '1' } -PathValue $originalPath
    Report "help-env-var-exit-0" ($r.ExitCode -eq 0 -and $r.StdOut -match '(?m)^Usage: get-nerv\.ps1') "exit=$($r.ExitCode)"

    # =====================================================================
    # Case: invalid channel via -Channel -> non-zero exit with a message
    # =====================================================================
    $targetDir = Join-Path $workDir "target-bad-channel-flag"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'bogus', '-Dir', $targetDir) -PathValue $originalPath
    Report "invalid-channel-flag-nonzero" ($r.ExitCode -ne 0 -and $r.StdErr -match 'Unknown channel') "exit=$($r.ExitCode) err=[$($r.StdErr)]"

    # =====================================================================
    # Case: invalid channel via NERV_CHANNEL -> non-zero exit with a message
    # (manual check, since the env fallback bypasses any parameter
    # validation)
    # =====================================================================
    $targetDir = Join-Path $workDir "target-bad-channel-env"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Dir', $targetDir) -EnvVars @{ NERV_CHANNEL = 'bogus' } -PathValue $originalPath
    Report "invalid-channel-env-nonzero" ($r.ExitCode -ne 0 -and $r.StdErr -match 'Unknown channel') "exit=$($r.ExitCode) err=[$($r.StdErr)]"

    # =====================================================================
    # Case: unknown parameter -> PowerShell's own non-zero exit
    # =====================================================================
    $r = Invoke-NervScriptFile -ScriptArgs @('-Bogus') -PathValue $originalPath
    Report "unknown-parameter-nonzero" ($r.ExitCode -ne 0) "exit=$($r.ExitCode)"

    # =====================================================================
    # Case: missing git -> exit 1 with a hint
    # =====================================================================
    $targetDir = Join-Path $workDir "target-missing-git"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Dir', $targetDir) -PathValue $pathNoGit
    Report "missing-git-exit-1-with-hint" ($r.ExitCode -eq 1 -and $r.StdErr -match 'git-scm\.com') "exit=$($r.ExitCode) err=[$($r.StdErr)]"

    # =====================================================================
    # Case: missing claude -> exit 1 with a hint (real git present)
    # =====================================================================
    $targetDir = Join-Path $workDir "target-missing-claude"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Dir', $targetDir) -PathValue $pathNoClaude
    Report "missing-claude-exit-1-with-hint" ($r.ExitCode -eq 1 -and $r.StdErr -match 'docs\.claude\.com') "exit=$($r.ExitCode) err=[$($r.StdErr)]"

    # =====================================================================
    # Case: stable resolves v0.1.0 and clones it
    # =====================================================================
    $targetDir = Join-Path $workDir "target-stable"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{ NERV_REPO_URL = $bareRepo; NERV_API_BASE = 'https://fixture.test'; NERV_API_FIXTURE_DIR = $fixturesCommon } `
        -PathValue $pathNormal
    $resolvedTag = (& git -C $targetDir describe --tags --exact-match 2>$null)
    Report "stable-resolves-v0.1.0" ($r.ExitCode -eq 0 -and $resolvedTag -eq 'v0.1.0') "exit=$($r.ExitCode) tag=[$resolvedTag] err=[$($r.StdErr)]"

    # =====================================================================
    # Case: alpha resolves v0.2.0-alpha.1
    # =====================================================================
    $targetDir = Join-Path $workDir "target-alpha"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'alpha', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{ NERV_REPO_URL = $bareRepo; NERV_API_BASE = 'https://fixture.test'; NERV_API_FIXTURE_DIR = $fixturesCommon } `
        -PathValue $pathNormal
    $resolvedTag = (& git -C $targetDir describe --tags --exact-match 2>$null)
    Report "alpha-resolves-v0.2.0-alpha.1" ($r.ExitCode -eq 0 -and $resolvedTag -eq 'v0.2.0-alpha.1') "exit=$($r.ExitCode) tag=[$resolvedTag] err=[$($r.StdErr)]"

    # =====================================================================
    # Case: rc resolves v0.2.0-rc.1
    # =====================================================================
    $targetDir = Join-Path $workDir "target-rc"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'rc', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{ NERV_REPO_URL = $bareRepo; NERV_API_BASE = 'https://fixture.test'; NERV_API_FIXTURE_DIR = $fixturesCommon } `
        -PathValue $pathNormal
    $resolvedTag = (& git -C $targetDir describe --tags --exact-match 2>$null)
    Report "rc-resolves-v0.2.0-rc.1" ($r.ExitCode -eq 0 -and $resolvedTag -eq 'v0.2.0-rc.1') "exit=$($r.ExitCode) tag=[$resolvedTag] err=[$($r.StdErr)]"

    # =====================================================================
    # Case: alpha with an empty releases list falls back to develop
    # =====================================================================
    $targetDir = Join-Path $workDir "target-alpha-fallback"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'alpha', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{ NERV_REPO_URL = $bareRepo; NERV_API_BASE = 'https://fixture.test'; NERV_API_FIXTURE_DIR = $fixturesEmpty } `
        -PathValue $pathNormal
    $headBranch = (& git -C $targetDir rev-parse --abbrev-ref HEAD 2>$null)
    Report "alpha-empty-releases-falls-back-to-develop" ($r.ExitCode -eq 0 -and $headBranch -eq 'develop') "exit=$($r.ExitCode) head=[$headBranch] err=[$($r.StdErr)]"

    # =====================================================================
    # Case: rc with no pre-release published yet -> exit 1
    # =====================================================================
    $targetDir = Join-Path $workDir "target-rc-none"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'rc', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{ NERV_REPO_URL = $bareRepo; NERV_API_BASE = 'https://fixture.test'; NERV_API_FIXTURE_DIR = $fixturesEmpty } `
        -PathValue $pathNormal
    Report "rc-empty-releases-exit-1" ($r.ExitCode -eq 1 -and $r.StdErr -match '(?i)no rc pre-release') "exit=$($r.ExitCode) err=[$($r.StdErr)]"

    # =====================================================================
    # Case: existing non-git directory -> refuse, exit 1
    # =====================================================================
    $targetDir = Join-Path $workDir "target-nongit"
    New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $targetDir "marker.txt") -Value "not a repo" -Encoding UTF8
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{ NERV_REPO_URL = $bareRepo; NERV_API_BASE = 'https://fixture.test'; NERV_API_FIXTURE_DIR = $fixturesCommon } `
        -PathValue $pathNormal
    Report "non-git-existing-dir-exit-1" ($r.ExitCode -eq 1 -and $r.StdErr -match '(?i)not a git repository') "exit=$($r.ExitCode) err=[$($r.StdErr)]"

    # =====================================================================
    # Case: second run updates the existing checkout instead of re-cloning
    # =====================================================================
    $srcUpdate = Join-Path $workDir "src-update"
    New-NervSourceRepo -Path $srcUpdate
    $bareUpdate = Join-Path $workDir "bare-update.git"
    New-NervBareRepo -SourcePath $srcUpdate -BarePath $bareUpdate

    $fixturesUpdate = Join-Path $workDir "fixtures-update"
    New-Item -ItemType Directory -Path $fixturesUpdate -Force | Out-Null
    Write-NervJsonFixture -Path (Join-Path $fixturesUpdate "latest.json") -Content @'
{
  "tag_name": "v0.1.0",
  "prerelease": false,
  "name": "v0.1.0"
}
'@

    $targetDir = Join-Path $workDir "target-update"
    $r1 = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{ NERV_REPO_URL = $bareUpdate; NERV_API_BASE = 'https://fixture.test'; NERV_API_FIXTURE_DIR = $fixturesUpdate } `
        -PathValue $pathNormal

    $markerFile = Join-Path $targetDir ".nerv-test-marker"
    Set-Content -LiteralPath $markerFile -Value "sentinel" -Encoding UTF8

    & git -C $srcUpdate checkout -q main | Out-Null
    Add-Content -LiteralPath (Join-Path $srcUpdate "plugin/tools/install.ps1") -Value "update marker" -Encoding UTF8
    & git -C $srcUpdate commit -q -am "chore: v0.1.1 fixture" | Out-Null
    & git -C $srcUpdate tag v0.1.1 | Out-Null
    & git -C $srcUpdate push -q $bareUpdate main v0.1.1 | Out-Null

    Write-NervJsonFixture -Path (Join-Path $fixturesUpdate "latest.json") -Content @'
{
  "tag_name": "v0.1.1",
  "prerelease": false,
  "name": "v0.1.1"
}
'@

    $r2 = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{ NERV_REPO_URL = $bareUpdate; NERV_API_BASE = 'https://fixture.test'; NERV_API_FIXTURE_DIR = $fixturesUpdate } `
        -PathValue $pathNormal

    $resolvedTag2 = (& git -C $targetDir describe --tags --exact-match 2>$null)
    $markerSurvived = Test-Path -LiteralPath $markerFile
    Report "second-run-updates-not-clones" ($r1.ExitCode -eq 0 -and $r2.ExitCode -eq 0 -and $markerSurvived -and $resolvedTag2 -eq 'v0.1.1') `
        "first=$($r1.ExitCode) second=$($r2.ExitCode) markerSurvived=$markerSurvived tag=[$resolvedTag2]"

    # =====================================================================
    # Case: pwsh invocation carries -RefreshCache -Skills -Configure when
    # stdin is not redirected (NERV_INPUT_REDIRECTED=0 overrides the probe)
    # =====================================================================
    $targetDir = Join-Path $workDir "target-tty"
    $pwshLog = Join-Path $workDir "pwsh-log-tty.txt"
    New-Item -ItemType File -Path $pwshLog -Force | Out-Null
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir) `
        -EnvVars @{
            NERV_REPO_URL          = $bareRepo
            NERV_API_BASE          = 'https://fixture.test'
            NERV_API_FIXTURE_DIR   = $fixturesCommon
            NERV_INPUT_REDIRECTED  = '0'
            NERV_STUB_PWSH_LOG     = $pwshLog
        } `
        -PathValue $pathNormal
    $logLine = (Get-Content -LiteralPath $pwshLog -Raw -ErrorAction SilentlyContinue)
    $hasAll = $logLine -and $logLine.Contains('-File') -and $logLine.Contains('-RefreshCache') -and $logLine.Contains('-Skills') -and $logLine.Contains('-Configure')
    Report "pwsh-invocation-with-configure-tty" ($r.ExitCode -eq 0 -and $hasAll) "exit=$($r.ExitCode) log=[$logLine] err=[$($r.StdErr)]"

    # =====================================================================
    # Case: -NoConfigure omits -Configure regardless of tty availability
    # =====================================================================
    $targetDir = Join-Path $workDir "target-no-configure-flag"
    $pwshLog = Join-Path $workDir "pwsh-log-noconfigure.txt"
    New-Item -ItemType File -Path $pwshLog -Force | Out-Null
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{
            NERV_REPO_URL         = $bareRepo
            NERV_API_BASE         = 'https://fixture.test'
            NERV_API_FIXTURE_DIR  = $fixturesCommon
            NERV_INPUT_REDIRECTED = '0'
            NERV_STUB_PWSH_LOG    = $pwshLog
        } `
        -PathValue $pathNormal
    $logLine = (Get-Content -LiteralPath $pwshLog -Raw -ErrorAction SilentlyContinue)
    $hasBase = $logLine -and $logLine.Contains('-File') -and $logLine.Contains('-RefreshCache') -and $logLine.Contains('-Skills')
    $hasConfigure = $logLine -and $logLine.Contains('-Configure')
    Report "pwsh-invocation-without-configure-flag" ($r.ExitCode -eq 0 -and $hasBase -and -not $hasConfigure) "exit=$($r.ExitCode) log=[$logLine]"

    # =====================================================================
    # Case: NERV_NO_CONFIGURE=1 (env) omits -Configure (env fallback)
    # =====================================================================
    $targetDir = Join-Path $workDir "target-no-configure-env"
    $pwshLog = Join-Path $workDir "pwsh-log-noconfigure-env.txt"
    New-Item -ItemType File -Path $pwshLog -Force | Out-Null
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir) `
        -EnvVars @{
            NERV_REPO_URL         = $bareRepo
            NERV_API_BASE         = 'https://fixture.test'
            NERV_API_FIXTURE_DIR  = $fixturesCommon
            NERV_INPUT_REDIRECTED = '0'
            NERV_NO_CONFIGURE     = '1'
            NERV_STUB_PWSH_LOG    = $pwshLog
        } `
        -PathValue $pathNormal
    $logLine = (Get-Content -LiteralPath $pwshLog -Raw -ErrorAction SilentlyContinue)
    $hasConfigure = $logLine -and $logLine.Contains('-Configure')
    Report "pwsh-invocation-without-configure-env" ($r.ExitCode -eq 0 -and -not $hasConfigure) "exit=$($r.ExitCode) log=[$logLine]"

    # =====================================================================
    # Case: no interactive terminal (NERV_INPUT_REDIRECTED=1) -> -Configure
    # omitted, skip message printed
    # =====================================================================
    $targetDir = Join-Path $workDir "target-no-tty"
    $pwshLog = Join-Path $workDir "pwsh-log-no-tty.txt"
    New-Item -ItemType File -Path $pwshLog -Force | Out-Null
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir) `
        -EnvVars @{
            NERV_REPO_URL         = $bareRepo
            NERV_API_BASE         = 'https://fixture.test'
            NERV_API_FIXTURE_DIR  = $fixturesCommon
            NERV_INPUT_REDIRECTED = '1'
            NERV_STUB_PWSH_LOG    = $pwshLog
        } `
        -PathValue $pathNormal
    $logLine = (Get-Content -LiteralPath $pwshLog -Raw -ErrorAction SilentlyContinue)
    $hasConfigure = $logLine -and $logLine.Contains('-Configure')
    $skippedMessage = $r.StdOut -match '(?i)skipping the configuration wizard' -and $r.StdOut -match 'configure\.ps1'
    Report "pwsh-invocation-without-configure-no-tty" ($r.ExitCode -eq 0 -and -not $hasConfigure -and $skippedMessage) "exit=$($r.ExitCode) log=[$logLine] out=[$($r.StdOut)]"

    # =====================================================================
    # Case: pwsh's non-zero exit code propagates as get-nerv.ps1's exit code
    # =====================================================================
    $targetDir = Join-Path $workDir "target-pwsh-fail"
    $pwshLog = Join-Path $workDir "pwsh-log-fail.txt"
    New-Item -ItemType File -Path $pwshLog -Force | Out-Null
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{
            NERV_REPO_URL       = $bareRepo
            NERV_API_BASE       = 'https://fixture.test'
            NERV_API_FIXTURE_DIR = $fixturesCommon
            NERV_STUB_PWSH_LOG  = $pwshLog
            NERV_STUB_PWSH_EXIT = '5'
        } `
        -PathValue $pathNormal
    Report "pwsh-nonzero-exit-propagates" ($r.ExitCode -eq 5) "exit=$($r.ExitCode)"

    # =====================================================================
    # Case: parameter wins over env (NERV_CHANNEL=rc, -Channel stable)
    # =====================================================================
    $targetDir = Join-Path $workDir "target-param-over-env"
    $r = Invoke-NervScriptFile -ScriptArgs @('-Channel', 'stable', '-Dir', $targetDir, '-NoConfigure') `
        -EnvVars @{ NERV_REPO_URL = $bareRepo; NERV_API_BASE = 'https://fixture.test'; NERV_API_FIXTURE_DIR = $fixturesCommon; NERV_CHANNEL = 'rc' } `
        -PathValue $pathNormal
    $resolvedTag = (& git -C $targetDir describe --tags --exact-match 2>$null)
    Report "parameter-over-env-precedence" ($r.ExitCode -eq 0 -and $resolvedTag -eq 'v0.1.0') "exit=$($r.ExitCode) tag=[$resolvedTag]"

    # =====================================================================
    # Case: `Get-Content -Raw | Invoke-Expression` runs the installer (the
    # iex path takes every option from the environment, since no parameters
    # can be passed through the pipe)
    # =====================================================================
    $targetDir = Join-Path $workDir "target-iex"
    $pwshLog = Join-Path $workDir "pwsh-log-iex.txt"
    New-Item -ItemType File -Path $pwshLog -Force | Out-Null
    $r = Invoke-NervScriptViaIex `
        -EnvVars @{
            NERV_REPO_URL         = $bareRepo
            NERV_API_BASE         = 'https://fixture.test'
            NERV_API_FIXTURE_DIR  = $fixturesCommon
            NERV_CHANNEL          = 'stable'
            NERV_HOME             = $targetDir
            NERV_NO_CONFIGURE     = '1'
            NERV_STUB_PWSH_LOG    = $pwshLog
        } `
        -PathValue $pathNormal
    $resolvedTag = (& git -C $targetDir describe --tags --exact-match 2>$null)
    $logLine = (Get-Content -LiteralPath $pwshLog -Raw -ErrorAction SilentlyContinue)
    Report "iex-runs-installer" ($r.ExitCode -eq 0 -and $resolvedTag -eq 'v0.1.0' -and $logLine -and $logLine.Contains('-RefreshCache')) `
        "exit=$($r.ExitCode) tag=[$resolvedTag] log=[$logLine] err=[$($r.StdErr)]"

    # =====================================================================
    # Unit cases: Resolve-NervRef with Invoke-NervApiRequest overridden
    # after dot-sourcing (no network, no fixture files).
    # =====================================================================
    function global:Invoke-NervApiRequest {
        param([string]$Uri)
        if ($Uri -like '*releases/latest') {
            return [pscustomobject]@{ tag_name = 'v9.9.9'; prerelease = $false; name = 'v9.9.9' }
        }
        if ($Uri -like '*releases*') {
            return @(
                [pscustomobject]@{ tag_name = 'v9.9.9-rc.2'; prerelease = $true; name = 'v9.9.9-rc.2' },
                [pscustomobject]@{ tag_name = 'v9.9.9-alpha.3'; prerelease = $true; name = 'v9.9.9-alpha.3' },
                [pscustomobject]@{ tag_name = 'v9.9.8'; prerelease = $false; name = 'v9.9.8' }
            )
        }
        throw "unexpected URI in unit-case mock: $Uri"
    }

    $stableResult = Resolve-NervRef -Channel stable -ApiBase 'https://fixture.test'
    Report "unit-resolve-stable" ($stableResult.Kind -eq 'tag' -and $stableResult.Ref -eq 'v9.9.9') "got=[$($stableResult.Kind) $($stableResult.Ref)]"

    $alphaResult = Resolve-NervRef -Channel alpha -ApiBase 'https://fixture.test'
    Report "unit-resolve-alpha" ($alphaResult.Kind -eq 'tag' -and $alphaResult.Ref -eq 'v9.9.9-alpha.3') "got=[$($alphaResult.Kind) $($alphaResult.Ref)]"

    $rcResult = Resolve-NervRef -Channel rc -ApiBase 'https://fixture.test'
    Report "unit-resolve-rc" ($rcResult.Kind -eq 'tag' -and $rcResult.Ref -eq 'v9.9.9-rc.2') "got=[$($rcResult.Kind) $($rcResult.Ref)]"

    function global:Invoke-NervApiRequest {
        param([string]$Uri)
        if ($Uri -like '*releases/latest') { throw "should not be called for alpha/rc" }
        return @()
    }

    $alphaFallback = Resolve-NervRef -Channel alpha -ApiBase 'https://fixture.test'
    Report "unit-resolve-alpha-fallback-develop" ($alphaFallback.Kind -eq 'branch' -and $alphaFallback.Ref -eq 'develop') "got=[$($alphaFallback.Kind) $($alphaFallback.Ref)]"

    $rcThrew = $false
    try {
        Resolve-NervRef -Channel rc -ApiBase 'https://fixture.test' | Out-Null
    }
    catch {
        $rcThrew = $_.Exception.Message -match '(?i)no rc pre-release'
    }
    Report "unit-resolve-rc-none-throws" $rcThrew

    # Unit case: Resolve-NervRef reads from NERV_API_FIXTURE_DIR-style
    # fixture files directly when a FixtureDir is given (bypasses the
    # Invoke-NervApiRequest override entirely).
    function global:Invoke-NervApiRequest {
        param([string]$Uri)
        throw "should not be called when FixtureDir is set: $Uri"
    }
    $fixtureResult = Resolve-NervRef -Channel stable -ApiBase 'https://fixture.test' -FixtureDir $fixturesCommon
    Report "unit-resolve-uses-fixture-dir" ($fixtureResult.Kind -eq 'tag' -and $fixtureResult.Ref -eq 'v0.1.0') "got=[$($fixtureResult.Kind) $($fixtureResult.Ref)]"
}
finally {
    Remove-Item -LiteralPath $workDir -Recurse -Force -ErrorAction SilentlyContinue
}

Finish
