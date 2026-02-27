# Where: tools/bootstrap/hyper-v/create-instance.ps1
# What: Creates and bootstraps a new Multipass instance with cloud-init.
# Why: Enforce new-instance operational model for Hyper-V environment setup.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$InstanceName,

    [Parameter(Mandatory = $true)]
    [string]$VarsFile,

    [string]$UserDataPath,

    [int]$Cpus = 2,

    [string]$Memory = "4G",

    [string]$Disk = "30G",

    [string]$NetworkHub,

    [string]$BootstrapUser = "ubuntu",

    # Kept for backward compatibility. Recreate now happens automatically when name already exists.
    [switch]$Force,

    [switch]$SkipPreflight,

    [switch]$RunSmokeTest
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Read-VarsFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $vars = @{}
    $lineNumber = 0
    foreach ($rawLine in Get-Content -LiteralPath $Path) {
        $lineNumber += 1
        $line = $rawLine.Trim()

        if ([string]::IsNullOrWhiteSpace($line) -or $line.StartsWith("#")) {
            continue
        }

        $separatorIndex = $line.IndexOf("=")
        if ($separatorIndex -lt 1) {
            throw "Invalid vars line $lineNumber in ${Path}: '$rawLine' (expected KEY=VALUE)"
        }

        $key = $line.Substring(0, $separatorIndex).Trim()
        $value = $line.Substring($separatorIndex + 1).Trim()

        if ((($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) -and $value.Length -ge 2) {
            $value = $value.Substring(1, $value.Length - 2)
        }

        $vars[$key] = $value
    }

    return $vars
}

function Get-OptionalVar {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$Vars,

        [Parameter(Mandatory = $true)]
        [string]$Key,

        [AllowEmptyString()]
        [string]$DefaultValue = ""
    )

    if (-not $Vars.ContainsKey($Key)) {
        return $DefaultValue
    }

    $value = [string]$Vars[$Key]
    if ([string]::IsNullOrWhiteSpace($value)) {
        return $DefaultValue
    }

    return $value
}

function Ensure-SingleLineValue {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [AllowEmptyString()]
        [string]$Value
    )

    if ($Value.Contains("`r") -or $Value.Contains("`n")) {
        throw "$Name must be a single-line value"
    }
}

function Resolve-SettingValue {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$Vars,
        [Parameter(Mandatory = $true)]
        [string]$VarsKey
    )

    $varsValue = Get-OptionalVar -Vars $Vars -Key $VarsKey -DefaultValue ""
    if (-not [string]::IsNullOrWhiteSpace($varsValue)) {
        return $varsValue
    }

    return ""
}

function Parse-PositiveInt {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [string]$Value
    )

    $parsed = 0
    if ((-not [int]::TryParse($Value, [ref]$parsed)) -or $parsed -lt 1) {
        throw "$Name must be a positive integer: $Value"
    }

    return $parsed
}

function New-RandomPassword {
    param(
        [int]$Length = 8
    )

    if ($Length -lt 1) {
        throw "Password length must be greater than zero."
    }

    $charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789".ToCharArray()
    $bytes = New-Object byte[] ($Length * 2)
    $passwordChars = New-Object 'System.Collections.Generic.List[char]'
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        while ($passwordChars.Count -lt $Length) {
            $rng.GetBytes($bytes)
            foreach ($b in $bytes) {
                if ($passwordChars.Count -ge $Length) {
                    break
                }
                $passwordChars.Add($charset[$b % $charset.Length]) | Out-Null
            }
        }
    }
    finally {
        $rng.Dispose()
    }

    return -join $passwordChars
}

function Write-NoticeBox {
    param(
        [string]$Title,
        [string[]]$Lines
    )

    $content = @()
    if (-not [string]::IsNullOrWhiteSpace($Title)) {
        $content += "[ $Title ]"
    }
    if ($null -ne $Lines) {
        $content += $Lines
    }
    if ($content.Count -eq 0) {
        return
    }

    $maxLength = 0
    foreach ($line in $content) {
        if ($line.Length -gt $maxLength) {
            $maxLength = $line.Length
        }
    }

    $border = "+" + ("-" * ($maxLength + 2)) + "+"
    Write-Host $border -ForegroundColor Yellow
    foreach ($line in $content) {
        $padding = " " * ($maxLength - $line.Length)
        Write-Host ("| " + $line + $padding + " |") -ForegroundColor Yellow
    }
    Write-Host $border -ForegroundColor Yellow
}

