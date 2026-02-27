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

function Parse-BoolSetting {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [AllowEmptyString()]
        [string]$Value,
        [bool]$DefaultValue
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return $DefaultValue
    }

    switch ($Value.Trim().ToLowerInvariant()) {
        "1" { return $true }
        "true" { return $true }
        "yes" { return $true }
        "on" { return $true }
        "0" { return $false }
        "false" { return $false }
        "no" { return $false }
        "off" { return $false }
        default { throw "$Name must be a boolean value (true/false): $Value" }
    }
}

function Parse-TcpPortList {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [AllowEmptyString()]
        [string]$Value
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return @()
    }

    $tokens = $Value -split '[,\s;/]+' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    $result = New-Object System.Collections.Generic.List[int]
    $seen = @{}
    foreach ($token in $tokens) {
        $port = 0
        if (-not [int]::TryParse($token, [ref]$port)) {
            throw "$Name contains a non-numeric TCP port: $token"
        }
        if ($port -lt 1 -or $port -gt 65535) {
            throw "$Name contains an out-of-range TCP port (1-65535): $port"
        }
        if (-not $seen.ContainsKey($port)) {
            $seen[$port] = $true
            $result.Add($port) | Out-Null
        }
    }

    return @($result.ToArray())
}

function Invoke-MultipassSudoBash {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstanceName,
        [Parameter(Mandatory = $true)]
        [string]$Script,
        [Parameter(Mandatory = $true)]
        [string]$Context
    )

    $outputLines = @(& multipass exec $InstanceName -- sudo bash -lc $Script 2>&1 | ForEach-Object { [string]$_ })
    $exitCode = $LASTEXITCODE

    foreach ($line in $outputLines) {
        if (-not [string]::IsNullOrWhiteSpace($line)) {
            Write-Host $line
        }
    }

    if ($exitCode -ne 0) {
        $details = ($outputLines -join "`n").Trim()
        if ([string]::IsNullOrWhiteSpace($details)) {
            throw "$Context failed in '$InstanceName'"
        }
        throw "$Context failed in '$InstanceName':`n$details"
    }
}

function Configure-HyperVAccess {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstanceName,
        [Parameter(Mandatory = $true)]
        [string]$LoginUser,
        [Parameter(Mandatory = $true)]
        [bool]$EnableSshPasswordAuth,
        [Parameter(Mandatory = $true)]
        [int[]]$OpenTcpPorts
    )

    $sshPasswordStatus = if ($EnableSshPasswordAuth) { "enabled" } else { "disabled" }
    $passwordAuthenticationValue = if ($EnableSshPasswordAuth) { "yes" } else { "no" }

    Write-Host "[hyper-v create] Configuring SSH for login user '$LoginUser' (password auth: $sshPasswordStatus)"
    Invoke-MultipassSudoBash -InstanceName $InstanceName -Context "SSH package installation" -Script @'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y openssh-server
'@

    $loginUserLiteral = Convert-ToBashSingleQuotedLiteral -Value $LoginUser
    $sshScript = @'
set -euo pipefail
login_user=__LOGIN_USER__
install -d -m 0755 /etc/ssh/sshd_config.d
cat > /etc/ssh/sshd_config.d/00-bootstrap-password-auth.conf <<'EOF_SSHD'
PasswordAuthentication __PASSWORD_AUTH_RAW__
KbdInteractiveAuthentication no
PermitRootLogin no
UsePAM yes
EOF_SSHD

# OpenSSH uses the first value encountered; 00-* must come before 50-cloud-init.conf.
rm -f /etc/ssh/sshd_config.d/99-bootstrap-password-auth.conf

if ! id "${login_user}" >/dev/null 2>&1; then
  echo "login user does not exist: ${login_user}" >&2
  exit 1
fi
if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
  systemctl enable ssh >/dev/null || true
  systemctl restart ssh || systemctl start ssh
