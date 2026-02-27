# Where: tools/bootstrap/core/render-user-data.ps1
# What: Renders cloud-init user-data from vars file.
# Why: Keep rendering logic shared across WSL and Hyper-V bootstrap flows.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$VarsFile,

    [Parameter(Mandatory = $true)]
    [string]$Output,

    [AllowEmptyString()]
    [string]$RootPassword = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$templatePath = Join-Path $scriptDir "..\cloud-init\user-data.template.yaml"

if (-not (Test-Path -LiteralPath $templatePath -PathType Leaf)) {
    throw "Template not found: $templatePath"
}

if (-not (Test-Path -LiteralPath $VarsFile -PathType Leaf)) {
    throw "Vars file not found: $VarsFile"
}

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

function Validate-Scalar {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Key,

        [Parameter(Mandatory = $true)]
        [AllowEmptyString()]
        [string]$Value
    )

    if ($Value.Contains("`r") -or $Value.Contains("`n")) {
        throw "$Key must be a single-line value"
    }

    if ($Value.Contains('"')) {
        throw "$Key must not contain double-quote characters (`")"
    }
}

function Build-CaCertsBlock {
    param(
        [Parameter(Mandatory = $true)]
        [AllowEmptyString()]
        [string]$CaCertPath,

        [Parameter(Mandatory = $true)]
        [string]$VarsFilePath
    )

    if ([string]::IsNullOrWhiteSpace($CaCertPath)) {
        return "# no custom CA certificate provided"
    }

    $resolvedPath = if ([System.IO.Path]::IsPathRooted($CaCertPath)) {
        [System.IO.Path]::GetFullPath($CaCertPath)
    }
    else {
        $varsDir = Split-Path -Parent ([System.IO.Path]::GetFullPath($VarsFilePath))
        [System.IO.Path]::GetFullPath((Join-Path $varsDir $CaCertPath))
    }

    if (-not (Test-Path -LiteralPath $resolvedPath -PathType Leaf)) {
        throw "SSL_INSPECTION_CA_CERT_PATH not found: $resolvedPath"
    }

    $rawText = [System.IO.File]::ReadAllText($resolvedPath)
    $pem = ""

    if ($rawText -match "-----BEGIN CERTIFICATE-----") {
        $pem = $rawText.Trim()
    }
    else {
        $certBytes = [System.IO.File]::ReadAllBytes($resolvedPath)
        $parsed = $false

        try {
            $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 -ArgumentList (, $certBytes)
            $parsed = $true
        }
        catch {
            $normalizedBase64 = ($rawText -replace "\s", "")
            if (-not [string]::IsNullOrWhiteSpace($normalizedBase64)) {
                try {
                    $decodedBytes = [Convert]::FromBase64String($normalizedBase64)
                    $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 -ArgumentList (, $decodedBytes)
                    $parsed = $true
                }
                catch {
                    $parsed = $false
                }
            }
        }

        if (-not $parsed) {
            throw "SSL_INSPECTION_CA_CERT_PATH must point to a valid certificate file (.cer/.crt/.pem): $resolvedPath"
        }

        $der = $cert.Export([System.Security.Cryptography.X509Certificates.X509ContentType]::Cert)
        $base64 = [System.Convert]::ToBase64String($der)
        $pemLines = [System.Collections.Generic.List[string]]::new()
        $pemLines.Add("-----BEGIN CERTIFICATE-----")
        for ($offset = 0; $offset -lt $base64.Length; $offset += 64) {
            $length = [System.Math]::Min(64, $base64.Length - $offset)
            $pemLines.Add($base64.Substring($offset, $length))
        }
        $pemLines.Add("-----END CERTIFICATE-----")
        $pem = ($pemLines -join "`n")
    }

    if ([string]::IsNullOrWhiteSpace($pem)) {
        throw "SSL_INSPECTION_CA_CERT_PATH resolved to an empty certificate: $resolvedPath"
    }

    $indentedPem = (($pem -split "`r?`n") | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | ForEach-Object { "        $_" }) -join "`n"
    return "ca_certs:`n  remove_defaults: false`n  trusted:`n    - |`n$indentedPem"
}

