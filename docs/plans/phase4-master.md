# Phase 4: Stability & Observability Reconstruction 実装計画書

## 目的 (Goal)

Go Agent 導入によって失われた機能（流量制御、ログメタデータの整合性）を再実装し、全ての E2E テスト（Autoscaling, Logs 含む）をパスさせる。その後、不要となった旧コードを削除し、アーキテクチャをクリーンにする。

## ロードマップと優先順位

1. **Step 1: オートスケーリングと流量制御の再実装** (Critical)
* Gateway 側での同時実行数制限（Semaphore）とキューイングの実装。


2. **Step 2: ログ収集基盤 (VictoriaLogs) の適合** (High)
* containerd 環境下でのメタデータ整合性の確保とネットワーク疎通確認。


3. **Step 3: レガシーコードの削除** (Medium)
* Python Orchestrator, Fluent Bit, Go Docker Runtime の削除。



---

## Step 1: オートスケーリングと流量制御の再実装

現在、Gateway はリクエストを無制限に Agent に流しています（Infinite Concurrency）。これを制御し、規定数を超えたら待機（Queueing）させる仕組みを `GrpcBackend` に組み込みます。

### Task 1.1: `SemaphoreMap` の実装

関数ごとの同時実行数を管理するメカニズムを作成します。

* **対象ファイル**: `services/gateway/services/grpc_backend.py`
* **実装内容**:
* `function_name` をキーとした `asyncio.Semaphore` の辞書を管理。
* 設定値 `MAX_CONCURRENT_REQUESTS` (デフォルト: 10 など) を読み込む。



### Task 1.2: リクエストキューイングの実装

セマフォが一杯の場合に即エラーにするのではなく、一定時間待機するロジックを追加します。

* **対象ファイル**: `services/gateway/services/grpc_backend.py`
* **変更点**: `invoke` メソッド内を以下のようにラップする。
```python
# 擬似コード
sem = self._get_semaphore(function_name)
try:
    async with asyncio.timeout(QUEUE_TIMEOUT):
        async with sem:
            return await self.stub.Invoke(...)
except asyncio.TimeoutError:
    raise QueueFullError("Request timed out in queue")

```



### Task 1.3: スケーリング E2E テストの復活

* **対象ファイル**: `tests/scenarios/autoscaling/test_e2e_autoscaling.py` 他
* **アクション**: `@pytest.mark.skip` を外し、テストを実行・デバッグしてパスさせる。

---

## Step 2: ログ収集基盤 (VictoriaLogs) の適合

`VictoriaLogsHandler` (HTTP Push) が containerd 環境から正しく動作するように修正します。

### Task 2.1: ネットワーク疎通の確認と修正

Agent 内の Lambda コンテナ (CNI Network: `10.88.x.x`) から、VictoriaLogs (Docker Network: `172.x.x.x`) への HTTP 通信が可能か確認します。

* **課題**: CNI ネットワークから Docker ネットワークのホスト名 (`victoria-logs`) が解決できない可能性がある。
* **対策**:
* Go Agent の `Ensure` 時に、コンテナの `/etc/hosts` に VictoriaLogs の IP を注入するか、Gateway 経由でログを送るアーキテクチャへの微修正が必要かを判断。
* または、`extra_hosts` 設定を `oci.Spec` に追加する。



### Task 2.2: メタデータの整合性確保

VictoriaLogs に送られるログには `container_id` や `function_name` がタグ付けされています。これらが Go Agent の管理する ID と一致しているか確認します。

* **対象ファイル**: `services/common/core/logging_config.py` (VictoriaLogsHandler)
* **アクション**:
* 環境変数 `AWS_LAMBDA_FUNCTION_NAME` 等が正しくコンテナに渡っているか確認。
* コンテナIDが `containerd` の ID (SHA256) に変わっているため、ログ検索時の ID 指定方法を合わせる。



### Task 2.3: ログ検証テストの復活

* **対象ファイル**: `tests/scenarios/standard/test_observability.py`
* **アクション**: スキップを外し、ログがクエリできることを確認する。

---

## Step 3: レガシーコードの削除 (Cleanup)

システムが安定稼働し、全テストが通った後に実施します。

### Task 3.1: Python Orchestrator の削除

もはや使用されない旧コンテナ管理サービスを削除します。

* `services/orchestrator/` ディレクトリ全体。
* `docker-compose.yml` から `orchestrator` サービス定義を削除。

### Task 3.2: Fluent Bit 設定の削除

Push 型 (VictoriaLogsHandler) に一本化したため、不要な設定を削除します。

* `config/fluent-bit.conf`, `config/parsers.conf`
* `docker-compose.yml` から `fluent-bit` サービス定義を削除。

### Task 3.3: Go Docker Runtime の削除

* `services/agent/internal/runtime/docker/` パッケージ。
* `go.mod` から `github.com/docker/docker` 依存を削除（`containerd` のみ残す）。

---

## 完了条件 (Definition of Done)

1. **全 E2E テスト通過**: `pytest tests/` が `skips` なし（VictoriaLogs/Autoscaling含む）でオールグリーンになること。
2. **流量制御**: 負荷をかけても Gateway/Agent がクラッシュせず、適切にキューイングまたは 429/503 エラーを返すこと。
3. **ログ可視化**: 実行した関数のログが VictoriaLogs (またはUI) から検索できること。
4. **クリーンアップ**: 不要なファイルや Docker イメージ依存が削除されていること。

このプランで Phase 4 を進めていきます。まずは **Step 1: オートスケーリングの再実装** から着手しましょう。