fi
'@
    $sshScript = $sshScript.
        Replace("__LOGIN_USER__", $loginUserLiteral).
        Replace("__PASSWORD_AUTH_RAW__", $passwordAuthenticationValue)
    Invoke-MultipassSudoBash -InstanceName $InstanceName -Context "SSH password authentication setup" -Script $sshScript

    if ($OpenTcpPorts.Count -gt 0) {
        Write-Host "[hyper-v create] Configuring UFW open TCP ports: $($OpenTcpPorts -join ', ')"
        $portListLiteral = Convert-ToBashSingleQuotedLiteral -Value ($OpenTcpPorts -join " ")
        $firewallScript = @'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
open_ports=__OPEN_PORTS__
apt-get update
apt-get install -y ufw
ufw --force disable || true
ufw --force reset
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp comment 'bootstrap-ssh'
for port in ${open_ports}; do
  ufw allow "${port}/tcp" comment 'bootstrap-open-port'
done
ufw --force enable
ufw status verbose
'@
        $firewallScript = $firewallScript.Replace("__OPEN_PORTS__", $portListLiteral)
        Invoke-MultipassSudoBash -InstanceName $InstanceName -Context "UFW TCP port setup" -Script $firewallScript
    }
    else {
        Write-Host "[hyper-v create] ALLOW_INBOUND_TCP_PORTS is empty; UFW port setup skipped."
    }
}

function Get-MultipassNetworkNames {
    param(
        [switch]$SwitchOnly
    )

    $lines = @(& multipass networks --format json 2>&1 | ForEach-Object { [string]$_ })
    if ($LASTEXITCODE -ne 0) {
        return @()
    }

    $jsonText = ($lines -join "`n").Trim()
    if ([string]::IsNullOrWhiteSpace($jsonText)) {
        return @()
    }

    $parsed = $null
    try {
        $parsed = $jsonText | ConvertFrom-Json -Depth 5
    }
    catch {
        return @()
    }

    $entries = @()
    if ($null -ne $parsed -and $parsed.PSObject.Properties.Name -contains "list") {
        $entries = @($parsed.list)
    }
    elseif ($null -ne $parsed) {
        $entries = @($parsed)
    }

    if ($SwitchOnly) {
        $entries = @($entries | Where-Object { [string]$_.type -eq "switch" })
    }

    return @(
        $entries |
            Where-Object { $null -ne $_ -and -not [string]::IsNullOrWhiteSpace([string]$_.name) } |
            ForEach-Object { [string]$_.name } |
            Sort-Object -Unique
    )
}

function Assert-ValidNetworkHub {
    param(
        [AllowEmptyString()]
        [string]$NetworkHub
    )

    if ([string]::IsNullOrWhiteSpace($NetworkHub)) {
        return
    }

    $availableNetworks = Get-MultipassNetworkNames -SwitchOnly
    if ($availableNetworks.Count -eq 0) {
        $availableNetworks = Get-MultipassNetworkNames
    }
    if ($availableNetworks.Count -eq 0) {
        Write-Host "[hyper-v create] WARN: Could not enumerate Multipass networks; skipped upfront NetworkHub validation."
        return
    }

    if ($availableNetworks -contains $NetworkHub) {
        return
    }

    $networkList = (($availableNetworks | ForEach-Object { "  - $_" }) -join "`n")
    throw "NetworkHub '$NetworkHub' was not found.`nAvailable Multipass networks:`n$networkList`nCheck with: multipass networks"
}

function Get-MultipassPrimaryIpv4 {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstanceName
    )

    $jsonLines = @(& multipass info $InstanceName --format json 2>&1 | ForEach-Object { [string]$_ })
    if ($LASTEXITCODE -eq 0) {
        $jsonText = ($jsonLines -join "`n").Trim()
        if (-not [string]::IsNullOrWhiteSpace($jsonText)) {
            try {
                $parsed = $jsonText | ConvertFrom-Json -Depth 10
                $entry = $null
                if ($null -ne $parsed -and $parsed.PSObject.Properties.Name -contains "info") {
                    $info = $parsed.info
                    if ($null -ne $info -and $info.PSObject.Properties.Name -contains $InstanceName) {
                        $entry = $info.$InstanceName
                    }
                }
                if ($null -ne $entry -and $entry.PSObject.Properties.Name -contains "ipv4") {
                    foreach ($candidate in @($entry.ipv4)) {
                        $ip = [string]$candidate
                        if ($ip -match '^\d{1,3}(\.\d{1,3}){3}$') {
                            return $ip
                        }
                    }
                }
            }
            catch {
                # Fall through to plain-text parsing.
            }
        }
    }

    $plainLines = @(& multipass info $InstanceName 2>&1 | ForEach-Object { [string]$_ })
    if ($LASTEXITCODE -ne 0) {
        return ""
    }

    foreach ($line in $plainLines) {
        if ($line -match '^\s*IPv4\s*:\s*(.+)$') {
            $raw = $Matches[1].Trim()
            $tokens = $raw -split '[,\s]+' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
            foreach ($token in $tokens) {
                if ($token -match '^\d{1,3}(\.\d{1,3}){3}$') {
                    return $token
                }
            }
        }
    }

    return ""
}

