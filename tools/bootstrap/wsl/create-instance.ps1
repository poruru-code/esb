# Where: tools/bootstrap/wsl/create-instance.ps1
# What: Creates a new WSL distro instance and applies cloud-init seed.
# Why: Automate new-instance bootstrap flow on Windows hosts.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$InstanceName,

    [Parameter(Mandatory = $true)]
    [string]$VarsFile,

    [string]$BaseDistro = "Ubuntu-24.04",

    [string]$InstallRoot = "$env:LOCALAPPDATA\WSL\Instances",

    [string]$UserDataPath,

    [string]$BootstrapUser = "ubuntu",

    [switch]$Force,

    [switch]$SkipPreflight,

    [switch]$RunSmokeTest
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
}

function Resolve-CertPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$VarsFilePath,

        [Parameter(Mandatory = $true)]
        [string]$InputPath
    )

    if ([System.IO.Path]::IsPathRooted($InputPath)) {
        return [System.IO.Path]::GetFullPath($InputPath)
    }

    $varsDir = Split-Path -Parent $VarsFilePath
    return [System.IO.Path]::GetFullPath((Join-Path $varsDir $InputPath))
}

function Get-WslDistroNames {
    $distroResult = Invoke-BootstrapNative -Command @("wsl", "--list", "--quiet") -Context "wsl list distros" -CaptureOutput
    return @($distroResult.OutputLines | ForEach-Object { $_.Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}

function Ensure-WslTargetRecreated {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstanceName,
        [Parameter(Mandatory = $true)]
        [string]$InstallRoot,
        [Parameter(Mandatory = $true)]
        [bool]$Force
    )

    $distros = Get-WslDistroNames
    $targetExists = $distros -contains $InstanceName
    $installDir = Join-Path $InstallRoot $InstanceName

    if (-not (Test-Path -LiteralPath $InstallRoot -PathType Container)) {
        New-Item -ItemType Directory -Path $InstallRoot -Force | Out-Null
    }
    $installDirExists = Test-Path -LiteralPath $installDir

    if (($targetExists -or $installDirExists) -and (-not $Force)) {
        $targets = @()
        if ($targetExists) {
            $targets += "WSL distro '$InstanceName'"
        }
        if ($installDirExists) {
            $targets += "install directory '$installDir'"
        }
        Confirm-RecreateIfNeeded -Message ("Existing target found: {0}. Delete and recreate?" -f ($targets -join ", "))
    }

    if ($targetExists) {
        Write-Host "[wsl create] Recreating existing distro '$InstanceName'"
        & wsl --terminate $InstanceName | Out-Null
        & wsl --unregister $InstanceName | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to unregister existing distro: $InstanceName"
        }
    }

    if ($installDirExists -and (Test-Path -LiteralPath $installDir)) {
        Remove-Item -LiteralPath $installDir -Recurse -Force
    }

    return $installDir
}

function Install-WslBaseDistro {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstanceName,
        [Parameter(Mandatory = $true)]
        [string]$BaseDistro,
        [Parameter(Mandatory = $true)]
        [string]$InstallDir
    )

    Write-Host "[wsl create] Installing fresh distro '$InstanceName' from '$BaseDistro'"
    Invoke-BootstrapNative -Command @("wsl", "--shutdown") -Context "wsl shutdown" | Out-Null
    Invoke-BootstrapNative -Command @("wsl", "--install", "--distribution", $BaseDistro, "--name", $InstanceName, "--location", $InstallDir, "--no-launch") -Context "wsl install" | Out-Null

    $distros = Get-WslDistroNames
    if (-not ($distros -contains $InstanceName)) {
        throw "WSL install completed but '$InstanceName' is not visible yet. Reboot Windows and re-run this command."
    }

    $enableSystemd = @"
set -euo pipefail
cat >/etc/wsl.conf <<'EOF_WSLCONF'
[boot]
systemd=true
EOF_WSLCONF
"@
    Invoke-BootstrapNative -Command @("wsl", "-d", $InstanceName, "--user", "root", "--", "bash", "-lc", $enableSystemd) -Context "wsl systemd pre-enable" | Out-Null
    Invoke-BootstrapNative -Command @("wsl", "--terminate", $InstanceName) -Context "wsl terminate after systemd pre-enable" | Out-Null
}