function Resolve-ExistingFilePath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InputPath,

        [Parameter(Mandatory = $true)]
        [string]$ScriptDir
    )

    if ([System.IO.Path]::IsPathRooted($InputPath)) {
        return [System.IO.Path]::GetFullPath($InputPath)
    }

    $bootstrapRoot = [System.IO.Path]::GetFullPath((Join-Path $ScriptDir ".."))
    $projectRoot = [System.IO.Path]::GetFullPath((Join-Path $bootstrapRoot "..\.."))

    $candidates = @(
        [System.IO.Path]::GetFullPath((Join-Path $ScriptDir $InputPath)),
        [System.IO.Path]::GetFullPath($InputPath),
        [System.IO.Path]::GetFullPath((Join-Path $projectRoot $InputPath)),
        [System.IO.Path]::GetFullPath((Join-Path $bootstrapRoot $InputPath)),
        [System.IO.Path]::GetFullPath((Join-Path (Join-Path $bootstrapRoot "cloud-init") $InputPath))
    )

    $seen = @{}
    foreach ($candidate in $candidates) {
        if ($seen.ContainsKey($candidate)) {
            continue
        }
        $seen[$candidate] = $true
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return $candidate
        }
    }

    return $candidates[0]
}

function Confirm-RecreateIfNeeded {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    try {
        $answer = Read-Host "$Message [y/N]"
    }
    catch {
        throw "Could not prompt for confirmation. Re-run with -Force to recreate without confirmation."
    }

    if ($answer -notmatch '^(?i:y|yes)$') {
        throw "Recreate cancelled by user."
    }
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$preflightPath = Join-Path $scriptDir "preflight.ps1"
$renderPath = Join-Path $scriptDir "..\core\render-user-data.ps1"
$validatePath = Join-Path $scriptDir "validate-instance.ps1"

if (-not (Get-Command multipass -ErrorAction SilentlyContinue)) {
    throw "multipass command not found"
}

if (-not (Test-Path -LiteralPath $renderPath -PathType Leaf)) {
    throw "Shared renderer not found: $renderPath"
}

if (-not $SkipPreflight) {
    & $preflightPath
}

if ([string]::IsNullOrWhiteSpace($UserDataPath)) {
    $UserDataPath = Join-Path $env:TEMP "$InstanceName-user-data.yaml"
}
$UserDataPath = [System.IO.Path]::GetFullPath($UserDataPath)
$varsFilePath = Resolve-ExistingFilePath -InputPath $VarsFile -ScriptDir $scriptDir
if (-not (Test-Path -LiteralPath $varsFilePath -PathType Leaf)) {
    throw "Vars file not found: $varsFilePath"
}

$vars = Read-VarsFile -Path $varsFilePath
$bootstrapUserFromVars = Get-OptionalVar -Vars $vars -Key "BOOTSTRAP_USER" -DefaultValue "ubuntu"
$effectiveBootstrapUser = if ($PSBoundParameters.ContainsKey("BootstrapUser")) {
    $BootstrapUser
}
elseif (-not [string]::IsNullOrWhiteSpace($bootstrapUserFromVars)) {
    $bootstrapUserFromVars
}
else {
    "ubuntu"
}

$cpusFromConfig = Resolve-SettingValue -Vars $vars -VarsKey "HYPERV_CPUS"
$memoryFromConfig = Resolve-SettingValue -Vars $vars -VarsKey "HYPERV_MEMORY"
$diskFromConfig = Resolve-SettingValue -Vars $vars -VarsKey "HYPERV_DISK"
$networkHubFromConfig = Resolve-SettingValue -Vars $vars -VarsKey "HYPERV_NETWORK_HUB"

if (-not $PSBoundParameters.ContainsKey("Cpus") -and -not [string]::IsNullOrWhiteSpace($cpusFromConfig)) {
    $Cpus = Parse-PositiveInt -Name "HYPERV_CPUS" -Value $cpusFromConfig
}
if (-not $PSBoundParameters.ContainsKey("Memory") -and -not [string]::IsNullOrWhiteSpace($memoryFromConfig)) {
    Ensure-SingleLineValue -Name "HYPERV_MEMORY" -Value $memoryFromConfig
    $Memory = $memoryFromConfig
}
if (-not $PSBoundParameters.ContainsKey("Disk") -and -not [string]::IsNullOrWhiteSpace($diskFromConfig)) {
    Ensure-SingleLineValue -Name "HYPERV_DISK" -Value $diskFromConfig
    $Disk = $diskFromConfig
}
if (-not $PSBoundParameters.ContainsKey("NetworkHub") -and -not [string]::IsNullOrWhiteSpace($networkHubFromConfig)) {
    Ensure-SingleLineValue -Name "HYPERV_NETWORK_HUB" -Value $networkHubFromConfig
    $NetworkHub = $networkHubFromConfig
}

if ($Cpus -lt 1) {
    throw "Cpus must be a positive integer: $Cpus"
}
Ensure-SingleLineValue -Name "Memory" -Value $Memory
Ensure-SingleLineValue -Name "Disk" -Value $Disk
if (-not [string]::IsNullOrWhiteSpace($NetworkHub)) {
    Ensure-SingleLineValue -Name "NetworkHub" -Value $NetworkHub
}

$rootPassword = New-RandomPassword -Length 8

& $renderPath -VarsFile $varsFilePath -Output $UserDataPath -RootPassword $rootPassword

& multipass info $InstanceName 1>$null 2>$null
$instanceExists = ($LASTEXITCODE -eq 0)

if ($instanceExists) {
    if (-not $Force) {
        Confirm-RecreateIfNeeded -Message "Instance '$InstanceName' already exists. Delete and recreate?"
    }

    Write-Host "[hyper-v create] Existing instance '$InstanceName' found. Deleting to enforce fresh creation."
    & multipass delete $InstanceName | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to delete instance '$InstanceName'"
    }
    & multipass purge | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to purge deleted instances"
    }
}

