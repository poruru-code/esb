<!--
Where: README.md
What: Project overview, usage, and CLI reference.
Why: Provide a single entry point for developers and operators.
-->
# Edge Serverless Box

**オンプレミス環境のための、自己完結型サーバーレス実行基盤**
*(A self-contained, serverless-compatible environment for on-premises and local development)*

### 特徴
- **True AWS Compatibility**: 実行エンジンに **AWS Lambda Runtime Interface Emulator (RIE)** を採用。クラウド上の Lambda と完全に一致する挙動をローカル環境で保証します。
- **Integrated Developer Experience (CLI)**: 専用 CLI ツール `esb` を提供。環境構築の核として SAM ビルドを提供し、起動は `docker compose` で実現します。
- **Production-Ready Architecture**: 外部公開用の `Gateway` と特権を持つ `Go Agent` を分離したマイクロサービス構成により、セキュリティと耐障害性を実現しています。
- **Docker-Contained Runtime**: `runtime-node` に `containerd + CNI + CoreDNS` を集約し、ホストのネットワーク改変を最小化しつつ Firecracker/Remote Node での透明なサービス解決を実現します。
- **Full Stack in a Box**: S3互換ストレージ (RustFS)、DynamoDB互換DB (ScyllaDB)、ログ基盤を同梱しており、`docker compose` で Control + Compute を立ち上げるだけでクラウドネイティブ環境が得られます。
- **Efficient Orchestration**: コンテナオーケストレーション技術により、Lambda関数コンテナをオンデマンドで起動・プーリング。`ReservedConcurrentExecutions` に基づくオートスケーリングと、**Scale-to-Zero (アイドル時自動停止)** によりリソースを最適化します。Gateway 側の Janitor がアイドルコンテナと孤児コンテナを定期的に整理します。

### CLI コマンド一覧

| コマンド       | 説明                                                                                   | 主なオプション                                                                               |
| -------------- | -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `esb build`    | SAM テンプレート (`template.yaml`) を解析して Dockerfile / config を生成し、関数イメージをビルドします。 | `--template (-t)`, `--env (-e)`, `--mode (-m)`, `--env-file`, `--output (-o)`, `--no-cache`, `--verbose (-v)`, `--force`, `--no-save-defaults`, `--bundle-manifest` |
| `esb deploy`   | SAM テンプレートを解析し、関数イメージのビルドと `CONFIG_DIR` への設定マージを行います。 | `--template (-t)`, `--env (-e)`, `--mode (-m)`, `--project (-p)`, `--env-file`, `--output (-o)`, `--no-cache`, `--no-deps` (default), `--with-deps`, `--verbose (-v)`, `--force`, `--no-save-defaults` |
| `esb completion` | Bash / Zsh / Fish 用の補完スクリプトを生成します。                                      | `bash`, `zsh`, `fish`                                                                        |
| `esb version`   | CLI のバージョン情報を表示します。                                                      | —                                                                                            |

補足:
- 対話実行時は `--template` / `--env` / `--mode` を省略でき、未指定分は入力を促します。非対話（TTYなし）では必須です。
- `esb deploy` は起動中の compose project を対話的に選択できます。非対話時は `--project` の指定を推奨します。
- 対話入力の直近値は `<repo_root>/.<brand>/config.yaml` に保存され、同一テンプレートパスの既定値やテンプレート選択の履歴に使われます（無効化は `--no-save-defaults`）。
- テンプレート内の `Parameters` は実行時に対話入力して解決されます。
- CLI はランタイムの起動・停止機能を提供しておらず、生成物は `docker compose` で起動します。

## アーキテクチャ