function Resolve-EffectiveBootstrapUser {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$Vars,
        [Parameter(Mandatory = $true)]
        [string]$BootstrapUser,
        [Parameter(Mandatory = $true)]
        [bool]$BootstrapUserWasBound
    )

    $bootstrapUserFromVars = Get-OptionalVar -Vars $Vars -Key "BOOTSTRAP_USER" -DefaultValue "ubuntu"
    if ($BootstrapUserWasBound) {
        return $BootstrapUser
    }
    if (-not [string]::IsNullOrWhiteSpace($bootstrapUserFromVars)) {
        return $bootstrapUserFromVars
    }
    return "ubuntu"
}

function Resolve-HyperVRuntimeSettings {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$Vars,
        [Parameter(Mandatory = $true)]
        [int]$Cpus,
        [Parameter(Mandatory = $true)]
        [string]$Memory,
        [Parameter(Mandatory = $true)]
        [string]$Disk,
        [AllowEmptyString()]
        [string]$NetworkHub,
        [Parameter(Mandatory = $true)]
        [bool]$CpusWasBound,
        [Parameter(Mandatory = $true)]
        [bool]$MemoryWasBound,
        [Parameter(Mandatory = $true)]
        [bool]$DiskWasBound,
        [Parameter(Mandatory = $true)]
        [bool]$NetworkHubWasBound
    )

    $resolvedCpus = $Cpus
    $resolvedMemory = $Memory
    $resolvedDisk = $Disk
    $resolvedNetworkHub = $NetworkHub

    $cpusFromConfig = Resolve-SettingValue -Vars $Vars -VarsKey "VM_CPUS"
    $memoryFromConfig = Resolve-SettingValue -Vars $Vars -VarsKey "VM_MEMORY"
    $diskFromConfig = Resolve-SettingValue -Vars $Vars -VarsKey "VM_DISK"
    $networkHubFromConfig = Resolve-SettingValue -Vars $Vars -VarsKey "VM_NETWORK_HUB"
    $sshPasswordAuthFromConfig = Resolve-SettingValue -Vars $Vars -VarsKey "ENABLE_SSH_PASSWORD_AUTH"
    $openTcpPortsFromConfig = Resolve-SettingValue -Vars $Vars -VarsKey "ALLOW_INBOUND_TCP_PORTS"

    if (-not $CpusWasBound -and -not [string]::IsNullOrWhiteSpace($cpusFromConfig)) {
        $resolvedCpus = Parse-PositiveInt -Name "VM_CPUS" -Value $cpusFromConfig
    }
    if (-not $MemoryWasBound -and -not [string]::IsNullOrWhiteSpace($memoryFromConfig)) {
        Ensure-SingleLineValue -Name "VM_MEMORY" -Value $memoryFromConfig
        $resolvedMemory = $memoryFromConfig
    }
    if (-not $DiskWasBound -and -not [string]::IsNullOrWhiteSpace($diskFromConfig)) {
        Ensure-SingleLineValue -Name "VM_DISK" -Value $diskFromConfig
        $resolvedDisk = $diskFromConfig
    }
    if (-not $NetworkHubWasBound -and -not [string]::IsNullOrWhiteSpace($networkHubFromConfig)) {
        Ensure-SingleLineValue -Name "VM_NETWORK_HUB" -Value $networkHubFromConfig
        $resolvedNetworkHub = $networkHubFromConfig
    }

    if ($resolvedCpus -lt 1) {
        throw "Cpus must be a positive integer: $resolvedCpus"
    }
    Ensure-SingleLineValue -Name "Memory" -Value $resolvedMemory
    Ensure-SingleLineValue -Name "Disk" -Value $resolvedDisk
    if (-not [string]::IsNullOrWhiteSpace($resolvedNetworkHub)) {
        Ensure-SingleLineValue -Name "NetworkHub" -Value $resolvedNetworkHub
    }

    $enableSshPasswordAuth = Parse-BoolSetting -Name "ENABLE_SSH_PASSWORD_AUTH" -Value $sshPasswordAuthFromConfig -DefaultValue $true
    $openTcpPorts = Parse-TcpPortList -Name "ALLOW_INBOUND_TCP_PORTS" -Value $openTcpPortsFromConfig
    Assert-ValidNetworkHub -NetworkHub $resolvedNetworkHub

    return [pscustomobject]@{
        Cpus                  = $resolvedCpus
        Memory                = $resolvedMemory
        Disk                  = $resolvedDisk
        NetworkHub            = $resolvedNetworkHub
        EnableSshPasswordAuth = $enableSshPasswordAuth
        OpenTcpPorts          = @($openTcpPorts)
    }
}

