#requires -Version 7
<#
.SYNOPSIS
    Bootstrap installer for NERV Gentle-AI on Windows (and any host with
    PowerShell 7).

.DESCRIPTION
    Resolves a release channel through the GitHub releases API, checks out
    the matching ref into a local directory, and delegates to
    plugin/tools/install.ps1 (marketplace registration, plugin cache
    refresh, skills, and the configuration wizard). Mirrors get-nerv.sh's
    behaviour exactly, in PowerShell.

    Usage:
      irm https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/get-nerv.ps1 | iex
      pwsh -File get-nerv.ps1 -Channel rc

    Run `get-nerv.ps1 -Help` for the full option list, and see the
    "Install" section of README.md for end-user documentation.
    tests/get-nerv.test.ps1 covers this file's behaviour with a stubbed
    PATH and a local fixture repo.

    Dot-source vs `irm | iex` guard: the CLI body at the bottom of this
    file is wrapped in `if ($MyInvocation.InvocationName -ne '.')`, the
    same pattern tools/release.ps1 and plugin/tools/install.ps1 already
    use. Dot-sourcing sets $MyInvocation.InvocationName to '.', so tests
    can dot-source this file to get every function defined without
    running the installer for real. Running the file directly
    (`pwsh -File get-nerv.ps1`) sets InvocationName to the script's own
    path, which is also not '.', so the CLI body runs. Piping the raw text
    into `Invoke-Expression` (the `irm | iex` one-liner) sets
    InvocationName to an empty string -- also not '.' -- so the CLI body
    runs there too, with no extra env-var seam required (a
    NERV_BOOTSTRAP_NO_MAIN=1-style seam was considered and is unnecessary:
    the existing guard already tells dot-source apart from every other
    invocation shape, verified interactively against pwsh 7.6).