```mermaid
flowchart TD
    User([Developer / Client]) -->|HTTPS| Gateway["API Gateway - FastAPI"]

    subgraph Control ["Control Plane (docker-compose.docker.yml)"]
        Gateway
        RustFS["RustFS (S3)"]
        ScyllaDB["ScyllaDB (DynamoDB)"]
        VictoriaLogs["VictoriaLogs"]
        Registry["Registry"]
    end

    subgraph Compute ["Compute Plane (docker-compose.containerd.yml)"]
        RuntimeNode["runtime-node (containerd + CNI)"]
        Agent["Go Agent (gRPC)"]
        CoreDNS["CoreDNS (Sidecar)"]
        LambdaRIE["Lambda RIE Containers"]
    end

    Gateway -->|gRPC| Agent
    Agent -->|containerd API| RuntimeNode
    RuntimeNode -->|CNI bridge| LambdaRIE
    LambdaRIE -->|AWS SDK / Logs| CoreDNS
    CoreDNS -->|Forward| Gateway
    CoreDNS -->|Forward| RustFS
    CoreDNS -->|Forward| ScyllaDB
    CoreDNS -->|Forward| VictoriaLogs
    Agent -.->|"Pull images (Containerd/FC only)"| Registry
    
    subgraph Toolchain ["CLI Toolchain"]
        esb[esb CLI] -->|build| GoBuilder[Go Builder]
    end
    
    GoBuilder -->|Generate| Routing[routing.yml]
    GoBuilder -->|Generate| Dockerfiles[Dockerfiles]
```

CLI は現在 `esb build` に特化しており、生成された構成は下の Compose ファイル群を使って手動で起動してください。

### システムコンポーネント
- **`Gateway`**: API Gateway 互換プロキシ。`routing.yml` に基づき認証・ルーティングを行い、Go Agent を介して Lambda コンテナをオンデマンドで呼び出します。
- **`Go Agent`**: コンテナのライフサイクル管理を担当。`containerd` を直接操作する高性能エージェントで、gRPC 通信により Gateway と高速かつ堅牢に連携します。
- **`runtime-node`**: `containerd + CNI` と DNSベースのサービスディスカバリを持つ実行基盤コンテナ。
- **`CoreDNS`**: `runtime-node` のサイドカーとして動作。Lambda VM からの DNS クエリを解決し、`s3-storage` / `database` などの論理名を適切な IP (Docker DNS または WireGuard Gateway) へマッピングします。
- **`esb CLI`**: SAM テンプレート (`template.yaml`) を **Single Source of Truth** とし、開発を自動化する統合コマンドラインツールです。

### ファイル構成
```text
.
├── docker-compose.docker.yml     # Docker mode (single file)
├── docker-compose.containerd.yml # Containerd mode (single file)
├── services/
│   ├── gateway/             # API Gateway (FastAPI)
│   ├── agent/               # Container Orchestrator (Go Agent)
│   ├── runtime-node/        # containerd + CNI runtime
│   └── common/              # 共通ライブラリ
├── config/                  # 設定ファイル
├── cli/                 # ESB CLI
├── tools/
│   ├── cert-gen/        # step-cli ベースの証明書生成
│   ├── gen_proto.py     # gRPC 生成補助
│   └── setup_devmapper.sh
├── e2e/                 # E2Eテスト用Lambda関数
│   ├── template.yaml    # SAM Source of Truth
│   └── functions/       # Lambda関数コード

```

### Compose ファイル構成と起動パターン

| ファイル                        | 役割                                   | 主な用途                                   |
| ------------------------------- | -------------------------------------- | ------------------------------------------ |
| `docker-compose.docker.yml`     | Docker モード                           | Docker ランタイムでの単一ノード構成         |
| `docker-compose.containerd.yml` | Containerd モード                        | Core + Compute を同一ホストで統合           |

#### Compose 要件
- Docker Compose プラグイン `v2.20+`（`additional_contexts` を使用するため）

#### Build 要件
- Docker Engine/CLI `23.0+`（`docker build --build-context` を使用するため）
- `DOCKER_BUILDKIT=0` は非対応（BuildKit 必須）