function Remove-ExistingMultipassInstance {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstanceName,
        [Parameter(Mandatory = $true)]
        [bool]$Force
    )

    $infoResult = Invoke-BootstrapNative -Command @("multipass", "info", $InstanceName) -Context "multipass info" -CaptureOutput -IgnoreExitCode
    $instanceExists = ($infoResult.ExitCode -eq 0)
    if (-not $instanceExists) {
        return
    }

    if (-not $Force) {
        Confirm-RecreateIfNeeded -Message "Instance '$InstanceName' already exists. Delete and recreate?"
    }

    Write-Host "[hyper-v create] Existing instance '$InstanceName' found. Deleting to enforce fresh creation."
    Invoke-BootstrapNative -Command @("multipass", "delete", $InstanceName) -Context "multipass delete existing instance" | Out-Null
    Invoke-BootstrapNative -Command @("multipass", "purge") -Context "multipass purge deleted instances" | Out-Null
}

function Launch-MultipassInstance {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstanceName,
        [Parameter(Mandatory = $true)]
        [int]$Cpus,
        [Parameter(Mandatory = $true)]
        [string]$Memory,
        [Parameter(Mandatory = $true)]
        [string]$Disk,
        [Parameter(Mandatory = $true)]
        [string]$UserDataPath,
        [AllowEmptyString()]
        [string]$NetworkHub
    )

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

    $launchOutputLines = @(& multipass @launchArgs 2>&1 | ForEach-Object { [string]$_ })
    $launchExit = $LASTEXITCODE
    foreach ($line in $launchOutputLines) {
        if (-not [string]::IsNullOrWhiteSpace($line)) {
            Write-Host $line
        }
    }
    if ($launchExit -ne 0) {
        $launchDetails = ($launchOutputLines -join "`n").Trim()
        if ([string]::IsNullOrWhiteSpace($launchDetails)) {
            throw "multipass launch failed for '$InstanceName'"
        }
        throw "multipass launch failed for '$InstanceName':`n$launchDetails"
    }
}

function Wait-MultipassCloudInitDone {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstanceName
    )

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

if (-not (Get-Command multipass -ErrorAction SilentlyContinue)) {
    throw "multipass command not found"
}

if (-not (Test-Path -LiteralPath $renderModulePath -PathType Leaf)) {
    throw "Shared render module not found: $renderModulePath"
}
Import-Module -Name $renderModulePath -Force
# Ensure shared helper functions remain available in this script scope.
Import-Module -Name $commonPath -Force

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
Assert-AllowedVarKeys -Vars $vars -AllowedKeys @(
    "BOOTSTRAP_USER",
    "DOCKER_VERSION",
    "PROXY_HTTP",
    "PROXY_HTTPS",
    "NO_PROXY",
    "PROXY_CA_CERT_PATH",
    "VM_CPUS",
    "VM_MEMORY",
    "VM_DISK",
    "VM_NETWORK_HUB",
    "ENABLE_SSH_PASSWORD_AUTH",
    "ALLOW_INBOUND_TCP_PORTS"
) -VarsFilePath $varsFilePath

$effectiveBootstrapUser = Resolve-EffectiveBootstrapUser -Vars $vars -BootstrapUser $BootstrapUser -BootstrapUserWasBound:$($PSBoundParameters.ContainsKey("BootstrapUser"))

