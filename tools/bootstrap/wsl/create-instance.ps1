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

function Invoke-Native {
    param([Parameter(Mandatory = $true)][string[]]$Command)

    $exe = $Command[0]
    $args = @()
    if ($Command.Count -gt 1) {
        $args = $Command[1..($Command.Count - 1)]
    }

    & $exe @args
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed ($LASTEXITCODE): $($Command -join ' ')"
    }
}

function Convert-ToBashSingleQuotedLiteral {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value)
    $replacement = @'
'"'"'
'@
    $escaped = $Value.Replace("'", $replacement)
    return "'" + $escaped + "'"
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
        throw "SSL_INSPECTION_CA_CERT_PATH must point to a valid certificate file (.cer/.crt/.pem): $Path"
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
$preflightPath = Join-Path $scriptDir "preflight.ps1"
$rendererPath = Join-Path $scriptDir "..\core\render-user-data.ps1"
$validatePath = Join-Path $scriptDir "validate-instance.ps1"

if (-not (Test-Path -LiteralPath $rendererPath -PathType Leaf)) {
    throw "Renderer not found: $rendererPath"
}

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
$proxyHttp = Get-OptionalVar -Vars $vars -Key "PROXY_HTTP" -DefaultValue ""
$proxyHttps = Get-OptionalVar -Vars $vars -Key "PROXY_HTTPS" -DefaultValue ""
$noProxy = Get-OptionalVar -Vars $vars -Key "NO_PROXY" -DefaultValue "localhost,127.0.0.1,::1"
$bootstrapUserFromVars = Get-OptionalVar -Vars $vars -Key "BOOTSTRAP_USER" -DefaultValue $BootstrapUser
$caCertPathInput = Get-OptionalVar -Vars $vars -Key "SSL_INSPECTION_CA_CERT_PATH" -DefaultValue ""

Validate-Scalar -Key "PROXY_HTTP" -Value $proxyHttp
Validate-Scalar -Key "PROXY_HTTPS" -Value $proxyHttps
Validate-Scalar -Key "NO_PROXY" -Value $noProxy
Validate-Scalar -Key "BOOTSTRAP_USER" -Value $bootstrapUserFromVars
Validate-Scalar -Key "SSL_INSPECTION_CA_CERT_PATH" -Value $caCertPathInput

if ([string]::IsNullOrWhiteSpace($noProxy)) {
    $noProxy = "localhost,127.0.0.1,::1"
}
if ([string]::IsNullOrWhiteSpace($bootstrapUserFromVars)) {
    $bootstrapUserFromVars = "ubuntu"
}
$rootPassword = New-RandomPassword -Length 8

if ([string]::IsNullOrWhiteSpace($UserDataPath)) {
    $UserDataPath = Join-Path $env:TEMP "$InstanceName-user-data.yaml"
}
$userDataPathFull = [System.IO.Path]::GetFullPath($UserDataPath)

& $rendererPath -VarsFile $varsFilePath -Output $userDataPathFull -RootPassword $rootPassword

$caPemTempPath = $null
if (-not [string]::IsNullOrWhiteSpace($caCertPathInput)) {
    $resolvedCaPath = Resolve-CertPath -VarsFilePath $varsFilePath -InputPath $caCertPathInput
    if (-not (Test-Path -LiteralPath $resolvedCaPath -PathType Leaf)) {
        throw "SSL_INSPECTION_CA_CERT_PATH not found: $resolvedCaPath"
    }

    $caPem = Convert-CertificateFileToPem -Path $resolvedCaPath
    $caPemTempPath = Join-Path $env:TEMP ("wsl-bootstrap-ca-{0}.crt" -f [Guid]::NewGuid().ToString("N"))
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($caPemTempPath, ($caPem + "`n"), $utf8NoBom)
}

$distros = @(& wsl --list --quiet)
$distros = @($distros | ForEach-Object { $_.Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })

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
    & wsl --unregister $InstanceName
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to unregister existing distro: $InstanceName"
    }
}

if ($installDirExists -and (Test-Path -LiteralPath $installDir)) {
    Remove-Item -LiteralPath $installDir -Recurse -Force
}

$tempBootstrapScript = Join-Path $env:TEMP ("wsl-bootstrap-{0}.sh" -f [guid]::NewGuid().ToString("N"))

try {
    Write-Host "[wsl create] Installing fresh distro '$InstanceName' from '$BaseDistro'"
    Invoke-Native -Command @("wsl", "--shutdown")
    Invoke-Native -Command @("wsl", "--install", "--distribution", $BaseDistro, "--name", $InstanceName, "--location", $installDir, "--no-launch")

    $distros = @(& wsl --list --quiet)
    $distros = @($distros | ForEach-Object { $_.Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
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
    Invoke-Native -Command @("wsl", "-d", $InstanceName, "--user", "root", "--", "bash", "-lc", $enableSystemd)
    Invoke-Native -Command @("wsl", "--terminate", $InstanceName)

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

    Set-Content -LiteralPath $tempBootstrapScript -Value $bootstrapScript -NoNewline
    $bootstrapScriptWslPath = Convert-WindowsPathToWsl -Path $tempBootstrapScript

    Write-Host "[wsl create] Applying cloud-init in '$InstanceName'"
    Invoke-Native -Command @("wsl", "-d", $InstanceName, "--user", "root", "--", "bash", $bootstrapScriptWslPath)

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
    Write-NoticeBox -Title "ROOT ACCESS" -Lines @(
        "Instance: $InstanceName"
        "Initial root password: $rootPassword"
        "Reset command:"
        "  wsl -d $InstanceName --user root -- passwd root"
    )
}
finally {
    if (Test-Path -LiteralPath $tempBootstrapScript) {
        Remove-Item -LiteralPath $tempBootstrapScript -Force
    }
    if (-not [string]::IsNullOrWhiteSpace($caPemTempPath) -and (Test-Path -LiteralPath $caPemTempPath)) {
        Remove-Item -LiteralPath $caPemTempPath -Force
    }
}