#### 起動パターン（docker compose）

単一ノード（containerd）:
```bash
docker compose -f docker-compose.containerd.yml up -d
```

Firecracker 相当（containerd ランタイム切替）:
```bash
CONTAINERD_RUNTIME=aws.firecracker docker compose -f docker-compose.containerd.yml up -d
```

#### Compose を使った起動パターン

- **Containerd**: `docker compose -f docker-compose.containerd.yml up -d`
- **Containerd + Firecracker**: `CONTAINERD_RUNTIME=aws.firecracker` を指定して起動

注意:
- 各モードは **単一ファイル**で完結します。
- 環境変数を分離したい場合は `--env-file` を併用します（例: `.env.prod`）。

#### docker-compose.yml (include) の使い方

利用者は `docker-compose.yml` を用意し、モード別 Compose を include して起動します。
環境変数は `.env` に記載します。

例: docker (include)
```yaml
include:
  - path: docker-compose.docker.yml
```

#### Proxy 設定の考え方

Proxy の有無に関わらず `docker-compose.yml` を使う前提です。

- config.json に proxy がある場合: **NO_PROXY のみを docker-compose.yml に明示**
- config.json に proxy がない場合: **HTTP(S)_PROXY と NO_PROXY を docker-compose.yml に明示**

例: NO_PROXY のみを追加 (config.json で proxy 済みの場合)
```yaml
x-no-proxy: &no_proxy
  NO_PROXY: localhost,127.0.0.1,::1,agent,database,gateway,s3-storage,victorialogs,registry,runtime-node,host.docker.internal
  no_proxy: localhost,127.0.0.1,::1,agent,database,gateway,s3-storage,victorialogs,registry,runtime-node,host.docker.internal

services:
  provisioner:
    environment:
      <<: *no_proxy
  gateway:
    environment:
      <<: *no_proxy
  agent:
    environment:
      <<: *no_proxy
```

例: proxy をすべて定義 (config.json がない場合)
```yaml
x-proxy: &proxy
  HTTP_PROXY: http://user:pass@proxy.example:8080
  HTTPS_PROXY: http://user:pass@proxy.example:8080
  NO_PROXY: localhost,127.0.0.1,::1,agent,database,gateway,s3-storage,victorialogs,registry,runtime-node,host.docker.internal
  http_proxy: http://user:pass@proxy.example:8080
  https_proxy: http://user:pass@proxy.example:8080
  no_proxy: localhost,127.0.0.1,::1,agent,database,gateway,s3-storage,victorialogs,registry,runtime-node,host.docker.internal

services:
  provisioner:
    environment:
      <<: *proxy
  gateway:
    environment:
      <<: *proxy
  agent:
    environment:
      <<: *proxy
```

## クイックスタート