$runtimeSettings = Resolve-HyperVRuntimeSettings `
    -Vars $vars `
    -Cpus $Cpus `
    -Memory $Memory `
    -Disk $Disk `
    -NetworkHub $NetworkHub `
    -CpusWasBound:$($PSBoundParameters.ContainsKey("Cpus")) `
    -MemoryWasBound:$($PSBoundParameters.ContainsKey("Memory")) `
    -DiskWasBound:$($PSBoundParameters.ContainsKey("Disk")) `
    -NetworkHubWasBound:$($PSBoundParameters.ContainsKey("NetworkHub"))

$Cpus = [int]$runtimeSettings.Cpus
$Memory = [string]$runtimeSettings.Memory
$Disk = [string]$runtimeSettings.Disk
$NetworkHub = [string]$runtimeSettings.NetworkHub
$enableSshPasswordAuth = [bool]$runtimeSettings.EnableSshPasswordAuth
$openTcpPorts = @($runtimeSettings.OpenTcpPorts)

$rootPassword = New-RandomPassword -Length 8
$bootstrapUserPassword = New-RandomPassword -Length 8

Invoke-RenderUserData `
    -VarsFile $varsFilePath `
    -Output $UserDataPath `
    -RootPassword $rootPassword `
    -BootstrapUserPassword $bootstrapUserPassword

Remove-ExistingMultipassInstance -InstanceName $InstanceName -Force:$Force
Launch-MultipassInstance -InstanceName $InstanceName -Cpus $Cpus -Memory $Memory -Disk $Disk -UserDataPath $UserDataPath -NetworkHub $NetworkHub
Wait-MultipassCloudInitDone -InstanceName $InstanceName

Configure-HyperVAccess -InstanceName $InstanceName -LoginUser $effectiveBootstrapUser -EnableSshPasswordAuth:$enableSshPasswordAuth -OpenTcpPorts $openTcpPorts

$sshPasswordAuthDisplay = if ($enableSshPasswordAuth) { "enabled" } else { "disabled" }
if ($RunSmokeTest) {
    if (-not (Test-Path -LiteralPath $validatePath -PathType Leaf)) {
        throw "validate-instance.ps1 not found: $validatePath"
    }
    $expectedSshPasswordAuth = if ($enableSshPasswordAuth) { "enabled" } else { "disabled" }
    & $validatePath `
        -InstanceName $InstanceName `
        -BootstrapUser $effectiveBootstrapUser `
        -ExpectedSshPasswordAuth $expectedSshPasswordAuth `
        -ExpectedOpenTcpPorts $openTcpPorts
}
else {
    Write-Host "[hyper-v create] Smoke test example:"
    $validateExample = ".\tools\bootstrap\hyper-v\validate-instance.ps1 -InstanceName $InstanceName -BootstrapUser $effectiveBootstrapUser -ExpectedSshPasswordAuth $sshPasswordAuthDisplay"
    if ($openTcpPorts.Count -gt 0) {
        $validateExample += " -ExpectedOpenTcpPorts " + ($openTcpPorts -join ",")
    }
    Write-Host $validateExample
}

$instanceIpv4 = Get-MultipassPrimaryIpv4 -InstanceName $InstanceName
$instanceIpv4Display = if ([string]::IsNullOrWhiteSpace($instanceIpv4)) { "not available" } else { $instanceIpv4 }
$sshCommandDisplay = if ([string]::IsNullOrWhiteSpace($instanceIpv4)) { "ssh $effectiveBootstrapUser@<instance-ip>" } else { "ssh $effectiveBootstrapUser@$instanceIpv4" }
if ([string]::IsNullOrWhiteSpace($instanceIpv4)) {
    Write-Host "[hyper-v create] WARN: Could not determine instance IPv4. Check with: multipass info $InstanceName"
}
Write-Host "[hyper-v create] Completed: $InstanceName"
Write-NoticeBox -Title "INITIAL CREDENTIALS" -Lines @(
    "Instance: $InstanceName"
    "[root]"
    "Initial password: $rootPassword"
    "Reset command:"
    "  multipass exec $InstanceName -- sudo passwd root"
    ""
    "[login user: $effectiveBootstrapUser]"
    "Initial password: $bootstrapUserPassword"
    "Reset command:"
    "  multipass exec $InstanceName -- sudo passwd $effectiveBootstrapUser"
    ""
    "[ssh]"
    "Password authentication: $sshPasswordAuthDisplay"
    "IPv4: $instanceIpv4Display"
    "Connect command: $sshCommandDisplay"
)
