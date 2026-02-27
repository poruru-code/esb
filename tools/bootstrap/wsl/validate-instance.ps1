# Where: tools/bootstrap/wsl/validate-instance.ps1
# What: Executes smoke verification in a WSL distro.
# Why: Provide one-command WSL validation equivalent to Multipass flow.
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

function Convert-WindowsPathToWsl {
    param([Parameter(Mandatory = $true)][string]$Path)

    $full = [System.IO.Path]::GetFullPath($Path)
    if ($full -notmatch '^[A-Za-z]:\\') {
        throw "Path must be an absolute Windows path: $full"
    }

    $drive = $full.Substring(0, 1).ToLowerInvariant()
    $rest = $full.Substring(2) -replace '\\', '/'
    $rest = $rest.TrimStart('/')
    return "/mnt/$drive/$rest"
}

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

$wsl = Get-Command wsl -ErrorAction SilentlyContinue
if ($null -eq $wsl) {
    throw "wsl command not found"
}

$distroResult = Invoke-BootstrapNative -Command @("wsl", "--list", "--quiet") -Context "wsl list distros" -CaptureOutput
$distros = @($distroResult.OutputLines | ForEach-Object { $_.Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
if (-not ($distros -contains $InstanceName)) {
    throw "WSL distro '$InstanceName' not found"
}

$verifyWslPath = Convert-WindowsPathToWsl -Path ([System.IO.Path]::GetFullPath($verifyPath))
$verifyWslLiteral = Convert-ToBashSingleQuotedLiteral -Value $verifyWslPath
$bootstrapUserLiteral = Convert-ToBashSingleQuotedLiteral -Value $BootstrapUser
$skipHelloWorldArg = if ($SkipHelloWorld) { " --skip-hello-world" } else { "" }

$cmd = @'
set -euo pipefail
cp __VERIFY_WSL_PATH__ /tmp/verify-instance.sh
chmod +x /tmp/verify-instance.sh
/tmp/verify-instance.sh --bootstrap-user __BOOTSTRAP_USER____SKIP_HELLO_WORLD__ --allow-cloud-init-disabled
'@
$cmd = $cmd.
    Replace("__VERIFY_WSL_PATH__", $verifyWslLiteral).
    Replace("__BOOTSTRAP_USER__", $bootstrapUserLiteral).
    Replace("__SKIP_HELLO_WORLD__", $skipHelloWorldArg)

Invoke-BootstrapNative -Command @("wsl", "-d", $InstanceName, "--user", "root", "--", "bash", "-lc", $cmd) -Context "wsl smoke verification" | Out-Null

Write-Host "[validate-wsl] OK"
