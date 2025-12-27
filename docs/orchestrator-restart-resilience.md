# Orchestrator再起動時のコンテナ復元（Adopt & Sync）

## 概要

**Orchestrator**コンテナが再起動した際、インメモリ状態（`last_accessed`, `locks`）が消失します。従来の「全削除（Kill-All）」方式では、Orchestrator再起動時に全Lambdaコンテナを強制終了していたため、次のリクエストで全てコールドスタートが発生していました。

**Adopt & Sync**方式では、Orchestrator起動時にDockerデーモンから既存コンテナの状態を同期し、実行中のコンテナは管理下に復帰、停止中のコンテナのみクリーンアップします。これにより、Orchestrator再起動時もサービス断を最小化できます。

> [!NOTE]
> 本ドキュメントの「実装詳細」は、**Python版 Orchestrator Service** の内部挙動を記述しています。
> **Go Agent (v2.1)** を使用する場合、Agent自体は **Stateless (Pure Factory)** であり、Gateway の `PoolManager` が Agent の `ListContainers` API を使用して状態同期（Adopt & Sync）を行います。

---

## 実装詳細 (Python Orchestrator)

### 1. `sync_with_docker()` メソッド

Orchestrator起動時（`main.py`の`lifespan`イベント）に実行されます。

**処理フロー:**
1. Dockerデーモンからラベル `created_by=esb` を持つ全コンテナを取得
2. コンテナの状態を確認:
   - **実行中（running）**: `last_accessed` に現在時刻を登録して管理下に復帰
   - **停止中（exited/paused等）**: `force=True` で削除
3. 同期結果をログ出力

**コード:** [`services/orchestrator/service.py`](../services/orchestrator/service.py)

---

### 2. 409 Conflictハンドリング

稀なレースコンディション（ロック取得の隙間で他プロセスがコンテナを作成）に対応するため、`ensure_container_running()`に409エラーハンドリングを追加しました。

**処理フロー:**
1. コンテナ作成を試行（`docker.run_container()`）
2. `APIError(status_code=409)` が返された場合:
   - 既存コンテナを取得（`docker.get_container()`）
   - 処理を続行（エラーにしない）
3. その他のエラーは再throwして上位でハンドリング

**コード:** [`services/orchestrator/service.py`](../services/orchestrator/service.py)

---

## テスト

### ユニットテスト

- `test_sync_with_docker_adopts_running_containers`: 実行中コンテナの復帰
- `test_sync_with_docker_removes_exited_containers`: 停止中コンテナの削除
- `test_sync_with_docker_handles_mixed_containers`: 混在ケース
- `test_ensure_container_running_handles_409_conflict`: 409 Conflictハンドリング

**実行:**
```bash
pytest services/orchestrator/tests/test_service.py -v -k "sync_with_docker or conflict"
```

### E2Eテスト

実際のOrchestrator再起動シナリオをテスト:

**テストシナリオ:** [`tests/test_e2e.py::TestE2E::test_orchestrator_restart_container_adoption`](../tests/test_e2e.py)

1. Lambda関数を呼び出してコンテナを起動（ウォームアップ）
2. `docker compose restart orchestrator` でOrchestratorを再起動
3. 同じLambda関数を再度呼び出し → **ウォームスタートで起動することを確認**

**実行:**
```bash
python tests/run_tests.py
```

**安定性検証結果（3回実行）:**
- 1回目: 11 passed in 35.64s ✅
- 2回目: 11 passed in 35.08s ✅
- 3回目: 11 passed in 35.34s ✅

---

## 効果

| 観点       | Before (Kill-All)      | After (Adopt & Sync)   |
| ---------- | ---------------------- | ---------------------- |
| **可用性** | 再起動時に全サービス断 | 実行中コンテナは維持 ✅ |
| **整合性** | 強引に整合（全削除）   | Dockerと同期して整合 ✅ |
| **堅牢性** | Conflict未対応         | 自己修復的に動作 ✅     |

---

## 参考資料

- TDD実装の詳細: 会話履歴 `72bcba0c-e16d-46c9-96ca-e79836ac5cef`
- 関連コミット:
  - `feat: Adopt & Sync方式でSPOF・状態管理問題を解決`
  - `test: Orchestrator再起動時のコンテナ復元E2Eテストを追加`
