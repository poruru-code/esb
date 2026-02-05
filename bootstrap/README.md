<!--
Where: bootstrap/README.md
What: Bootstrap guide for WSL/Hyper-V environments.
Why: Document initial host setup steps for the system.
-->
# Bootstrap Guide

このガイドでは、WSL (Windows Subsystem for Linux) または Hyper-V 上の Ubuntu 環境に対して、Docker およびプロキシ設定を自動構成するための手順を説明します。

## 前提条件

*   **OS**: Ubuntu 22.04 LTS / 24.04 LTS (推奨)
*   **権限**: `sudo` 権限を持つユーザーであること
*   **ツール**: Python 3 がインストールされていること（Ansible の実行に必要）

## 1. Ansible のインストール

まだ Ansible がインストールされていない場合は、以下のコマンドでインストールしてください。

```bash
sudo apt update
sudo apt install -y software-properties-common
sudo add-apt-repository --yes --update ppa:ansible/ansible
sudo apt install -y ansible
```

### 1-a. プロキシ未設定 / 閉域環境でのインストール（代替案）

対象 PC から外部リポジトリに到達できない場合、以下のいずれかの方法で「ローカルインストール」してください。

**案 A: オフライン APT パッケージ（推奨・簡易）**

プロキシ設定済みの Ubuntu（WSL でも可）で必要な `.deb` を集め、対象 PC に持ち込みます。

```bash
# 取得側（プロキシ設定済み Ubuntu）
sudo apt update
sudo apt install -y apt-rdepends
mkdir -p /tmp/ansible_debs
cd /tmp/ansible_debs
apt download ansible
apt-rdepends ansible | awk '/^ /{print $1}' | xargs -r apt download
```

取得した `/tmp/ansible_debs` を USB 等で対象 PC にコピーし、以下で導入します。

```bash
# 対象 PC（オフライン）
cd /path/to/ansible_debs
sudo dpkg -i ./*.deb || sudo apt -f install -y
```

**案 B: ローカル APT リポジトリを用意**

同一ネットワーク内に apt-mirror / aptly などでローカルリポジトリを構築し、
対象 PC の `sources.list` をローカルに向けます。複数台導入や継続運用向きです。

**案 C: Python wheel でインストール（上級者向け）**

プロキシ設定済みの環境で `pip download ansible` し、wheel を持ち込んで
`pip install --no-index --find-links` で導入します。依存関係の管理が必要です。

> 注: 「Windows で apt を取得してローカルインストール」は Ubuntu での
> インストールには使えません。apt は Ubuntu/Debian 向けのパッケージマネージャです。
> Windows 側で準備する場合は WSL/Ubuntu VM で `.deb` を取得してください。

## 2. 設定ファイルの編集

`bootstrap` ディレクトリ内の `vars.yml` を編集し、環境に合わせた設定を行います。

```bash
cd bootstrap
nano vars.yml
```

**設定項目:**

*   **`proxy_http` / `proxy_https`**:
    *   プロキシ環境下の場合は、`http://proxy.example.com:8080` のように設定してください。
    *   プロキシを使用しない場合は、`""` (空文字) のままにしてください。
*   **`docker_users`**:
    *   Docker グループに追加するユーザーを指定します。デフォルトでは実行ユーザーが対象です。

## 3. Playbook の実行

### 基本的な実行方法

以下のコマンドを実行して、セットアップを開始します。`BECOME password` プロンプトが表示されたら、sudo パスワードを入力してください。

```bash
ansible-playbook -i inventory playbook.yml --ask-become-pass
```

## 4. 環境ごとの注意点

### WSL (Windows Subsystem for Linux) の場合

WSL 2 では、**Systemd** が有効になっていることが推奨されます（Docker デーモンの管理のため）。

1.  `/etc/wsl.conf` を確認（または作成）します：
    ```bash
    sudo nano /etc/wsl.conf
    ```
2.  以下の設定が含まれていることを確認します：
    ```ini
    [boot]
    systemd=true
    ```
3.  設定を変更した場合は、PowerShell で `wsl --shutdown` を実行し、WSL を再起動してください。

### Hyper-V (Ubuntu 仮想マシン) の場合

通常の Ubuntu Server/Desktop と同様の手順で実行可能です。
SSH 経由でセットアップを行う場合は、`inventory` ファイルを編集してリモートホストを指定することも可能です。

**例: リモートホストへの適用 (inventory ファイル)**
```ini
[targets]
192.168.1.100 ansible_user=ubuntu
```

実行コマンド:
```bash
ansible-playbook -i inventory playbook.yml --ask-become-pass
```

## 5. 動作確認

セットアップ完了後、一度ログアウトして再ログインする（または `newgrp docker` を実行する）ことで、`docker` コマンドが sudo なしで実行可能になります。

以下のコマンドで正常に動作することを確認してください：

```bash
docker run --rm hello-world
```

プロキシ環境下の場合、Docker イメージの Pull が成功すればプロキシ設定も正常に適用されています。

---

## Implementation references
- `bootstrap/playbook.yml`
- `bootstrap/vars.yml`
