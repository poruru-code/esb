# Where: tools/bootstrap/wsl/preflight.ps1
# What: Mandatory preflight checks for Windows-hosted WSL bootstrap flow.
# Why: Detect missing host prerequisites before distro creation starts.
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$failures = New-Object System.Collections.Generic.List[string]
$warnings = New-Object System.Collections.Generic.List[string]

function Write-Pass {
    param([Parameter(Mandatory = $true)][string]$Message)
    Write-Host "[wsl preflight] PASS: $Message"
}

function Add-Warning {
    param([Parameter(Mandatory = $true)][string]$Message)
    $warnings.Add($Message)
    Write-Host "[wsl preflight] WARN: $Message"
}

function Add-Failure {
    param([Parameter(Mandatory = $true)][string]$Message)
    $failures.Add($Message)
    Write-Host "[wsl preflight] FAIL: $Message"
}

if ([Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) {
    Write-Pass "Windows host detected"
}
else {
    Add-Failure "This script must run on Windows PowerShell"
}

$wslCmd = Get-Command wsl -ErrorAction SilentlyContinue
if ($null -eq $wslCmd) {
    Add-Failure "wsl command not found. Enable/install WSL before continuing."
}
else {
    Write-Pass "wsl command available"
}

if ($null -ne $wslCmd) {
    try {
        & wsl --status | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Pass "wsl --status command succeeded"
        }
        else {
            Add-Failure "wsl --status returned exit code $LASTEXITCODE. Complete WSL setup and reboot if required."
        }
    }
    catch {
        Add-Failure "Failed to execute wsl --status. Detail: $($_.Exception.Message)"
    }
}

$optionalFeatures = @(
    "Microsoft-Windows-Subsystem-Linux",
    "VirtualMachinePlatform"
)
foreach ($featureName in $optionalFeatures) {
    try {
        $feature = Get-WindowsOptionalFeature -Online -FeatureName $featureName -ErrorAction Stop
        if ($feature.State -eq "Enabled") {
            Write-Pass "$featureName is enabled"
        }
        else {
            Add-Failure "$featureName state is '$($feature.State)'. Enable it and reboot."
        }
    }
    catch {
        Add-Warning "Could not read $featureName state. Run elevated PowerShell for detailed diagnostics. Detail: $($_.Exception.Message)"
    }
}

if ($warnings.Count -gt 0) {
    Write-Host "[wsl preflight] warnings:"
    foreach ($warning in $warnings) {
        Write-Host "  - $warning"
    }
}

if ($failures.Count -gt 0) {
    Write-Host "[wsl preflight] errors:"
    foreach ($failure in $failures) {
        Write-Host "  - $failure"
    }
    throw "WSL preflight failed with $($failures.Count) error(s)"
}

Write-Host "[wsl preflight] OK"
