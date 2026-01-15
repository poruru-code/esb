# Edge Serverless Box Bootstrap Guide

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
