# Where: tools/bootstrap/core/bootstrap-common.psm1
# What: Shared helper functions for bootstrap entrypoint scripts.
# Why: Eliminate duplicated utility logic across WSL and Hyper-V flows.

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

function Convert-ToBashSingleQuotedLiteral {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value)
    $replacement = @'
'"'"'
'@
    $escaped = $Value.Replace("'", $replacement)
    return "'" + $escaped + "'"
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

function Assert-AllowedVarKeys {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$Vars,

        [Parameter(Mandatory = $true)]
        [string[]]$AllowedKeys,

        [Parameter(Mandatory = $true)]
        [string]$VarsFilePath
    )

    $allowed = @{}
    foreach ($key in $AllowedKeys) {
        if (-not [string]::IsNullOrWhiteSpace($key)) {
            $allowed[$key] = $true
        }
    }

    $unknown = @()
    foreach ($key in $Vars.Keys) {
        if (-not $allowed.ContainsKey([string]$key)) {
            $unknown += [string]$key
        }
    }

    if ($unknown.Count -gt 0) {
        $unknownSorted = @($unknown | Sort-Object -Unique)
        $allowedSorted = @($AllowedKeys | Sort-Object -Unique)
        throw "Unknown vars key(s) in ${VarsFilePath}: $($unknownSorted -join ', '). Allowed keys: $($allowedSorted -join ', ')"
    }
}

function Invoke-BootstrapNative {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Command,

        [Parameter(Mandatory = $true)]
        [string]$Context,

        [switch]$CaptureOutput,

        [switch]$IgnoreExitCode
    )

    if ($Command.Count -lt 1) {
        throw "Invoke-BootstrapNative requires at least one command token."
    }

    $exe = $Command[0]
    $args = @()
    if ($Command.Count -gt 1) {
        $args = $Command[1..($Command.Count - 1)]
    }

    $outputLines = @()
    if ($CaptureOutput) {
        $outputLines = @(& $exe @args 2>&1 | ForEach-Object { [string]$_ })
    }
    else {
        & $exe @args
    }

    $exitCode = $LASTEXITCODE
    if (-not $IgnoreExitCode -and $exitCode -ne 0) {
        if ($CaptureOutput) {
            $details = ($outputLines -join "`n").Trim()
            if ([string]::IsNullOrWhiteSpace($details)) {
                throw "$Context failed (exit=$exitCode): $($Command -join ' ')"
            }
            throw "$Context failed (exit=$exitCode): $($Command -join ' ')`n$details"
        }
        throw "$Context failed (exit=$exitCode): $($Command -join ' ')"
    }

    return [pscustomobject]@{
        ExitCode    = $exitCode
        OutputLines = @($outputLines)
    }
}

Export-ModuleMember -Function @(
    "Read-VarsFile",
    "Get-OptionalVar",
    "Convert-ToBashSingleQuotedLiteral",
    "New-RandomPassword",
    "Write-NoticeBox",
    "Confirm-RecreateIfNeeded",
    "Resolve-ExistingFilePath",
    "Assert-AllowedVarKeys",
    "Invoke-BootstrapNative"
)
