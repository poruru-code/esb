# Where: tools/bootstrap/hyper-v/preflight.ps1
# What: Mandatory preflight checks for Hyper-V (Multipass) bootstrap flow.
# Why: Detect host prerequisites early and return actionable remediation guidance.
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$commonPath = Join-Path $scriptDir "..\core\bootstrap-common.psm1"
$commonPath = [System.IO.Path]::GetFullPath($commonPath)
if (-not (Test-Path -LiteralPath $commonPath -PathType Leaf)) {
    throw "Common helper script not found: $commonPath"
}
Import-Module -Name $commonPath -Force

$failures = New-Object System.Collections.Generic.List[string]
$warnings = New-Object System.Collections.Generic.List[string]

function Write-Pass {
    param([Parameter(Mandatory = $true)][string]$Message)
    Write-Host "[hyper-v preflight] PASS: $Message"
}

function Add-Warning {
    param([Parameter(Mandatory = $true)][string]$Message)
    $warnings.Add($Message)
    Write-Host "[hyper-v preflight] WARN: $Message"
}

function Add-Failure {
    param([Parameter(Mandatory = $true)][string]$Message)
    $failures.Add($Message)
    Write-Host "[hyper-v preflight] FAIL: $Message"
}

if ([Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) {
    Write-Pass "Windows host detected"
}
else {
    Add-Failure "This script must run on Windows PowerShell"
}

$multipassCmd = Get-Command multipass -ErrorAction SilentlyContinue
if ($null -eq $multipassCmd) {
    Add-Failure "multipass command not found. Install Multipass before continuing."
}
else {
    Write-Pass "multipass command available"
}

try {
    $hyperVFeature = Get-WindowsOptionalFeature -Online -FeatureName "Microsoft-Hyper-V-All" -ErrorAction Stop
    if ($hyperVFeature.State -eq "Enabled") {
        Write-Pass "Microsoft-Hyper-V-All feature is enabled"
    }
    else {
        Add-Failure "Hyper-V feature state is '$($hyperVFeature.State)'. Enable Hyper-V and reboot."
    }
}
catch {
    Add-Failure "Could not read Hyper-V feature state. Run this script in an elevated PowerShell session. Detail: $($_.Exception.Message)"
}

try {
    $computerSystem = Get-CimInstance -ClassName Win32_ComputerSystem -ErrorAction Stop
    if ($computerSystem.HypervisorPresent) {
        Write-Pass "Hypervisor is running"
    }
    else {
        Add-Failure "Hypervisor is not running. Verify BIOS virtualization and run 'bcdedit /set hypervisorlaunchtype auto', then reboot."
    }
}
catch {
    Add-Failure "Could not query hypervisor state via CIM. Detail: $($_.Exception.Message)"
}

try {
    $processors = @(Get-CimInstance -ClassName Win32_Processor -ErrorAction Stop)
    if ($processors.Count -gt 0) {
        $firmwareDisabled = @($processors | Where-Object { -not $_.VirtualizationFirmwareEnabled })
        if ($firmwareDisabled.Count -gt 0) {
            Add-Warning "One or more CPUs report VirtualizationFirmwareEnabled=false. If launches fail, enable virtualization in BIOS/UEFI."
        }
        else {
            Write-Pass "CPU virtualization firmware flag is enabled"
        }
    }
}
catch {
    Add-Warning "Could not query CPU virtualization firmware flag. Detail: $($_.Exception.Message)"
}

$multipassService = Get-Service -Name "Multipass" -ErrorAction SilentlyContinue
if ($null -eq $multipassService) {
    Add-Warning "Multipass Windows service was not found. Multipass installation might be incomplete."
}
elseif ($multipassService.Status -eq "Running") {
    Write-Pass "Multipass service is running"
}
else {
    Add-Warning "Multipass service is '$($multipassService.Status)'. Start-Service Multipass if launch commands fail."
}

if ($null -ne $multipassCmd) {
    try {
        $versionResult = Invoke-BootstrapNative -Command @("multipass", "version") -Context "multipass version preflight check" -CaptureOutput -IgnoreExitCode
        $versionLines = @($versionResult.OutputLines)
        $versionExitCode = $versionResult.ExitCode
        if ($versionExitCode -eq 0) {
            Write-Pass "multipass command execution succeeded"
        }
        else {
            $detail = ($versionLines -join "`n").Trim()
            if ([string]::IsNullOrWhiteSpace($detail)) {
                Add-Failure "multipass command exists but returned exit code $versionExitCode."
            }
            else {
                Add-Failure "multipass command exists but returned exit code $versionExitCode. Detail: $detail"
            }
        }
    }
    catch {
        Add-Failure "multipass command exists but could not execute successfully. Detail: $($_.Exception.Message)"
    }
}

if ($warnings.Count -gt 0) {
    Write-Host "[hyper-v preflight] warnings:"
    foreach ($warning in $warnings) {
        Write-Host "  - $warning"
    }
}

if ($failures.Count -gt 0) {
    Write-Host "[hyper-v preflight] errors:"
    foreach ($failure in $failures) {
        Write-Host "  - $failure"
    }
    throw "Hyper-V preflight failed with $($failures.Count) error(s)"
}

Write-Host "[hyper-v preflight] OK"