$vars = Read-VarsFile -Path $VarsFile

$proxyHttp = Get-OptionalVar -Vars $vars -Key "PROXY_HTTP" -DefaultValue ""
$proxyHttps = Get-OptionalVar -Vars $vars -Key "PROXY_HTTPS" -DefaultValue ""
$noProxy = Get-OptionalVar -Vars $vars -Key "NO_PROXY" -DefaultValue "localhost,127.0.0.1,::1"
$bootstrapUser = Get-OptionalVar -Vars $vars -Key "BOOTSTRAP_USER" -DefaultValue "ubuntu"
$dockerVersion = Get-OptionalVar -Vars $vars -Key "DOCKER_VERSION" -DefaultValue "latest"
$sslInspectionCaCertPath = Get-OptionalVar -Vars $vars -Key "SSL_INSPECTION_CA_CERT_PATH" -DefaultValue ""
$effectiveRootPassword = if ($PSBoundParameters.ContainsKey("RootPassword")) {
    $RootPassword
}
else {
    Get-OptionalVar -Vars $vars -Key "ROOT_PASSWORD" -DefaultValue ""
}

Validate-Scalar -Key "PROXY_HTTP" -Value $proxyHttp
Validate-Scalar -Key "PROXY_HTTPS" -Value $proxyHttps
Validate-Scalar -Key "NO_PROXY" -Value $noProxy
Validate-Scalar -Key "BOOTSTRAP_USER" -Value $bootstrapUser
Validate-Scalar -Key "DOCKER_VERSION" -Value $dockerVersion
Validate-Scalar -Key "SSL_INSPECTION_CA_CERT_PATH" -Value $sslInspectionCaCertPath
Validate-Scalar -Key "ROOT_PASSWORD" -Value $effectiveRootPassword

$isCustomCaConfigured = -not [string]::IsNullOrWhiteSpace($sslInspectionCaCertPath)
$caCertsBlock = Build-CaCertsBlock -CaCertPath $sslInspectionCaCertPath -VarsFilePath $VarsFile
$sslInspectionCaConfigured = if ($isCustomCaConfigured) { "true" } else { "false" }

$template = Get-Content -LiteralPath $templatePath -Raw
$rendered = $template

$replacements = @{
    "__PROXY_HTTP__" = $proxyHttp
    "__PROXY_HTTPS__" = $proxyHttps
    "__NO_PROXY__" = $noProxy
    "__BOOTSTRAP_USER__" = $bootstrapUser
    "__DOCKER_VERSION__" = $dockerVersion
    "__SSL_INSPECTION_CA_CONFIGURED__" = $sslInspectionCaConfigured
    "__CA_CERTS_BLOCK__" = $caCertsBlock
    "__ROOT_PASSWORD__" = $effectiveRootPassword
}

foreach ($placeholder in $replacements.Keys) {
    $rendered = $rendered.Replace($placeholder, [string]$replacements[$placeholder])
}

if ($rendered -match "__[A-Z0-9_]+__") {
    throw "Rendering failed: unresolved template placeholders remain in output"
}

$outputPath = [System.IO.Path]::GetFullPath($Output)
$outputDir = Split-Path -Parent $outputPath
if (-not [string]::IsNullOrWhiteSpace($outputDir) -and -not (Test-Path -LiteralPath $outputDir -PathType Container)) {
    New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
}

$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($outputPath, $rendered, $utf8NoBom)

Write-Host "Rendered cloud-init user-data: $outputPath"
Write-Host "  bootstrap user: $bootstrapUser"
if ($dockerVersion.ToLowerInvariant() -eq "latest") {
    Write-Host "  docker version policy: latest"
}
else {
    Write-Host "  docker minimum version: $dockerVersion"
}
if (-not $isCustomCaConfigured) {
    Write-Host "  custom CA: disabled"
}
else {
    Write-Host "  custom CA: enabled"
}
