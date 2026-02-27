# Where: tools/bootstrap/hyper-v/validate-instance.ps1
# What: Executes smoke verification in a Multipass instance.
# Why: Provide a one-command smoke test flow for Hyper-V bootstrap.
# Note: Validation criteria are defined in ..\cloud-init\verify-instance.sh.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$InstanceName,

    [string]$BootstrapUser = "ubuntu",

    [switch]$SkipHelloWorld
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$verifyPath = Join-Path $scriptDir "..\cloud-init\verify-instance.sh"

if (-not (Test-Path -LiteralPath $verifyPath -PathType Leaf)) {
    throw "verify-instance.sh not found: $verifyPath"
}

$multipass = Get-Command multipass -ErrorAction SilentlyContinue
if ($null -eq $multipass) {
    throw "multipass command not found"
}

& multipass transfer $verifyPath "$InstanceName`:/tmp/verify-instance.sh"
if ($LASTEXITCODE -ne 0) {
    throw "Failed to transfer verify-instance.sh to $InstanceName"
}

& multipass exec $InstanceName -- sudo chmod +x /tmp/verify-instance.sh
if ($LASTEXITCODE -ne 0) {
    throw "Failed to chmod verify-instance.sh in $InstanceName"
}

$cmd = "sudo /tmp/verify-instance.sh --bootstrap-user $BootstrapUser"
if ($SkipHelloWorld) {
    $cmd += " --skip-hello-world"
}

& multipass exec $InstanceName -- bash -lc $cmd
if ($LASTEXITCODE -ne 0) {
    throw "Smoke verification failed in $InstanceName"
}

Write-Host "[validate-multipass] OK"
