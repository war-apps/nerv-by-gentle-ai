# Tests the functions of scripts/install.ps1 without running the install:
# each function is loaded from the script's syntax tree, so nothing is
# downloaded and nothing is written outside a temp directory.
$ErrorActionPreference = 'Stop'

$scriptPath = Join-Path $PSScriptRoot '..\scripts\install.ps1'
$ast = [System.Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$null, [ref]$null)
$functions = $ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $true)
foreach ($function in $functions) {
    Invoke-Expression $function.Extent.Text
}

$script:pass = 0
$script:fail = 0

function Assert-Case {
    param([string]$Name, [scriptblock]$Body)
    try {
        & $Body
        $script:pass++
        Write-Host "ok   $Name"
    } catch {
        $script:fail++
        Write-Host "FAIL $Name`: $_"
    }
}

$dir = Join-Path $env:TEMP ('install-ps1-test-' + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $dir | Out-Null
try {
    $archive = Join-Path $dir 'nerv_1.0.0_windows_amd64.zip'
    Set-Content -Path $archive -Value 'fixture archive'
    $hash = (Get-FileHash -Path $archive -Algorithm SHA256).Hash.ToLower()
    $other = 'nerv_1.0.0_windows_arm64.zip'

    Assert-Case 'Test-Checksum accepts the matching entry of a dotted version' {
        $sums = Join-Path $dir 'ok.txt'
        Set-Content -Path $sums -Value @("0000  $other", "$hash  nerv_1.0.0_windows_amd64.zip")
        Test-Checksum -ArchivePath $archive -ChecksumsPath $sums
    }

    Assert-Case 'Test-Checksum refuses a mismatching hash' {
        $sums = Join-Path $dir 'bad.txt'
        Set-Content -Path $sums -Value "deadbeef  nerv_1.0.0_windows_amd64.zip"
        $threw = $false
        try { Test-Checksum -ArchivePath $archive -ChecksumsPath $sums } catch { $threw = $true }
        if (-not $threw) { throw 'expected a checksum mismatch' }
    }

    Assert-Case 'Test-Checksum refuses an archive missing from checksums.txt' {
        $sums = Join-Path $dir 'missing.txt'
        Set-Content -Path $sums -Value "0000  $other"
        $threw = $false
        try { Test-Checksum -ArchivePath $archive -ChecksumsPath $sums } catch { $threw = $true }
        if (-not $threw) { throw 'expected a missing-entry error' }
    }

    Assert-Case 'Get-Arch maps AMD64 to amd64' {
        $old = $env:PROCESSOR_ARCHITECTURE
        $oldW6432 = $env:PROCESSOR_ARCHITEW6432
        try {
            $env:PROCESSOR_ARCHITECTURE = 'AMD64'
            $env:PROCESSOR_ARCHITEW6432 = $null
            if ((Get-Arch) -ne 'amd64') { throw "expected amd64" }
        } finally {
            $env:PROCESSOR_ARCHITECTURE = $old
            $env:PROCESSOR_ARCHITEW6432 = $oldW6432
        }
    }

    Assert-Case 'Get-Arch maps ARM64 to arm64' {
        $old = $env:PROCESSOR_ARCHITECTURE
        $oldW6432 = $env:PROCESSOR_ARCHITEW6432
        try {
            $env:PROCESSOR_ARCHITECTURE = 'ARM64'
            $env:PROCESSOR_ARCHITEW6432 = $null
            if ((Get-Arch) -ne 'arm64') { throw "expected arm64" }
        } finally {
            $env:PROCESSOR_ARCHITECTURE = $old
            $env:PROCESSOR_ARCHITEW6432 = $oldW6432
        }
    }

    Assert-Case 'Get-Arch refuses an unknown architecture' {
        $old = $env:PROCESSOR_ARCHITECTURE
        $oldW6432 = $env:PROCESSOR_ARCHITEW6432
        try {
            $env:PROCESSOR_ARCHITECTURE = 'IA64'
            $env:PROCESSOR_ARCHITEW6432 = $null
            $threw = $false
            try { Get-Arch | Out-Null } catch { $threw = $true }
            if (-not $threw) { throw 'expected an unsupported-architecture error' }
        } finally {
            $env:PROCESSOR_ARCHITECTURE = $old
            $env:PROCESSOR_ARCHITEW6432 = $oldW6432
        }
    }

    Assert-Case 'Resolve-Version returns NERV_VERSION unchanged when set, with no API call' {
        $old = $env:NERV_VERSION
        try {
            $env:NERV_VERSION = 'v9.9.9'
            if ((Resolve-Version) -ne 'v9.9.9') { throw 'expected the explicit version' }
        } finally {
            $env:NERV_VERSION = $old
        }
    }
} finally {
    Remove-Item -Path $dir -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "$($script:pass) passed, $($script:fail) failed"
if ($script:fail -gt 0) { exit 1 }
