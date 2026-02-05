<!--
Where: cli/docs/docker-architecture.md
What: Image design principles and build pipeline structure.
Why: Capture base image strategy and build constraints.
-->
# Docker イメージ設計アーキテクチャ

本基盤における Docker イメージの設計思想、原則、およびビルドパイプラインの構造について記述します。本ドキュメントは、システムの堅牢性とメンテナンス性を維持するための技術的指針となります。

---

## 1. 設計思想と原則

本基盤のイメージ設計は、以下の 3 つの核心的原則に基づいています。

### 1.1 不変性と一貫性 (Immutability & Consistency)
OS およびランタイムの断片化（例：Alpine と Debian の混在）は、ライブラリ互換性 (libc) やパッケージ管理の複雑化を招きます。
- **原則**: 全てのシステムサービスは Debian 12 を共通基盤とし、Python サービスは `<brand>-python-base`、非 Python サービスは `<brand>-os-base` を起点とする。
- **利点**: バイナリ互換性の 100% 確保、およびセキュリティ脆弱性スキャンの効率化。

### 1.2 隔離性とセキュリティ (Isolation & Security)
広すぎるビルドコンテキストは、機密情報の漏洩リスクを高め、ビルドキャッシュの効率を低下させます。
- **原則**: Dockerfile では必要なサブディレクトリのみを `COPY` し、依存解決はサービス固有の定義を参照する。
- **実装例**: `services/gateway/pyproject.toml` を使って依存を解決し、`services/common` と `services/gateway` のみをコピーする。
- **利点**: 無関係な変更でキャッシュが壊れない、意図しないファイル依存を排除できる。

### 1.3 決定論的信頼 (Deterministic Trust)
動的な証明書設定は、実行時の予期せぬ失敗（x509 エラー）の温床となります。
- **原則**: Root CA はビルド時に焼き込み、存在しなければビルドを失敗させる。
- **利点**: 実行時の信頼ストア更新が不要になり、最小権限に寄せられる。

---

## 2. ビルドパイプライン構造

本基盤のビルドプロセスは、効率的なキャッシュ利用とクリーンな最終イメージ生成のために、マルチステージビルドを標準化しています。

```mermaid
graph TD
    subgraph "Base Layer"
        OS["<brand>-os-base (Debian 12)"]
        PY["<brand>-python-base (Debian 12 + Python 3.12)"]
        TRUST["Root CA Layer (Build-Time)"]
        OS --> TRUST
        PY --> TRUST
    end

    subgraph "Builder Stage"
        BUILD["Builder Stage (<brand>-python-base)"]
        UV["uv Binary (Installer)"]
        SPEC["Gateway deps (services/gateway/pyproject.toml)"]
        DEPS["Dependencies (venv)"]
        
        BUILD --> UV
        UV --> SPEC
        SPEC --> DEPS
    end

    subgraph "Production Stage (Final)"
        PROD["Prod Stage (<brand>-python-base)"]
        COPY_VENV["COPY --from=builder /app/.venv"]
        COPY_APP["COPY services/common + services/gateway"]
        ENTRY["entrypoint.sh (Service Init)"]
        
        PROD --> COPY_VENV
        COPY_VENV --> COPY_APP
        COPY_APP --> ENTRY
    end

    TRUST -.-> PROD
    SRC -.-> COPY_APP
```

---

## 3. 重要コンポーネントの詳解

### 3.1 Root CA のビルド時焼き込み
Root CA はビルド時にイメージへ焼き込み、実行時に更新しません。
- **BuildKit secret `meta.RootCAMountID`**: `${CERT_DIR}/rootCA.crt` をビルド時に渡し、`/usr/local/share/ca-certificates/rootCA.crt` として配置します。
- **ビルド時更新**: `update-ca-certificates` をビルドで実行し、実行時の権限要件を排除します。
- **適用対象**: `<brand>-os-base` と `<brand>-python-base` の両方で同一の CA ストアを保持します。
- **BuildKit 必須**: `docker build --secret` / `docker buildx bake` の build secrets を利用します。
- **ローテーション**: CA を更新する場合はイメージを再ビルドします。
- **mTLS クライアント証明書**: `tools/cert-gen/generate.py` が `client.crt`/`client.key`
  を生成し、Gateway ↔ Agent の gRPC mTLS で利用します。

### 3.2 パッケージ管理 (`uv`)
ビルドの高速化と再現性のために `uv` を採用しています。
- **開発用バイナリの同梱**: `prod` イメージにも `/usr/local/bin/uv` を同梱し、運用時のライブラリデバッグを容易にしています。

### 3.3 ランタイム系統別イメージ分離
Docker / containerd の2系統に分離し、containerd 系統が Firecracker 依存を包含します。
- **Runtime Node**: `services/runtime-node/Dockerfile.containerd` のみを使用します。
- **Gateway/Agent/Provisioner**: `Dockerfile.docker` / `Dockerfile.containerd` を使い分けます。

---

## 4. 今後の拡張への指針

- **非 root 実行**: Gateway は Docker モードで非 root で動作します。`<repo_root>/.<brand>/certs`
  を読むため、compose の `RUN_UID`/`RUN_GID`（Dockerfile の `SERVICE_UID`/`SERVICE_GID`）を
  ホストの UID/GID に合わせてください。
  containerd 系（WireGuard 利用時）は `user: 0:0` を指定します。
- **C 拡張への対応**: 新たなライブラリを追加する際は、`builder` ステージでビルドされたバイナリが
  `prod` ステージで必要とする共有ライブラリ (`.so`) を、OS パッケージとして `apt-get` 等で追加することを忘れないでください。

---

## Implementation references
- `services/common/Dockerfile.os-base`
- `services/common/Dockerfile.python-base`
- `services/gateway/Dockerfile.docker`
- `services/gateway/Dockerfile.containerd`
