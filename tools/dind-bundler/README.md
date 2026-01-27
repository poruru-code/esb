# DinD Bundler Tools

このディレクトリには、ESB (Edge Serverless Box) スタック全体を単一の Docker-in-Docker (DinD) コンテナにパッケージングするためのツールが含まれています。
これにより、インターネット接続のない環境や、完全に自己完結したデモ/テスト環境において、`docker pull` やビルドプロセスなしで ESB スタックを実行できます。

## 概要

このツールセットは以下の機能を提供します：
1.  **ビルド自動化**: `esb build` を実行して Lambda 関数イメージを生成し、必要なベースイメージと共に単一の tar アーカイブにまとめます。
2.  **自己完結イメージ**: すべての依存イメージと設定ファイルを含む DinD イメージを作成します。
3.  **自動起動**: コンテナ起動時に内部 Docker デーモンを立ち上げ、イメージをロードし、`docker-compose` でスタックを起動します。

## 使用方法

プロジェクトのルートディレクトリで以下のコマンドを実行してください。

```bash
# 基本的な使用方法
./tools/dind-bundler/build.sh <SAMテンプレートパス> <出力イメージタグ>

# 例: e2eテスト用のテンプレートを使用してビルドする場合
./tools/dind-bundler/build.sh e2e/fixtures/template.yaml my-esb-bundle:latest
```

※ `SAMテンプレートパス` は必須です。

### ビルドオプション (環境変数)

*   `SKIP_ESB_BUILD=true`: `esb build` プロセスをスキップします（既存のアーティファクトを再利用する場合やテスト用）。
*   `ESB_ENV=<env>` / `${ENV_PREFIX}_ENV`: `esb build --env` に渡す環境名を指定します。
*   `CERT_DIR=<path>`: 証明書の保存先を指定します（デフォルト: `~/.<cli_cmd>/certs`）。存在しない場合はエラーになります。
*   `ESB_OUTPUT_DIR=<path>` / `${ENV_PREFIX}_OUTPUT_DIR`: `esb build` の出力ディレクトリ（デフォルト: `.<cli_cmd>`）。
*   `BUNDLE_MANIFEST_PATH=<path>`: バンドル用マニフェストのパスを明示指定します。

`.env` が存在する場合は DinD イメージに同梱されます。`.env` がない場合は、
`ENV` と `RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` の最小構成を生成します。

証明書は事前に用意されている必要があります（ダミー生成は行いません）。

## マニフェスト駆動

バンドラーは **マニフェストを唯一の入力** としてイメージを収集します。
`esb build` は `--bundle-manifest` により `bundle/manifest.json` を生成し、
ビルドスクリプトはこのファイルを読み取って `images.tar` を作成します。
既定パスは `.<cli_cmd>/<env>/bundle/manifest.json` です。

## 実行方法

作成されたイメージは、特権モード (`--privileged`) で実行する必要があります。

```bash
docker run --privileged --name esb-bundle -p 8443:8443 -p 9000:9000 -p 9001:9001 -p 9428:9428 -d my-esb-bundle:latest
```

## アーキテクチャとフロー

### ビルドプロセス

```mermaid
sequenceDiagram
    participant User as ユーザー
    participant Script as build.sh
    participant CLI as esb CLI
    participant Docker as Docker Daemon
    participant Tar as images.tar
    participant Image as Target Image

    User->>Script: 実行 (template.yaml)
    Script->>Script: 証明書確認 (なければエラー)
    Script->>CLI: esb build (関数のビルド)
    CLI->>Docker: 関数イメージの作成 (com.esb.kind=function)
    Script->>Docker: 外部イメージのPull (Scylla, RustFS等)
    Script->>Docker: 全イメージのSave
    Docker->>Tar: イメージのエクスポート
    Script->>Docker: docker build (DinDイメージ)
    Docker->>Image: コンテキスト(Tar, Config)を含めてビルド
```

### 実行 (Runtime) プロセス

```mermaid
sequenceDiagram
    participant User as ユーザー
    participant Container as DinDコンテナ
    participant Entrypoint as entrypoint.sh
    participant Dockerd as Internal Dockerd
    participant Compose as docker-compose

    User->>Container: docker run --privileged
    Container->>Entrypoint: 起動
    Entrypoint->>Dockerd: バックグラウンドで起動
    loop 待機
        Entrypoint->>Dockerd: 接続確認 (docker info)
    end
    Entrypoint->>Dockerd: docker load -i images.tar
    Dockerd-->>Entrypoint: ロード完了
    Entrypoint->>Entrypoint: images.tar を削除 (容量確保)
    Entrypoint->>Compose: docker compose up
    Compose->>Dockerd: サービスコンテナの起動
```

## ファイル構成

*   `build.sh`: バンドルイメージ作成のオーケストレーションを行うスクリプト。
*   `Dockerfile`: DinD ベースの Dockerfile。`docker-compose` のインストールとアーティファクトのコピーを行います。
*   `entrypoint.sh`: コンテナ起動時の初期化（Dockerd起動、イメージロード）とサービスの立ち上げを行います。