詳細な開発環境セットアップ（`mise` / `lefthook` を使った依存インストールや Git フック設定）は [CONTRIBUTING.md#1開発環境セットアップ](CONTRIBUTING.md#1開発環境セットアップ) に詳述しています。

### 開発環境セットアップ

詳細なセットアップ手順や開発ガイドライン（Lint, Type Check, VS Code設定など）については、[CONTRIBUTING.md](CONTRIBUTING.md) を参照してください。

```bash
# 1. 仮想環境作成と依存関係インストール
uv sync --all-extras

# 2. Git hooks のセットアップ
lefthook install

# 3. 仮想環境の有効化
source .venv/bin/activate  # macOS/Linux
# .venv\Scripts\activate   # Windows
```

#### 開発ツールチェーン
本プロジェクトでは品質維持のために以下の最新ツールを採用しています：
*   **uv**: 高速なパッケージ管理
*   **Ruff**: 高速な Lint / Formatting
*   **Ty**: 統合型チェック


### 証明書のセットアップ

開発環境で使用するSSL証明書を生成します。

```bash
mise run setup:certs
```

これにより `<repo_root>/.<brand>/certs` に証明書が生成され、ローカル開発環境でHTTPSが利用可能になります。
初回実行時に `step certificate install` が実行され、ローカルのルートCAがシステムにインストールされます。
CA/サーバー証明書の有効期限は `tools/cert-gen/config.toml` で調整できます。


### ビルドと起動

`esb build` は SAM テンプレートを基に Dockerfile・`functions.yml`/`routing.yml`・`resources.yml` を生成し、関数コンテナと control-plane イメージをビルドします。`Parameters` に `Default` がない場合は対話入力されます。

出力先は既定で `<template_dir>/.<brand>/<env>` になります（`--output` で変更可能）。

```bash
# .env.example を .env にリネームして環境変数を用意
mv .env.example .env

# 本番は不変タグを明示
export <BRAND>_TAG=vX.Y.Z

# ビルド
esb build --template template.yaml --env prod --mode docker

# 起動（必要なら env ファイルを分離）
docker compose -f docker-compose.docker.yml --env-file .env.prod up -d

# containerd 本番（registry 必須）
export <BRAND>_TAG=vX.Y.Z
export <BRAND>_REGISTRY=registry.example.com/
esb build --template template.yaml --env prod --mode containerd
CONTAINERD_RUNTIME=aws.firecracker docker compose -f docker-compose.containerd.yml --env-file .env.prod up -d
```

## ドキュメント

詳細な技術ドキュメントは `docs/` ディレクトリにあります。

| ドキュメント                                      | 説明                                 |
| ------------------------------------------------- | ------------------------------------ |
| [環境変数一覧](docs/environment-variables.md)     | 全環境変数の役割と設定方法           |
| [architecture-containerd.md](docs/architecture-containerd.md) | 標準実行モード (Docker) の構成図     |
| [container-management.md](docs/container-management.md)         | コンテナ管理とイメージ運用           |
| [resilience.md](docs/resilience.md)                 | システム回復性とサーキットブレーカー |
| [spec.md](docs/spec.md)                             | システム仕様                         |

## 実行ガイド

1. SAM テンプレートのあるディレクトリで `esb build --template template.yaml --env prod --mode docker` を実行します。`Parameters` に `Default` がないものは対話的に尋ねられます。
2. 本番は `<BRAND>_TAG=vX.Y.Z` を設定してから `docker compose -f docker-compose.docker.yml --env-file .env.prod up -d` を実行します。Containerd は `docker-compose.containerd.yml` を使用し、`<BRAND>_REGISTRY` を必ず設定します。Firecracker は `CONTAINERD_RUNTIME=aws.firecracker` で切替えます。
3. `docker compose logs` / `docker compose down` などで監視・停止を行います。生成済 `.<brand>/<env>/config/`（または `--output` で指定したパス）には `functions.yml` / `routing.yml` / `resources.yml` が収められています。

### シェル補完

`esb completion <bash|zsh|fish>` で補完スクリプトを生成できます。設定例:

```bash
# bash
esb completion bash > ~/.bash_completion.d/esb
source ~/.bash_completion.d/esb

# zsh
mkdir -p ~/.zsh/completions
esb completion zsh > ~/.zsh/completions/_esb
fpath=(~/.zsh/completions $fpath)
autoload -Uz compinit && compinit

# fish
mkdir -p ~/.config/fish/completions
esb completion fish > ~/.config/fish/completions/esb.fish
```

## テスト

- Go のユニットテスト: `uv run go test ./cli/internal/...`
- E2E: `uv run python e2e/run_tests.py --unit-only` または `uv run python e2e/run_tests.py --profile e2e-containerd`

## トラブルシューティング

**Q. `esb` コマンドが見つからない**  
A. `.venv` を有効化するか、`uv run esb ...` を利用してください。

**Q. ビルド成果物をクリアしたい**  
A. `.<brand>/<env>/` を削除して再度 `esb build` を実行してください。
