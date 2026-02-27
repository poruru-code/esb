# Where: tools/bootstrap/core/render-user-data.psm1
# What: Cloud-init user-data rendering module.
# Why: Keep rendering logic reusable from WSL/Hyper-V entrypoint scripts.

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$moduleDir = Split-Path -Parent $PSCommandPath
$commonModulePath = Join-Path $moduleDir "bootstrap-common.psm1"
if (-not (Test-Path -LiteralPath $commonModulePath -PathType Leaf)) {
    throw "Common module not found: $commonModulePath"
}
Import-Module -Name $commonModulePath

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
        [bool]$EnableCloudInitCaCerts,

        [Parameter(Mandatory = $true)]
        [string]$VarsFilePath
    )

    if ([string]::IsNullOrWhiteSpace($CaCertPath)) {
        return "# no custom CA certificate provided"
    }
    if (-not $EnableCloudInitCaCerts) {
        return "# custom CA injection is handled outside cloud-init for this platform"
    }

    $resolvedPath = if ([System.IO.Path]::IsPathRooted($CaCertPath)) {
        [System.IO.Path]::GetFullPath($CaCertPath)
    }
    else {
        $varsDir = Split-Path -Parent ([System.IO.Path]::GetFullPath($VarsFilePath))
        [System.IO.Path]::GetFullPath((Join-Path $varsDir $CaCertPath))
    }

    if (-not (Test-Path -LiteralPath $resolvedPath -PathType Leaf)) {
        throw "PROXY_CA_CERT_PATH not found: $resolvedPath"
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
            throw "PROXY_CA_CERT_PATH must point to a valid certificate file (.cer/.crt/.pem): $resolvedPath"
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
        throw "PROXY_CA_CERT_PATH resolved to an empty certificate: $resolvedPath"
    }

    $indentedPem = (($pem -split "`r?`n") | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | ForEach-Object { "        $_" }) -join "`n"
    return "ca_certs:`n  remove_defaults: false`n  trusted:`n    - |`n$indentedPem"
}

function Invoke-RenderUserData {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]
        [string]$VarsFile,

        [Parameter(Mandatory = $true)]
        [string]$Output,

        [AllowEmptyString()]
        [string]$RootPassword = "",

        [AllowEmptyString()]
        [string]$BootstrapUserPassword = "",

        [bool]$EnableSshPasswordAuth = $true,

        [bool]$EnableCloudInitCaCerts = $true
    )

    $templatePath = Join-Path $moduleDir "..\cloud-init\user-data.template.yaml"
    if (-not (Test-Path -LiteralPath $templatePath -PathType Leaf)) {
        throw "Template not found: $templatePath"
    }

    if (-not (Test-Path -LiteralPath $VarsFile -PathType Leaf)) {
        throw "Vars file not found: $VarsFile"
    }

    $vars = Read-VarsFile -Path $VarsFile
    $proxyHttp = Get-OptionalVar -Vars $vars -Key "PROXY_HTTP" -DefaultValue ""
    $proxyHttps = Get-OptionalVar -Vars $vars -Key "PROXY_HTTPS" -DefaultValue ""
    $noProxy = Get-OptionalVar -Vars $vars -Key "NO_PROXY" -DefaultValue "localhost,127.0.0.1,::1"
    $bootstrapUser = Get-OptionalVar -Vars $vars -Key "BOOTSTRAP_USER" -DefaultValue "ubuntu"
    $dockerVersion = Get-OptionalVar -Vars $vars -Key "DOCKER_VERSION" -DefaultValue "latest"
    $proxyCaCertPath = Get-OptionalVar -Vars $vars -Key "PROXY_CA_CERT_PATH" -DefaultValue ""
    $effectiveRootPassword = $RootPassword
    $effectiveBootstrapUserPassword = $BootstrapUserPassword
    $effectiveSshPwauth = if ($EnableSshPasswordAuth) { "true" } else { "false" }

    Validate-Scalar -Key "PROXY_HTTP" -Value $proxyHttp
    Validate-Scalar -Key "PROXY_HTTPS" -Value $proxyHttps
    Validate-Scalar -Key "NO_PROXY" -Value $noProxy
    Validate-Scalar -Key "BOOTSTRAP_USER" -Value $bootstrapUser
    Validate-Scalar -Key "DOCKER_VERSION" -Value $dockerVersion
    Validate-Scalar -Key "PROXY_CA_CERT_PATH" -Value $proxyCaCertPath
    Validate-Scalar -Key "ROOT_PASSWORD" -Value $effectiveRootPassword
    Validate-Scalar -Key "BOOTSTRAP_USER_PASSWORD" -Value $effectiveBootstrapUserPassword

    $isCustomCaConfigured = -not [string]::IsNullOrWhiteSpace($proxyCaCertPath)
    $caCertsBlock = Build-CaCertsBlock -CaCertPath $proxyCaCertPath -EnableCloudInitCaCerts:$EnableCloudInitCaCerts -VarsFilePath $VarsFile
    $sslInspectionCaConfigured = if ($isCustomCaConfigured) { "true" } else { "false" }

    $template = Get-Content -LiteralPath $templatePath -Raw
    $rendered = $template

    $replacements = @{
        "__PROXY_HTTP__" = $proxyHttp
        "__PROXY_HTTPS__" = $proxyHttps
        "__NO_PROXY__" = $noProxy
        "__BOOTSTRAP_USER__" = $bootstrapUser
        "__DOCKER_VERSION__" = $dockerVersion
        "__SSH_PWAUTH__" = $effectiveSshPwauth
        "__SSL_INSPECTION_CA_CONFIGURED__" = $sslInspectionCaConfigured
        "__CA_CERTS_BLOCK__" = $caCertsBlock
        "__ROOT_PASSWORD__" = $effectiveRootPassword
        "__BOOTSTRAP_USER_PASSWORD__" = $effectiveBootstrapUserPassword
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
        if ($EnableCloudInitCaCerts) {
            Write-Host "  custom CA: enabled (cloud-init ca_certs)"
        }
        else {
            Write-Host "  custom CA: enabled (platform pre-bootstrap)"
        }
    }
}

Export-ModuleMember -Function @(
    "Invoke-RenderUserData"
)
