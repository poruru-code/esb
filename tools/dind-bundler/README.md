# DinD Bundler Tools

このディレクトリには、既存 artifact 出力を単一の Docker-in-Docker (DinD) コンテナにパッケージングするツールが含まれます。

## 概要

`build.sh` は **artifact 入力専用** です。入力として既存 artifact を受け取り DinD を構築します。
生成した DinD イメージは、別 PC に持ち込んだオフライン環境でも単体起動できることを前提にしています。

1. `artifact.yml` から `project` / `mode` / `runtime_config_dir` を取得
2. compose（`--compose-file` または自動解決）と `functions.yml` から必要イメージを収集
3. `runtime_config_dir` を統合して `runtime-config` を作成（競合ファイルは fail）
4. runtime 用に `docker compose config` を展開した `docker-compose.bundle.yml` を生成し、`build` セクションを除去
5. `images.tar` / `runtime-config` / `.env` / cert を同梱した DinD イメージを構築
6. DinD 起動時にローカル registry を自動起動・seed（失敗時は即終了）
7. DinD 起動時に one-shot `provisioner` を自動実行し、DynamoDB/S3 リソースを初期化

## 使用方法

```bash
# 単一 artifact ディレクトリ
./tools/dind-bundler/build.sh \
  -a e2e/artifacts/e2e-docker \
  -e e2e/environments/e2e-docker/.env \
  my-stack-bundle:latest

# 複数 artifact ディレクトリ + 明示 compose
./tools/dind-bundler/build.sh \
  -a artifacts/a \
  -a artifacts/b \
  -e environments/prod/.env \
  -c environments/prod/docker-compose.yml \
  --prepare-images \
  my-stack-bundle:latest
```

## compose の解決順

`--compose-file` を指定しない場合は次の順で解決します。

1. `--env-file` と同じディレクトリの `docker-compose.yml`
2. artifact `mode` に対応するルート compose
   - `docker` -> `docker-compose.docker.yml`
   - `containerd` -> `docker-compose.containerd.yml`

## 必須入力

- 各 `--artifact-dir` 配下に `artifact.yml` が存在すること
- `artifact.yml` の entry から辿れる `runtime_config_dir` が存在すること
- `--env-file` で渡す環境変数ファイルが存在すること
- 収集対象イメージ（compose service image + functions.yml の function image）がローカルに存在すること
  - 不足時は `--prepare-images` で自動準備を試行可能
- 証明書ディレクトリに以下が揃っていること
  - `rootCA.crt`, `server.crt`, `server.key`, `client.crt`, `client.key`

## 環境変数

- `CERT_DIR=<path>`: 証明書ディレクトリ（既定: `./.<artifact-project>/certs`）
- `RUN_UID=<uid>`, `RUN_GID=<gid>`: 証明書コピー時の所有者指定
- `AUTO_PROVISION_ON_BOOT=0`: 起動時 one-shot provisioner を無効化（既定は有効）

補足:
- `--env-file` で指定したファイルを `.env` として同梱します。
- コンテナ内では `CERT_DIR` と `CONFIG_DIR` は固定されます。
  - `CERT_DIR=/root/.<artifact-project>/certs`
  - `CONFIG_DIR=/app/runtime-config`
- `PORT_*` が未設定または `0` の場合、DinD 同梱時に次の固定値を設定します。
  - `PORT_GATEWAY_HTTPS=8443`
  - `PORT_VICTORIALOGS=9428`
  - `PORT_AGENT_METRICS=9091`
  - `PORT_S3=9000`
  - `PORT_S3_MGMT=9001`
  - `PORT_DATABASE=8000`
  - `PORT_REGISTRY=5010`
- `--prepare-images` は以下を実行して不足画像の補完を試みます。
  - `docker compose build`
  - `docker compose pull --ignore-pull-failures`
  - artifact の `functions/*/Dockerfile` から不足関数画像を `docker buildx build --load` で作成
- `--prepare-images` 実行時、`FROM 127.0.0.1:<port>/...` を使う Dockerfile 向けにホスト側ローカル registry を自動利用し、buildx から参照できるよう必要イメージを自動 publish します。

## 実行方法

作成されたイメージは特権モードで実行してください。

```bash
docker run --privileged --name stack-bundle \
  -p 8443:8443 -p 9428:9428 -p 9091:9091 -p 9000:9000 -p 9001:9001 -p 8000:8000 \
  -d my-stack-bundle:latest
```

## ファイル構成

- `build.sh`: artifact 入力から DinD イメージを作るオーケストレーション
- `Dockerfile`: DinD ベースイメージ定義
- `entrypoint.sh`: dockerd 起動・`images.tar` ロード・ローカル registry 自動 seed・起動時 provision 実行・`docker compose up`
