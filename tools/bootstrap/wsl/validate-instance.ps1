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

function Convert-ToBashSingleQuotedLiteral {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value)
    $replacement = @'
'"'"'
'@
    $escaped = $Value.Replace("'", $replacement)
    return "'" + $escaped + "'"
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$verifyPath = Join-Path $scriptDir "..\cloud-init\verify-instance.sh"
if (-not (Test-Path -LiteralPath $verifyPath -PathType Leaf)) {
    throw "verify-instance.sh not found: $verifyPath"
}

$wsl = Get-Command wsl -ErrorAction SilentlyContinue
if ($null -eq $wsl) {
    throw "wsl command not found"
}

$distros = @(& wsl --list --quiet)
$distros = @($distros | ForEach-Object { $_.Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
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

& wsl -d $InstanceName --user root -- bash -lc $cmd
if ($LASTEXITCODE -ne 0) {
    throw "WSL smoke verification failed in $InstanceName"
}

Write-Host "[validate-wsl] OK"