function Convert-CertificateFileToPem {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $rawText = [System.IO.File]::ReadAllText($Path)
    if ($rawText -match "-----BEGIN CERTIFICATE-----") {
        return $rawText.Trim()
    }

    $parsed = $false
    $cert = $null
    $certBytes = [System.IO.File]::ReadAllBytes($Path)

    try {
        $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 -ArgumentList (, $certBytes)
        $parsed = $true
    }
    catch {
        $normalizedBase64 = ($rawText -replace "\s", "")
        if (-not [string]::IsNullOrWhiteSpace($normalizedBase64)) {
            try {
                $decoded = [Convert]::FromBase64String($normalizedBase64)
                $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 -ArgumentList (, $decoded)
                $parsed = $true
            }
            catch {
                $parsed = $false
            }
        }
    }

    if (-not $parsed -or $null -eq $cert) {
        throw "PROXY_CA_CERT_PATH must point to a valid certificate file (.cer/.crt/.pem): $Path"
    }

    $der = $cert.Export([System.Security.Cryptography.X509Certificates.X509ContentType]::Cert)
    $base64 = [System.Convert]::ToBase64String($der)
    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.Add("-----BEGIN CERTIFICATE-----")
    for ($offset = 0; $offset -lt $base64.Length; $offset += 64) {
        $length = [System.Math]::Min(64, $base64.Length - $offset)
        $lines.Add($base64.Substring($offset, $length))
    }
    $lines.Add("-----END CERTIFICATE-----")
    return ($lines -join "`n")
}

$wslCmd = Get-Command wsl -ErrorAction SilentlyContinue
if ($null -eq $wslCmd) {
    throw "wsl command not found"
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$commonPath = Join-Path $scriptDir "..\core\bootstrap-common.psm1"
$commonPath = [System.IO.Path]::GetFullPath($commonPath)
if (-not (Test-Path -LiteralPath $commonPath -PathType Leaf)) {
    throw "Common helper script not found: $commonPath"
}
Import-Module -Name $commonPath -Force

$preflightPath = Join-Path $scriptDir "preflight.ps1"
$renderModulePath = Join-Path $scriptDir "..\core\render-user-data.psm1"
$validatePath = Join-Path $scriptDir "validate-instance.ps1"

if (-not (Test-Path -LiteralPath $renderModulePath -PathType Leaf)) {
    throw "Render module not found: $renderModulePath"
}
Import-Module -Name $renderModulePath -Force
# Ensure shared helper functions remain available in this script scope.
Import-Module -Name $commonPath -Force

if (-not $SkipPreflight) {
    if (-not (Test-Path -LiteralPath $preflightPath -PathType Leaf)) {
        throw "Preflight script not found: $preflightPath"
    }
    & $preflightPath
}

$varsFilePath = Resolve-ExistingFilePath -InputPath $VarsFile -ScriptDir $scriptDir
if (-not (Test-Path -LiteralPath $varsFilePath -PathType Leaf)) {
    throw "Vars file not found: $varsFilePath"
}

$vars = Read-VarsFile -Path $varsFilePath
Assert-AllowedVarKeys -Vars $vars -AllowedKeys @(
    "BOOTSTRAP_USER",
    "DOCKER_VERSION",
    "PROXY_HTTP",
    "PROXY_HTTPS",
    "NO_PROXY",
    "PROXY_CA_CERT_PATH"
) -VarsFilePath $varsFilePath

$proxyHttp = Get-OptionalVar -Vars $vars -Key "PROXY_HTTP" -DefaultValue ""
$proxyHttps = Get-OptionalVar -Vars $vars -Key "PROXY_HTTPS" -DefaultValue ""
$noProxy = Get-OptionalVar -Vars $vars -Key "NO_PROXY" -DefaultValue "localhost,127.0.0.1,::1"
$bootstrapUserFromVars = Get-OptionalVar -Vars $vars -Key "BOOTSTRAP_USER" -DefaultValue $BootstrapUser
$caCertPathInput = Get-OptionalVar -Vars $vars -Key "PROXY_CA_CERT_PATH" -DefaultValue ""

Validate-Scalar -Key "PROXY_HTTP" -Value $proxyHttp
Validate-Scalar -Key "PROXY_HTTPS" -Value $proxyHttps
Validate-Scalar -Key "NO_PROXY" -Value $noProxy
Validate-Scalar -Key "BOOTSTRAP_USER" -Value $bootstrapUserFromVars
Validate-Scalar -Key "PROXY_CA_CERT_PATH" -Value $caCertPathInput

if ([string]::IsNullOrWhiteSpace($noProxy)) {
    $noProxy = "localhost,127.0.0.1,::1"
}
if ([string]::IsNullOrWhiteSpace($bootstrapUserFromVars)) {
    $bootstrapUserFromVars = "ubuntu"
}
$rootPassword = New-RandomPassword -Length 8
$bootstrapUserPassword = New-RandomPassword -Length 8

if ([string]::IsNullOrWhiteSpace($UserDataPath)) {
    $UserDataPath = Join-Path $env:TEMP "$InstanceName-user-data.yaml"
}
$userDataPathFull = [System.IO.Path]::GetFullPath($UserDataPath)

Invoke-RenderUserData `
    -VarsFile $varsFilePath `
    -Output $userDataPathFull `
    -RootPassword $rootPassword `
    -BootstrapUserPassword $bootstrapUserPassword `
    -EnableCloudInitCaCerts:$false

$caPemTempPath = $null
if (-not [string]::IsNullOrWhiteSpace($caCertPathInput)) {
    $resolvedCaPath = Resolve-CertPath -VarsFilePath $varsFilePath -InputPath $caCertPathInput
    if (-not (Test-Path -LiteralPath $resolvedCaPath -PathType Leaf)) {
        throw "PROXY_CA_CERT_PATH not found: $resolvedCaPath"
    }

    $caPem = Convert-CertificateFileToPem -Path $resolvedCaPath
    $caPemTempPath = Join-Path $env:TEMP ("wsl-bootstrap-ca-{0}.crt" -f [Guid]::NewGuid().ToString("N"))
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($caPemTempPath, ($caPem + "`n"), $utf8NoBom)
}

