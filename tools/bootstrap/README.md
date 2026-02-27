<!--
Where: tools/bootstrap/README.md
What: Single integrated bootstrap guide for host provisioning.
Why: Make one canonical runbook for WSL2 and Hyper-V (Multipass) new-instance setup.
-->
# Bootstrap (Integrated Guide)

`tools/bootstrap/` は開発環境を毎回「新規作成」するための統合入口です。  
既存インスタンスの上書き更新や冪等運用は前提にせず、作り直しを基本とします。

## Scope

- OS: Ubuntu 24.04 LTS
- Configuration: single fixed configuration
- Platform A: WSL2（新規ディストリ作成まで自動化）
- Platform B: Hyper-V + Multipass（新規インスタンス作成）
- Validation: スモークテストのみ

## Common Policy

- Docker は公式 Ubuntu 手順準拠で導入
- cloud-init datasource は WSL では `NoCloud, None` を明示、Multipass では `NoCloud` seed を使用
- 初回 bootstrap 時に Ubuntu パッケージアップグレードを実施（`package_upgrade: true`）
- `DOCKER_VERSION` は既定 `latest`
- `DOCKER_VERSION` に具体値を設定した場合は minimum version として検証
- `containerd` / `buildx` / `compose` は個別固定しない（Docker Engine 導入結果に従う）
- Proxy 設定は任意
  - 設定時は APT に加えて `/etc/environment` と `/etc/profile.d/bootstrap-proxy.sh` に反映
- SSL inspection 用 CA 追加は任意
  - Hyper-V (Multipass): cloud-init `ca_certs` で投入
  - WSL2: cloud-init 導入前の pre-bootstrap で投入
- `mise` は bootstrap で自動導入
- `gh` (GitHub CLI) は apt で自動導入
- 作成ごとに root / ログインユーザー初期パスワードをランダム生成し、完了時に再設定コマンドと合わせて表示
- vars ファイルは未知キーを許可せず、typo を fail-fast で停止

## Variables

各プラットフォームの `vars.example` をコピーして使います。

```powershell
Copy-Item .\tools\bootstrap\wsl\vars.example "$env:USERPROFILE\bootstrap-wsl.vars"
Copy-Item .\tools\bootstrap\hyper-v\vars.example "$env:USERPROFILE\bootstrap-hyperv.vars"
notepad "$env:USERPROFILE\bootstrap-wsl.vars"
notepad "$env:USERPROFILE\bootstrap-hyperv.vars"
```

主要変数:

- `PROXY_HTTP` / `PROXY_HTTPS`: 任意
- `NO_PROXY`: 既定 `localhost,127.0.0.1,::1`
- `BOOTSTRAP_USER`: 既定 `ubuntu`（未存在時は cloud-init が作成）
- `DOCKER_VERSION`: 既定 `latest`（または minimum version 指定）
- `PROXY_CA_CERT_PATH`: 任意

### CA Certificate (Optional)

`PROXY_CA_CERT_PATH` は Windows パスを指定可能です。  
例:

```text
PROXY_CA_CERT_PATH=C:\certs\corp-root-ca.cer
```

- `.cer` (DER / Base64), `.crt`, `.pem` を受け付け
- renderer 側で PEM 化して投入
  - Hyper-V (Multipass): cloud-init `ca_certs` に埋め込み
  - WSL2: pre-bootstrap で `/usr/local/share/ca-certificates` へ配置
- 相対パス指定時は vars ファイルの配置ディレクトリ基準で解決

## Platform Flow: WSL2

`create-instance.ps1` 実行時に `preflight.ps1` を内部実行します。  
（必要時のみ `-SkipPreflight` でスキップ可能）
WSL は `wsl --install --name` を使って常に新規ディストリを作成します（既存 `Ubuntu` の export/import は使用しません）。
同名ディストリ/同名インストールディレクトリがある場合は削除確認を行い、`-Force` 指定時は無確認で再作成します。
WSL の初回起動前に `BOOTSTRAP_USER` を自動作成し default user を設定するため、初回の対話ユーザー作成プロンプトは発生しません。

### Create New Distro + Apply Cloud-init

```powershell
.\tools\bootstrap\wsl\create-instance.ps1 `
  -InstanceName bootstrap-wsl `
  -VarsFile "$env:USERPROFILE\bootstrap-wsl.vars" `
  -RunSmokeTest
```

既存同名ディストリを再作成する場合:

```powershell
.\tools\bootstrap\wsl\create-instance.ps1 `
  -InstanceName bootstrap-wsl `
  -VarsFile "$env:USERPROFILE\bootstrap-wsl.vars" `
  -Force `
  -RunSmokeTest
```

## Platform Flow: Hyper-V (Multipass)

`create-instance.ps1` 実行時に `preflight.ps1` を内部実行します。  
（必要時のみ `-SkipPreflight` でスキップ可能）
同名インスタンスがある場合は削除確認を行い、`-Force` 指定時は無確認で削除して新規作成します。
`BOOTSTRAP_USER` が既存でない場合も cloud-init で自動作成し、`docker` グループへ付与します。

### Create New Instance + Apply Cloud-init

```powershell
.\tools\bootstrap\hyper-v\create-instance.ps1 `
  -InstanceName bootstrap-hv `
  -VarsFile "$env:USERPROFILE\bootstrap-hyperv.vars" `
  -RunSmokeTest
```

Hyper-V のリソース/ネットワークを vars ファイルで指定する場合（任意）:

- vars ファイルキー:
  - `VM_CPUS`
  - `VM_MEMORY`
  - `VM_DISK`
  - `VM_NETWORK_HUB` (空/`default`/`auto` は Multipass 既定ネットワーク)
  - `ENABLE_SSH_PASSWORD_AUTH`
  - `ALLOW_INBOUND_TCP_PORTS`

優先順位: `create-instance.ps1` 引数 > vars > 既定値

## Smoke Test Only

- WSL2:

```powershell
.\tools\bootstrap\wsl\validate-instance.ps1 -InstanceName bootstrap-wsl -BootstrapUser ubuntu
```

- Hyper-V / Multipass:

```powershell
.\tools\bootstrap\hyper-v\validate-instance.ps1 -InstanceName bootstrap-hv -BootstrapUser ubuntu
```

必要に応じて SSH/UFW の期待値検証も可能です:

```powershell
.\tools\bootstrap\hyper-v\validate-instance.ps1 -InstanceName bootstrap-hv -BootstrapUser ubuntu -ExpectedSshPasswordAuth enabled -ExpectedOpenTcpPorts 443,19000,9001,8001,9428
```

## Optional: Run Preflight Only

- WSL2:

```powershell
.\tools\bootstrap\wsl\preflight.ps1
```

- Hyper-V / Multipass:

```powershell
.\tools\bootstrap\hyper-v\preflight.ps1
```

## Implementation Map

- WSL2:
  - `tools/bootstrap/wsl/preflight.ps1`
  - `tools/bootstrap/wsl/create-instance.ps1`
  - `tools/bootstrap/wsl/validate-instance.ps1`
- Hyper-V / Multipass:
  - `tools/bootstrap/hyper-v/preflight.ps1`
  - `tools/bootstrap/hyper-v/create-instance.ps1`
  - `tools/bootstrap/hyper-v/validate-instance.ps1`
- Shared cloud-init:
  - `tools/bootstrap/cloud-init/user-data.template.yaml`
  - `tools/bootstrap/core/bootstrap-common.psm1`
  - `tools/bootstrap/core/render-user-data.psm1`
  - `tools/bootstrap/cloud-init/verify-instance.sh`