.PARAMETER Channel
    Release channel to install: stable, alpha, or rc. No -ValidateSet
    attribute is used here (deviation from the plan): under `irm | iex`
    no parameters are ever bound, so this parameter is always left at its
    default (empty string) in that path, and a ValidateSet with only
    non-empty values fails parameter binding on that empty default before
    the script body ever runs -- verified interactively. Channel
    validation is instead a single manual check in the CLI body below,
    covering both the parameter and the NERV_CHANNEL environment fallback
    with one code path (matching get-nerv.sh's own `case` statement).
    Defaults to $env:NERV_CHANNEL, then "stable".

.PARAMETER Dir
    Checkout directory. Defaults to $env:NERV_HOME, then
    "<home>/.nerv/src" ($env:HOME when set, else $env:USERPROFILE).

.PARAMETER NoConfigure
    Skip the configuration wizard after installing. Defaults to whether
    $env:NERV_NO_CONFIGURE is set to a non-empty, non-"0" value.

.PARAMETER Help
    Show usage and exit 0. Same effect as $env:NERV_HELP being set to a
    non-empty, non-"0" value.

.EXAMPLE
    pwsh -File get-nerv.ps1 -Channel rc -Dir C:\nerv
.EXAMPLE
    $env:NERV_CHANNEL = 'alpha'; irm https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/get-nerv.ps1 | iex
#>
[CmdletBinding()]
param(
    [string]$Channel,
    [string]$Dir,
    [switch]$NoConfigure,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Captured at script scope (not inside a function) so a nested call below
# can tell "-Channel was passed" apart from "-Channel defaulted to ''"
# regardless of invocation shape (file, dot-source, or iex).
$script:NervBoundParameters = $PSBoundParameters

$script:NervRepoUrlDefault = 'https://github.com/war-apps/nerv-gentle-ai.git'
$script:NervApiBaseDefault = 'https://api.github.com/repos/war-apps/nerv-gentle-ai'

# =============================================================================
# Usage / error output.
# =============================================================================
function Show-NervUsage {
    <#
    .SYNOPSIS
        Prints the usage text to stdout, or to stderr with -ToStdErr (used
        when an invalid option forces a non-zero exit).
    #>
    param([switch]$ToStdErr)

    $text = @'
Usage: get-nerv.ps1 [options]

Bootstrap installer for NERV Gentle-AI: checks out a release channel and
delegates to plugin/tools/install.ps1 (marketplace registration, plugin
cache refresh, skills, and the configuration wizard).

Options:
  -Channel <stable|alpha|rc>  Release channel to install (default: stable)
  -Dir <path>                 Checkout directory (default: $HOME/.nerv/src)
  -NoConfigure                Skip the configuration wizard after installing
  -Help                       Show this help and exit

Environment fallbacks (used only when the matching parameter is not given;
this is the only way to configure the installer when piped into iex, since
`irm ... | iex` never passes parameters):
  NERV_CHANNEL        same as -Channel
  NERV_HOME           same as -Dir
  NERV_NO_CONFIGURE   non-empty, non-"0" skips the wizard, same as -NoConfigure
  NERV_HELP           non-empty, non-"0" shows usage and exits 0, same as -Help

Advanced/test hooks:
  NERV_REPO_URL          git remote to clone/fetch from
                         (default: https://github.com/war-apps/nerv-gentle-ai.git)
  NERV_API_BASE          GitHub API base used to resolve releases
                         (default: https://api.github.com/repos/war-apps/nerv-gentle-ai)
  NERV_API_FIXTURE_DIR   directory containing latest.json/releases.json to
                         read instead of calling the GitHub API (tests only)
  NERV_INPUT_REDIRECTED  "1"/"0" forces the has-a-terminal check that
                         decides whether the configuration wizard can
                         attach to a real console; unset probes
                         [Console]::IsInputRedirected for real
  NERV_PWSH_EXE          executable used for the install.ps1 delegation
                         instead of resolving "pwsh" on PATH (default:
                         "pwsh"); pwsh prepends its own install directory
                         to a child process's PATH, so PATH-stubbing a
                         nested "pwsh" from a pwsh test runner does not
                         work without this seam
'@

    if ($ToStdErr) {
        [Console]::Error.WriteLine($text)
    }
    else {
        Write-Host $text
    }
}

function Write-NervError {
    <#
    .SYNOPSIS
        Writes a message to the real stderr stream (not the PowerShell
        error stream), matching get-nerv.sh's log_err().
    #>
    param([string]$Message)
    [Console]::Error.WriteLine($Message)
}

# =============================================================================
# Home directory / default checkout dir.
# =============================================================================
function Get-NervHomeDir {
    <#
    .SYNOPSIS
        Resolves the user's home directory: $env:HOME first, then
        $env:USERPROFILE.
    #>
    if ($env:HOME) { return $env:HOME }
    return $env:USERPROFILE
}

function Get-NervDefaultDir {
    <#
    .SYNOPSIS
        Default checkout directory: "<home>/.nerv/src".
    #>
    Join-Path (Get-NervHomeDir) '.nerv/src'
}

# =============================================================================
# Terminal detection for the configuration wizard.
# =============================================================================
function Test-NervInputRedirected {
    <#
    .SYNOPSIS
        True when stdin is redirected (no interactive terminal available
        to run the configuration wizard). NERV_INPUT_REDIRECTED overrides
        the real probe for tests ("1" forces redirected/no-tty, "0" forces
        not-redirected/tty-available).
    #>
    if ($env:NERV_INPUT_REDIRECTED -eq '1') { return $true }
    if ($env:NERV_INPUT_REDIRECTED -eq '0') { return $false }
    return [Console]::IsInputRedirected
}

# =============================================================================
# Prerequisites.
# =============================================================================
function Assert-NervPrereqs {
    <#
    .SYNOPSIS
        Throws with a hint when git or claude are missing on PATH.
        PowerShell itself is implicitly >= 7 via #requires. On Windows,
        Get-Command resolves claude.cmd/claude.exe the same way it
        resolves any other PATH executable.
    #>
    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        throw "git is required but was not found on PATH.`nInstall it: https://git-scm.com/downloads"
    }

    if (-not (Get-Command claude -ErrorAction SilentlyContinue)) {
        throw "claude (Claude Code CLI) is required but was not found on PATH.`nInstall it: https://docs.claude.com/en/docs/claude-code/setup"
    }
}

# =============================================================================
# GitHub API access.
# =============================================================================
function Invoke-NervApiRequest {
    <#
    .SYNOPSIS
        Thin wrapper around Invoke-RestMethod (GitHub requires a
        User-Agent header). The only seam Resolve-NervRef needs for
        network access -- unit tests redefine this function after
        dot-sourcing this script, since Invoke-RestMethod itself has no
        file:// support to point at canned fixtures.
    #>
    param([Parameter(Mandatory)][string]$Uri)
    Invoke-RestMethod -Uri $Uri -Headers @{ 'User-Agent' = 'nerv-gentle-ai-get-nerv' }
}

function Get-NervJsonProperty {
    <#
    .SYNOPSIS
        Safely reads a property off a possibly-$null / possibly-partial
        JSON object without tripping Set-StrictMode -Version Latest's
        "property not found" error.
    #>
    param($InputObject, [Parameter(Mandatory)][string]$Name)
    if ($null -eq $InputObject) { return $null }
    $prop = $InputObject.PSObject.Properties[$Name]
    if ($null -eq $prop) { return $null }
    return $prop.Value
}

function Resolve-NervRef {
    <#
    .SYNOPSIS
        Resolves a channel to a "KIND" ("tag" or "branch") and a ref name,
        via the GitHub releases API (or NERV_API_FIXTURE_DIR's
        latest.json/releases.json, for end-to-end tests that cannot
        override Invoke-NervApiRequest from outside the process). Throws
        with a clear message when no ref can be resolved.

    .PARAMETER FixtureDir
        When set, reads latest.json (stable) or releases.json
        (alpha/rc) from this directory instead of calling the API.
    #>
    param(
        [Parameter(Mandatory)]
        [ValidateSet('stable', 'alpha', 'rc')]
        [string]$Channel,

        [Parameter(Mandatory)]
        [string]$ApiBase,

        [string]$FixtureDir
    )

    function Get-NervLatestRelease {
        if ($FixtureDir) {
            $path = Join-Path $FixtureDir 'latest.json'
            return (Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json)
        }
        return (Invoke-NervApiRequest -Uri "$ApiBase/releases/latest")
    }

    function Get-NervReleaseList {
        if ($FixtureDir) {
            $path = Join-Path $FixtureDir 'releases.json'
            return (Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json)
        }
        return (Invoke-NervApiRequest -Uri "$ApiBase/releases?per_page=50")
    }

    if ($Channel -eq 'stable') {
        $release = Get-NervLatestRelease
        $tag = Get-NervJsonProperty -InputObject $release -Name 'tag_name'
        if (-not $tag) {
            throw "Could not resolve the latest stable release from $ApiBase/releases/latest."
        }
        return [pscustomobject]@{ Kind = 'tag'; Ref = $tag }
    }

    # alpha / rc: first release (in array/newest-first order, as GitHub
    # returns them) that is a pre-release and whose tag contains
    # "-<channel>.".
    $marker = "-$Channel."
    $releases = @(Get-NervReleaseList)
    $found = $null
    foreach ($item in $releases) {
        $isPrerelease = Get-NervJsonProperty -InputObject $item -Name 'prerelease'
        $tagName = Get-NervJsonProperty -InputObject $item -Name 'tag_name'
        if ($isPrerelease -eq $true -and $tagName -and $tagName.Contains($marker)) {
            $found = $tagName
            break
        }
    }

    if ($found) {
        return [pscustomobject]@{ Kind = 'tag'; Ref = $found }
    }

    if ($Channel -eq 'alpha') {
        return [pscustomobject]@{ Kind = 'branch'; Ref = 'develop' }
    }

    throw "No rc pre-release published yet."
}

# =============================================================================
# Checkout / update.
# =============================================================================
function Invoke-NervCheckout {
    <#
    .SYNOPSIS
        Clones Dir fresh when it has no .git; otherwise fetches Ref and
        checks it out. Mirrors get-nerv.sh's checkout_ref() exactly: a tag
        is fetched into an explicit local refs/tags/<ref> (force-updated
        with "+", since plain `git fetch origin <tag>` only updates
        FETCH_HEAD, not a local tag ref) and checked out detached; a
        branch is fetched into FETCH_HEAD and checked out via
        `checkout -B <ref> FETCH_HEAD`. Refuses to touch a Dir that
        already exists and is not a git repository. Throws (never exits)
        so callers control the exit code.
    #>
    param(
        [Parameter(Mandatory)][string]$Dir,
        [Parameter(Mandatory)][string]$Ref,
        [Parameter(Mandatory)][ValidateSet('tag', 'branch')][string]$Kind,
        [Parameter(Mandatory)][string]$RepoUrl
    )

    $gitDir = Join-Path $Dir '.git'

    if ((Test-Path -LiteralPath $Dir) -and -not (Test-Path -LiteralPath $gitDir)) {
        throw "$Dir already exists and is not a git repository."
    }

    if (Test-Path -LiteralPath $gitDir) {
        if ($Kind -eq 'tag') {
            & git -C $Dir fetch --depth 1 origin "+refs/tags/${Ref}:refs/tags/${Ref}"
            if ($LASTEXITCODE -ne 0) { throw "git fetch failed (exit $LASTEXITCODE)." }
            & git -C $Dir checkout --detach "refs/tags/${Ref}"
            if ($LASTEXITCODE -ne 0) { throw "git checkout failed (exit $LASTEXITCODE)." }
        }
        else {
            & git -C $Dir fetch --depth 1 origin $Ref
            if ($LASTEXITCODE -ne 0) { throw "git fetch failed (exit $LASTEXITCODE)." }
            & git -C $Dir checkout -B $Ref FETCH_HEAD
            if ($LASTEXITCODE -ne 0) { throw "git checkout failed (exit $LASTEXITCODE)." }
        }
    }
    else {
        $parent = Split-Path -Parent $Dir
        if ($parent -and -not (Test-Path -LiteralPath $parent)) {
            New-Item -ItemType Directory -Path $parent -Force | Out-Null
        }
        & git clone --depth 1 --branch $Ref $RepoUrl $Dir
        if ($LASTEXITCODE -ne 0) { throw "git clone failed (exit $LASTEXITCODE)." }
    }
}

# =============================================================================
# CLI body. Guarded so dot-sourcing (tests) only defines the functions
# above and never runs the installer for real -- same pattern as
# tools/release.ps1 and plugin/tools/install.ps1 (see the .DESCRIPTION
# block above for why this guard also runs correctly under `iex`).
# =============================================================================
if ($MyInvocation.InvocationName -ne '.') {

    $helpRequested = $Help.IsPresent -or ($env:NERV_HELP -and $env:NERV_HELP -ne '0')
    if ($helpRequested) {
        Show-NervUsage
        exit 0
    }

    $effectiveChannel = if ($script:NervBoundParameters.ContainsKey('Channel')) {
        $Channel
    }
    elseif ($env:NERV_CHANNEL) {
        $env:NERV_CHANNEL
    }
    else {
        'stable'
    }

    if ($effectiveChannel -notin @('stable', 'alpha', 'rc')) {
        Write-NervError "Unknown channel: $effectiveChannel"
        Show-NervUsage -ToStdErr
        exit 2
    }

    $effectiveDir = if ($script:NervBoundParameters.ContainsKey('Dir')) {
        $Dir
    }
    elseif ($env:NERV_HOME) {
        $env:NERV_HOME
    }
    else {
        Get-NervDefaultDir
    }

    $effectiveNoConfigure = $false
    if ($script:NervBoundParameters.ContainsKey('NoConfigure')) {
        $effectiveNoConfigure = $NoConfigure.IsPresent
    }
    elseif ($env:NERV_NO_CONFIGURE -and $env:NERV_NO_CONFIGURE -ne '0') {
        $effectiveNoConfigure = $true
    }

    try {
        Assert-NervPrereqs
    }
    catch {
        Write-NervError $_.Exception.Message
        exit 1
    }

    $repoUrl = if ($env:NERV_REPO_URL) { $env:NERV_REPO_URL } else { $script:NervRepoUrlDefault }
    $apiBase = if ($env:NERV_API_BASE) { $env:NERV_API_BASE } else { $script:NervApiBaseDefault }
    $fixtureDir = $env:NERV_API_FIXTURE_DIR

    try {
        $resolved = Resolve-NervRef -Channel $effectiveChannel -ApiBase $apiBase -FixtureDir $fixtureDir
    }
    catch {
        Write-NervError $_.Exception.Message
        exit 1
    }

    Write-Host "Resolved channel '$effectiveChannel' to $($resolved.Kind) '$($resolved.Ref)'."

    try {
        Invoke-NervCheckout -Dir $effectiveDir -Ref $resolved.Ref -Kind $resolved.Kind -RepoUrl $repoUrl
    }
    catch {
        Write-NervError $_.Exception.Message
        exit 1
    }

    $installScript = Join-Path $effectiveDir 'plugin/tools/install.ps1'
    $pwshArgs = @('-NoProfile', '-File', $installScript, '-RefreshCache', '-Skills')

    if (-not $effectiveNoConfigure) {
        if (Test-NervInputRedirected) {
            Write-Host "No interactive terminal available; skipping the configuration wizard."
            Write-Host "Run it later with: pwsh $effectiveDir/plugin/tools/configure.ps1"
        }
        else {
            $pwshArgs += '-Configure'
        }
    }

    $pwshExe = if ($env:NERV_PWSH_EXE) { $env:NERV_PWSH_EXE } else { 'pwsh' }
    Write-Host "Running: $pwshExe $($pwshArgs -join ' ')"
    & $pwshExe @pwshArgs
    exit $LASTEXITCODE
}