$installDir = Ensure-WslTargetRecreated -InstanceName $InstanceName -InstallRoot $InstallRoot -Force:$Force

$tempBootstrapScript = Join-Path $env:TEMP ("wsl-bootstrap-{0}.sh" -f [guid]::NewGuid().ToString("N"))

try {
    Install-WslBaseDistro -InstanceName $InstanceName -BaseDistro $BaseDistro -InstallDir $installDir

    $proxyHttpLiteral = Convert-ToBashSingleQuotedLiteral -Value $proxyHttp
    $proxyHttpsLiteral = Convert-ToBashSingleQuotedLiteral -Value $proxyHttps
    $noProxyLiteral = Convert-ToBashSingleQuotedLiteral -Value $noProxy
    $bootstrapUserLiteral = Convert-ToBashSingleQuotedLiteral -Value $bootstrapUserFromVars
    $userDataWslPath = Convert-WindowsPathToWsl -Path $userDataPathFull
    $userDataWslLiteral = Convert-ToBashSingleQuotedLiteral -Value $userDataWslPath
    $caSourceLiteral = "''"
    if (-not [string]::IsNullOrWhiteSpace($caPemTempPath)) {
        $caSourceLiteral = Convert-ToBashSingleQuotedLiteral -Value (Convert-WindowsPathToWsl -Path $caPemTempPath)
    }

    $bootstrapScript = @'
set -euo pipefail
PROXY_HTTP=__PROXY_HTTP__
PROXY_HTTPS=__PROXY_HTTPS__
NO_PROXY=__NO_PROXY__
BOOTSTRAP_USER=__BOOTSTRAP_USER__
CA_CERT_SOURCE=__CA_CERT_SOURCE__

if [[ -n "${PROXY_HTTP}" || -n "${PROXY_HTTPS}" ]]; then
  {
    [[ -n "${PROXY_HTTP}" ]] && printf 'Acquire::http::Proxy "%s";\n' "${PROXY_HTTP}"
    [[ -n "${PROXY_HTTPS}" ]] && printf 'Acquire::https::Proxy "%s";\n' "${PROXY_HTTPS}"
  } > /etc/apt/apt.conf.d/01proxy

  [[ -n "${PROXY_HTTP}" ]] && export http_proxy="${PROXY_HTTP}" HTTP_PROXY="${PROXY_HTTP}"
  [[ -n "${PROXY_HTTPS}" ]] && export https_proxy="${PROXY_HTTPS}" HTTPS_PROXY="${PROXY_HTTPS}"
  export no_proxy="${NO_PROXY}" NO_PROXY="${NO_PROXY}"
else
  rm -f /etc/apt/apt.conf.d/01proxy
fi

if [[ -n "${CA_CERT_SOURCE}" ]]; then
  install -d -m 0755 /usr/local/share/ca-certificates
  cp "${CA_CERT_SOURCE}" /usr/local/share/ca-certificates/bootstrap-custom-ca.crt
  chmod 0644 /usr/local/share/ca-certificates/bootstrap-custom-ca.crt
  update-ca-certificates
fi

apt-get update
apt-get install -y cloud-init
cat >/etc/cloud/cloud.cfg.d/90-datasource.cfg <<'EOF_DS'
datasource_list: [ NoCloud, None ]
EOF_DS
cat >/etc/cloud/cloud.cfg.d/99-warnings.cfg <<'EOF_WARN'
warnings:
  dsid_missing_source: off
EOF_WARN
touch /root/.cloud-warnings.skip
mkdir -p /var/lib/cloud/instance/warnings
touch /var/lib/cloud/instance/warnings/.skip
target_user="${BOOTSTRAP_USER:-ubuntu}"
if [[ -z "${target_user}" ]]; then
  target_user="ubuntu"
fi
if [[ "${target_user}" != "root" ]] && ! id "${target_user}" >/dev/null 2>&1; then
  useradd_opts=(-m -s /bin/bash)
  if ! getent passwd 1000 >/dev/null 2>&1; then
    useradd_opts=(-u 1000 "${useradd_opts[@]}")
  fi
  useradd "${useradd_opts[@]}" "${target_user}"
  usermod -aG adm,cdrom,sudo,dip,plugdev "${target_user}" || true
fi
cat >/etc/wsl.conf <<EOF_WSLCONF
[boot]
systemd=true

[user]
default=${target_user}
EOF_WSLCONF
mkdir -p /var/lib/cloud/seed/nocloud
cp __USER_DATA_WSL__ /var/lib/cloud/seed/nocloud/user-data
cat >/var/lib/cloud/seed/nocloud/meta-data <<'EOF_META'
instance-id: __INSTANCE_NAME__
local-hostname: __INSTANCE_NAME__
EOF_META
cloud-init clean
cloud-init init --local
cloud-init init
cloud-init modules --mode=config
cloud-init modules --mode=final
'@
    $bootstrapScript = $bootstrapScript.
        Replace("__PROXY_HTTP__", $proxyHttpLiteral).
        Replace("__PROXY_HTTPS__", $proxyHttpsLiteral).
        Replace("__NO_PROXY__", $noProxyLiteral).
        Replace("__BOOTSTRAP_USER__", $bootstrapUserLiteral).
        Replace("__CA_CERT_SOURCE__", $caSourceLiteral).
        Replace("__USER_DATA_WSL__", $userDataWslLiteral).
        Replace("__INSTANCE_NAME__", $InstanceName)

    $bootstrapScriptLf = $bootstrapScript -replace "`r`n", "`n"
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($tempBootstrapScript, $bootstrapScriptLf, $utf8NoBom)
    $bootstrapScriptWslPath = Convert-WindowsPathToWsl -Path $tempBootstrapScript

    Write-Host "[wsl create] Applying cloud-init in '$InstanceName'"
    Invoke-BootstrapNative -Command @("wsl", "-d", $InstanceName, "--user", "root", "--", "bash", $bootstrapScriptWslPath) -Context "wsl apply cloud-init bootstrap" | Out-Null

    Write-Host "[wsl create] Completed: $InstanceName"
    if ($RunSmokeTest) {
        if (-not (Test-Path -LiteralPath $validatePath -PathType Leaf)) {
            throw "validate-instance.ps1 not found: $validatePath"
        }
        & $validatePath -InstanceName $InstanceName -BootstrapUser $bootstrapUserFromVars
    }
    else {
        Write-Host "[wsl create] Smoke test example:"
        Write-Host ".\tools\bootstrap\wsl\validate-instance.ps1 -InstanceName $InstanceName -BootstrapUser $bootstrapUserFromVars"
    }
    Write-NoticeBox -Title "INITIAL CREDENTIALS" -Lines @(
        "Instance: $InstanceName"
        "[root]"
        "Initial password: $rootPassword"
        "Reset command:"
        "  wsl -d $InstanceName --user root -- passwd root"
        ""
        "[login user: $bootstrapUserFromVars]"
        "Initial password: $bootstrapUserPassword"
        "Reset command:"
        "  wsl -d $InstanceName --user root -- passwd $bootstrapUserFromVars"
    )
    Write-Host "[wsl create] Launch command:"
    Write-Host "wsl.exe -d $InstanceName"
}
finally {
    if (Test-Path -LiteralPath $tempBootstrapScript) {
        Remove-Item -LiteralPath $tempBootstrapScript -Force
    }
    if (-not [string]::IsNullOrWhiteSpace($caPemTempPath) -and (Test-Path -LiteralPath $caPemTempPath)) {
        Remove-Item -LiteralPath $caPemTempPath -Force
    }
}
