# 同時実行E2Eテストのための環境分離設計

## 目的
1台のマシン上で、リソース（ポート、コンテナ名、ボリューム、ネットワーク）の競合を起こすことなく、Edge Serverless Box (ESB) のE2Eテストを複数同時に実行できるようにする。

## 設計思想 (Design Philosophy)
- **コア機能としての実装**: テスト専用の「一時的な処置」ではなく、CLIツールの **正式な機能（First-class Citizen）** として実装します。
    - `esb up --env <name>` のように、ユーザーが明示的に使用できる堅牢なインターフェースを提供します。
    - これにより、単一マシン上で「開発環境」「ステージング環境」「複数のテスト環境」を安全に共存・同時実行できる基盤を提供します。
- **E2Eテストは最初のユースケース**: 今回の対応は、この汎用的な環境分離機能を利用する **最初の適用例** としてE2Eテストの同時実行を実現するものです。

## 現状の課題
現在の実装では、以下がハードコードされています：
- **コンテナ名** (例: `esb-gateway`, `esb-registry`)
- **ホストポート** (例: 443, 50051, 5010, 8001)
- **Dockerネットワーク名** (例: `esb_net`, `runtime_net`)
- **ボリューム名** (例: `esb_registry_data`)

このため、テストランナーを複数同時に実行しようとすると、これらのリソースで競合が発生し、テストが失敗します（Port already in use, ConflictError 等）。

## 設計方針

**環境名 (Environment Name)** (`ESB_ENV`) という概念を導入し、環境を分離します。

### 1. 命名規則とスコープ
すべてのリソースはユニークな環境名（例: `dev-1`, `e2e-containerd`）によってスコープ化されます。

- **Docker プロジェクト名**: `esb-${ESB_ENV}` (Docker Composeの標準機能)
  - *効果*: コンテナ名（`esb-e2e-containerd-esb-gateway-1`）やネットワーク名（`esb-e2e-containerd_default`）に、指定した環境名がプレフィックスとして付与されます。
- **コンテナ名**: `docker-compose.yml` 内の明示的な `container_name` 指定は **削除** または **パラメータ化** します。
  - *決定事項*: 明示的な `container_name` フィールドは **削除** します。これにより Docker Compose が自動的に `${ProjectName}-${ServiceName}-${Index}` という一貫性のある名前を生成します。
  - *推奨*: これにより、「`E2E-containerd` 環境の `gateway`」は常に予測可能な名前になり、デバッグ時の特定が容易になります。
- **ボリューム**: トップレベルの `volumes` キーをパラメータ化するか、プロジェクトスコープのデフォルトボリュームを使用します。

### 2. 動的ポート割り当て
ホスト側のポートは、衝突を避けるために環境名に基づいてオフセットさせます。

- **ベースポート**: 標準ポートの定義（Gateway: 443, Registry: 5010など）。
- **オフセット計算**: `実ポート = ベースポート + (環境名のハッシュ % 1000)` 等。
  - ※ハッシュだと衝突の可能性があるため、テスト実行時は明示的にインデックスを指定するか、環境名から決定論的に算出するロジックを採用します。
- **実装**:
  - `docker-compose.yml`: `ports: - "${ESB_PORT_GATEWAY_HTTPS:-443}:443"` のようにパラメータ化。

### 3. ファイルと設定の分離
- **設定ディレクトリ**: `~/.esb` -> `~/.esb/${ESB_ENV}`
- **証明書**: `~/.esb/certs` -> `~/.esb/${ESB_ENV}/certs`
- **ソケット**: `/var/run/esb/...` -> `.../${ESB_ENV}/...`

### 4. ネットワークとサブネットの分離
- **外部ネットワーク**: `172.50.0.0/16` -> `172.${50+Index}.0.0/16`
- **ランタイムネットワーク**: `172.20.0.0/16` -> `172.${20+Index}.0.0/16`

## コンポーネント別の変更点

### A. Docker Compose ファイル (`docker-compose*.yml`)
**アクション**: ホストポート、サブネットをパラメータ化し、`container_name` を削除します。

```yaml
services:
  gateway:
    # container_name: esb-gateway  <-- 削除
    # 結果: esb-${ESB_ENV}-gateway-1 のような名前になります
    ports:
      - "${ESB_PORT_GATEWAY_HTTPS:-443}:443"
...
```

### B. CLIツール (`tools/cli/`)
**アクション**: 
1. `config.py`: `ESB_ENV` 環境変数に基づいてポートやパスを解決。
2. `up.py` / `down.py`: `esb up --env <name>` を受け取り、Docker Compose にプロジェクト名を渡す。
3. `compose.py`: 必要な環境変数を注入。