Write-Host "[hyper-v create] Launching '$InstanceName'"
$launchArgs = @(
    "launch",
    "24.04",
    "--name", $InstanceName,
    "--cpus", "$Cpus",
    "--memory", $Memory,
    "--disk", $Disk,
    "--cloud-init", $UserDataPath
)
if (-not [string]::IsNullOrWhiteSpace($NetworkHub)) {
    $launchArgs += @("--network", "name=$NetworkHub")
}
$networkDisplay = if ([string]::IsNullOrWhiteSpace($NetworkHub)) { "default" } else { $NetworkHub }
Write-Host "[hyper-v create] launch config: cpus=$Cpus memory=$Memory disk=$Disk network=$networkDisplay"
& multipass @launchArgs
if ($LASTEXITCODE -ne 0) {
    throw "multipass launch failed for '$InstanceName'"
}

Write-Host "[hyper-v create] Waiting for cloud-init completion"
$cloudInitLines = @(& multipass exec $InstanceName -- cloud-init status --long 2>&1 | ForEach-Object { [string]$_ })
$cloudInitLong = $cloudInitLines -join "`n"
$cloudInitExit = $LASTEXITCODE
Write-Host $cloudInitLong

$cloudInitStatus = $null
foreach ($line in $cloudInitLines) {
    $normalized = ($line -replace "`e\[[\d;]*[A-Za-z]", "").Trim()
    if ($normalized -match "^(?i)status\s*:\s*(.+)$") {
        $cloudInitStatus = $Matches[1].Trim()
        break
    }
}

if ([string]::IsNullOrWhiteSpace($cloudInitStatus)) {
    throw "cloud-init status line could not be parsed in '$InstanceName'"
}

if ($cloudInitStatus -notmatch "^(?i)done$") {
    throw "cloud-init did not complete successfully in '$InstanceName'"
}

if ($cloudInitExit -ne 0) {
    Write-Host "[hyper-v create] WARN: cloud-init returned non-zero (degraded), but status is done"
}

if ($RunSmokeTest) {
    if (-not (Test-Path -LiteralPath $validatePath -PathType Leaf)) {
        throw "validate-instance.ps1 not found: $validatePath"
    }
    & $validatePath -InstanceName $InstanceName -BootstrapUser $effectiveBootstrapUser
}
else {
    Write-Host "[hyper-v create] Smoke test example:"
    Write-Host ".\tools\bootstrap\hyper-v\validate-instance.ps1 -InstanceName $InstanceName -BootstrapUser $effectiveBootstrapUser"
}

Write-Host "[hyper-v create] Completed: $InstanceName"
Write-NoticeBox -Title "ROOT ACCESS" -Lines @(
    "Instance: $InstanceName"
    "Initial root password: $rootPassword"
    "Reset command:"
    "  multipass exec $InstanceName -- sudo passwd root"
)
