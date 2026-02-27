# Where: tools/bootstrap/hyper-v/validate-instance.ps1
# What: Executes smoke verification in a Multipass instance.
# Why: Provide a one-command smoke test flow for Hyper-V bootstrap.
# Note: Validation criteria are defined in ..\cloud-init\verify-instance.sh.
#       SSH/UFW checks are enabled only when Expected* parameters are provided.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$InstanceName,

    [string]$BootstrapUser = "ubuntu",

    [switch]$SkipHelloWorld,

    [AllowEmptyString()]
    [string]$ExpectedSshPasswordAuth = "",

    [int[]]$ExpectedOpenTcpPorts = @()
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$commonPath = Join-Path $scriptDir "..\core\bootstrap-common.psm1"
$commonPath = [System.IO.Path]::GetFullPath($commonPath)
if (-not (Test-Path -LiteralPath $commonPath -PathType Leaf)) {
    throw "Common helper script not found: $commonPath"
}
Import-Module -Name $commonPath -Force

$verifyPath = Join-Path $scriptDir "..\cloud-init\verify-instance.sh"

if (-not (Test-Path -LiteralPath $verifyPath -PathType Leaf)) {
    throw "verify-instance.sh not found: $verifyPath"
}

$multipass = Get-Command multipass -ErrorAction SilentlyContinue
if ($null -eq $multipass) {
    throw "multipass command not found"
}

Invoke-BootstrapNative -Command @("multipass", "transfer", $verifyPath, "$InstanceName`:/tmp/verify-instance.sh") -Context "multipass transfer verify-instance.sh" | Out-Null

Invoke-BootstrapNative -Command @("multipass", "exec", $InstanceName, "--", "sudo", "chmod", "+x", "/tmp/verify-instance.sh") -Context "multipass chmod verify-instance.sh" | Out-Null

$verifyArgs = @(
    "exec",
    $InstanceName,
    "--",
    "sudo",
    "/tmp/verify-instance.sh",
    "--bootstrap-user",
    $BootstrapUser
)
if ($SkipHelloWorld) {
    $verifyArgs += "--skip-hello-world"
}

if (-not [string]::IsNullOrWhiteSpace($ExpectedSshPasswordAuth)) {
    $normalizedExpectedSsh = $ExpectedSshPasswordAuth.Trim().ToLowerInvariant()
    if ($normalizedExpectedSsh -ne "enabled" -and $normalizedExpectedSsh -ne "disabled") {
        throw "ExpectedSshPasswordAuth must be 'enabled' or 'disabled': $ExpectedSshPasswordAuth"
    }
    $verifyArgs += @("--expect-ssh-password-auth", $normalizedExpectedSsh)
}

if ($ExpectedOpenTcpPorts.Count -gt 0) {
    $portList = ($ExpectedOpenTcpPorts | ForEach-Object { [string]$_ }) -join ","
    $verifyArgs += @("--expect-open-tcp-ports", $portList)
}

Invoke-BootstrapNative -Command (@("multipass") + $verifyArgs) -Context "multipass smoke verification" | Out-Null

Write-Host "[validate-multipass] OK"
