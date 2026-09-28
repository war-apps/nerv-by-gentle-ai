#requires -Version 7
<#
.SYNOPSIS
    Thin forwarder. The real script lives at
    plugin/tools/configure-models.ps1 (it ships inside the plugin, so it
    is the copy that actually runs from the installed plugin cache). This
    root-level copy exists only so the documented terminal command
    (`pwsh tools/configure-models.ps1 ...`) keeps working when run from a
    repo checkout.
#>
& "$PSScriptRoot/../plugin/tools/configure-models.ps1" @args
exit $LASTEXITCODE