```python
# tools/cli/context.py

def get_env_name():
    return os.getenv("ESB_ENV", "default")
```

### C. リソースプロビジョナー (`tools/provisioner/main.py`)
（変更なし：注入された環境変数を使用）

### D. テストランナー (`e2e/run_tests.py`)
**アクション**:
1. テスト実行時に使用する環境名（例: `e2e-ci`, `e2e-dev`）を決定。
2. `esb up --env <name>` を呼び出す。

### E. ハードコード参照のリファクタリング
（変更なし）**アクション**: 監査で見つかった箇所（例: `cert.py`, `entrypoint.sh`）への対処。
- `cert.py`: SAN (Subject Alternative Name) に `esb-gateway` (内部サービス名) を使用するか？ -> はい、プロジェクトネットワーク内での内部DNSは安定的です。
- `build.py`: "Registry is not reachable" チェックは動的なレジストリポートを使用する必要があります。

## 詳細な実装手順

1. **Composeファイルのリファクタリング**:
   - 静的ポートを変数に置換。
   - `container_name` を削除。
   - ネットワーク名をパラメータ化。
2. **CLIコアの更新**:
   - `ESB_SESSION_ID` 検知の実装。
   - ポートマップを返す `get_env_vars_for_session()` の実装。
   - `cli/compose.py` でこれらの変数を注入するように更新。
3. **テストのリファクタリング**:
   - `conftest.py` が `os.getenv("GATEWAY_PORT")` 等を使用するように更新。
   - `run_tests.py` で分離をオーケストレーションするように修正。
4. **サービススクリプトのリファクタリング (必要な場合)**:
   - `runtime-node` 用の `entrypoint.sh` が、レジストリ接続先解決のためにロジックまたは環境変数を使用することを確認。

## 実行コマンド例 (Usage Examples)

### E2Eテストの同時実行 (Concurrent E2E Testing)
ターミナルを2つ開き、それぞれで以下のコマンドを実行します。

**Terminal A:**
```bash
# 環境名 "E2E-containerd" でテストを実行
# 内部的に `esb up --env E2E-containerd` が呼び出されます
# コンテナ名は esb-E2E-containerd-gateway-1 のようになります
ESB_ENV=E2E-containerd uv run e2e/run_tests.py
```

**Terminal B:**
```bash
# 環境名 "E2E-firecracker" でテストを実行
ESB_ENV=E2E-firecracker uv run e2e/run_tests.py
```

### 特定環境での手動デバッグ (Manual Debugging)
開発者が特定の分離環境を立ち上げて調査する場合：

```bash
# 環境 "dev-feature-x" を立ち上げ
uv run esb up --env dev-feature-x

# ログ確認 (プレフィックス付きのコンテナを自動解決)
uv run esb logs --env dev-feature-x
uv run esb down --env dev-feature-x
```

## コードレベルの変更仕様 (File & Function Level Changes)

### 1. `tools/cli/config.py`
- **変更点**: 環境名(`ESB_ENV`)とポートマッピングの管理ロジックを追加。
- **追加関数**:
  ```python
  def get_env_name() -> str:
      """取得ロジック: 引数 --env > 環境変数 ESB_ENV > 'default'"""
      pass
  
  def get_port_mapping(env_name: str) -> dict[str, str]:
      """環境名ハッシュに基づくポートオフセット計算"""
      pass
  ```

### 2. `tools/cli/compose.py`
- **変更内容**: `config.get_port_mapping()` の結果を `os.environ` にマージ。

### 3. `tools/cli/commands/up.py` / `down.py`
- **変更内容**:
    - `--env` 引数の追加。
    - Docker Compose 呼び出し時のプロジェクト名 (`-p esb-<env_name>`) を明示的に指定。

### 4. `tools/provisioner/main.py`
- **実装**: 環境変数から動的エンドポイントを構築。

### 5. `e2e/run_tests.py`
- **変更内容**:
    - `esb up --env <name>` を使用するように変更。

### 6. `tests/conftest.py` & シナリオファイル
- **変更内容**: `os.getenv` による動的ポート解決へ統一。

## 検証戦略
1. **単独実行**: デフォルトの "default" 環境で `esb up` が変わらず動作することを確認。
2. **同時実行**: `ESB_ENV=test1` と `test2` で同時起動し、`docker ps` で `esb-test1-...` と `esb-test2-...` が共存していることを確認する。
