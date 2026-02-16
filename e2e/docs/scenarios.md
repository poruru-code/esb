<!--
Where: e2e/docs/scenarios.md
What: Full list of E2E scenario test cases and runtime coverage matrix.
Why: Make docker/containerd support status explicit per test case.
-->
# E2Eシナリオとランタイム対応表

## 対象範囲
- 参照元:
  - `e2e/environments/test_matrix.yaml`
  - `e2e/scenarios/**/test_*.py`
- このドキュメントは、現在実装されているテストケースを一覧化したものです。
- パラメータ化テストは展開後のケース（例: `[python]`, `[java]`）として記載しています。

## シナリオ別サマリー

| シナリオ | Suite割り当て | 展開後ケース数 | Docker | Containerd | 備考 |
| --- | --- | ---: | :---: | :---: | --- |
| smoke | `smoke` | 10 | ✅ | ✅ | `test_connectivity` 2件 + `test_smoke` 8件 |
| autoscaling | `standard` | 6 | ✅ | ✅ | `../scenarios/autoscaling/` 経由で実行 |
| standard | `standard` | 29 | ✅ | ✅ | `../scenarios/standard/` 経由で実行 |
| runtime/java | `runtime` | 3 | ✅ | ❌ | 現在の matrix では `e2e-docker` のみ |
| runtime/python | `runtime` | 3 | ✅ | ❌ | 現在の matrix では `e2e-docker` のみ |
| restart | `restart` | 2 | ✅ | ❌ | 現在の matrix では `e2e-docker` のみ |
| **合計** |  | **53** |  |  |  |

## テスト一覧

### 目的カテゴリ定義
- `機能系`: 業務機能（echo, DynamoDB, S3, Lambda連携, scheduler等）が仕様どおり動作することを確認するテスト。
- `制御系`: ヘルスチェック、認証、ルーティング、基本ガードなど制御プレーン系の挙動を確認するテスト。
- `観測性`: ログ・メトリクス・トレースの取得/伝播が成立することを確認するテスト。
- `性能`: 並列実行やスケーリングなど、負荷時の挙動を確認するテスト。
- `耐障害`: 再起動・障害注入・回復シーケンスなど、障害時の復元性を確認するテスト。

### smoke

| テストファイル | テストケース | 説明ID | 目的カテゴリ | Docker | Containerd | 備考 |
| --- | --- | --- | --- | :---: | :---: | --- |
| `e2e/scenarios/smoke/test_connectivity.py` | `test_gateway_health` | [SMK-001](#desc-smoke-gateway-health) | 制御系 | ✅ | ✅ |  |
| `e2e/scenarios/smoke/test_connectivity.py` | `test_victorialogs_health` | [SMK-002](#desc-smoke-victorialogs-health) | 観測性 | ✅ | ✅ |  |
| `e2e/scenarios/smoke/test_smoke.py` | `test_runtime_echo[python]` | [SMK-003](#desc-smoke-runtime-echo-python) | 機能系 | ✅ | ✅ | `runtime=python` |
| `e2e/scenarios/smoke/test_smoke.py` | `test_runtime_echo[java]` | [SMK-004](#desc-smoke-runtime-echo-java) | 機能系 | ✅ | ✅ | `runtime=java` |
| `e2e/scenarios/smoke/test_smoke.py` | `test_runtime_dynamodb_put[python]` | [SMK-005](#desc-smoke-runtime-dynamodb-put-python) | 機能系 | ✅ | ✅ | `runtime=python` |
| `e2e/scenarios/smoke/test_smoke.py` | `test_runtime_dynamodb_put[java]` | [SMK-006](#desc-smoke-runtime-dynamodb-put-java) | 機能系 | ✅ | ✅ | `runtime=java` |
| `e2e/scenarios/smoke/test_smoke.py` | `test_runtime_s3_put[python]` | [SMK-007](#desc-smoke-runtime-s3-put-python) | 機能系 | ✅ | ✅ | `runtime=python` |
| `e2e/scenarios/smoke/test_smoke.py` | `test_runtime_s3_put[java]` | [SMK-008](#desc-smoke-runtime-s3-put-java) | 機能系 | ✅ | ✅ | `runtime=java` |
| `e2e/scenarios/smoke/test_smoke.py` | `test_runtime_chain_invoke[python]` | [SMK-009](#desc-smoke-runtime-chain-invoke-python) | 機能系 | ✅ | ✅ | `runtime=python` |
| `e2e/scenarios/smoke/test_smoke.py` | `test_runtime_chain_invoke[java]` | [SMK-010](#desc-smoke-runtime-chain-invoke-java) | 機能系 | ✅ | ✅ | `runtime=java` |

### autoscaling

| テストファイル | テストケース | 説明ID | 目的カテゴリ | Docker | Containerd | 備考 |
| --- | --- | --- | --- | :---: | :---: | --- |
| `e2e/scenarios/autoscaling/test_e2e_autoscaling.py` | `test_repeated_invocations` | [ASC-001](#desc-autoscaling-repeated-invocations) | 性能 | ✅ | ✅ |  |
| `e2e/scenarios/autoscaling/test_e2e_autoscaling.py` | `test_concurrent_queueing` | [ASC-002](#desc-autoscaling-concurrent-queueing) | 性能 | ✅ | ✅ |  |
| `e2e/scenarios/autoscaling/test_e2e_autoscaling.py` | `test_concurrent_stress_10_requests` | [ASC-003](#desc-autoscaling-concurrent-stress-10) | 性能 | ✅ | ✅ |  |
| `e2e/scenarios/autoscaling/test_e2e_autoscaling.py` | `test_concurrent_different_functions` | [ASC-004](#desc-autoscaling-concurrent-different-functions) | 性能 | ✅ | ✅ |  |
| `e2e/scenarios/autoscaling/test_scale_to_zero.py` | `test_invocation_after_idle_window` | [ASC-005](#desc-autoscaling-invocation-after-idle-window) | 性能 | ✅ | ✅ |  |
| `e2e/scenarios/autoscaling/test_scale_to_zero.py` | `test_periodic_requests_across_idle_window` | [ASC-006](#desc-autoscaling-periodic-requests-across-idle-window) | 性能 | ✅ | ✅ |  |

### standard

| テストファイル | テストケース | 説明ID | 目的カテゴリ | Docker | Containerd | 備考 |
| --- | --- | --- | --- | :---: | :---: | --- |
| `e2e/scenarios/standard/test_dynamo.py` | `test_put_get` | [STD-001](#desc-standard-dynamo-put-get) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_dynamo.py` | `test_update_item` | [STD-002](#desc-standard-dynamo-update-item) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_dynamo.py` | `test_delete_item` | [STD-003](#desc-standard-dynamo-delete-item) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_dynamo.py` | `test_get_nonexistent` | [STD-004](#desc-standard-dynamo-get-nonexistent) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_gateway_basics.py` | `test_health` | [STD-005](#desc-standard-gateway-basics-health) | 制御系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_gateway_basics.py` | `test_auth` | [STD-006](#desc-standard-gateway-basics-auth) | 制御系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_gateway_basics.py` | `test_routing_401` | [STD-007](#desc-standard-gateway-basics-routing-401) | 制御系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_gateway_basics.py` | `test_routing_404` | [STD-008](#desc-standard-gateway-basics-routing-404) | 制御系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_id_specs.py` | `test_id_propagation_with_chain` | [STD-009](#desc-standard-id-specs-id-propagation-with-chain) | 観測性 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_image_function.py` | `test_image_function_basic` | [STD-010](#desc-standard-image-function-basic) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_image_function.py` | `test_image_function_chain_invoke` | [STD-027](#desc-standard-image-function-chain-invoke) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_image_function.py` | `test_image_function_s3_access` | [STD-028](#desc-standard-image-function-s3-access) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_image_function.py` | `test_image_function_victorialogs` | [STD-029](#desc-standard-image-function-victorialogs) | 観測性 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_lambda.py` | `test_sync_chain_invoke` | [STD-011](#desc-standard-lambda-sync-chain-invoke) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_lambda.py` | `test_async_chain_invoke` | [STD-012](#desc-standard-lambda-async-chain-invoke) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_metrics_api.py` | `test_metrics_api` | [STD-013](#desc-standard-metrics-api) | 観測性 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_prometheus_metrics.py` | `test_metrics_endpoint_accessible` | [STD-014](#desc-standard-prometheus-metrics-endpoint) | 観測性 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_reconciliation.py` | `test_grace_period_prevents_premature_deletion` | [STD-015](#desc-standard-reconciliation-grace-period) | 耐障害 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_resilience.py` | `test_orchestrator_restart_recovery` | [STD-016](#desc-standard-resilience-orchestrator-restart-recovery) | 耐障害 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_resilience.py` | `test_gateway_cache_hit` | [STD-017](#desc-standard-resilience-gateway-cache-hit) | 性能 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_resilience.py` | `test_circuit_breaker` | [STD-018](#desc-standard-resilience-circuit-breaker) | 耐障害 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_s3.py` | `test_put_get` | [STD-019](#desc-standard-s3-put-get) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_s3.py` | `test_list_objects` | [STD-020](#desc-standard-s3-list-objects) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_s3.py` | `test_delete_object` | [STD-021](#desc-standard-s3-delete-object) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_s3.py` | `test_overwrite` | [STD-022](#desc-standard-s3-overwrite) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_s3.py` | `test_list_with_prefix` | [STD-023](#desc-standard-s3-list-with-prefix) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_s3.py` | `test_bucket_lifecycle_configuration` | [STD-024](#desc-standard-s3-bucket-lifecycle-configuration) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_scheduler.py` | `test_schedule_trigger` | [STD-025](#desc-standard-scheduler-schedule-trigger) | 機能系 | ✅ | ✅ |  |
| `e2e/scenarios/standard/test_trace_propagation.py` | `test_chained_trace_consistency` | [STD-026](#desc-standard-trace-propagation-chained-trace-consistency) | 観測性 | ✅ | ✅ |  |

### runtime/java

| テストファイル | テストケース | 説明ID | 目的カテゴリ | Docker | Containerd | 備考 |
| --- | --- | --- | --- | :---: | :---: | --- |
| `e2e/scenarios/runtime/java/test_echo.py` | `test_java_echo_basic` | [RTJ-001](#desc-runtime-java-echo-basic) | 機能系 | ✅ | ❌ | `runtime` suite は containerd に未割り当て |
| `e2e/scenarios/runtime/java/test_observability.py` | `test_java_echo_logs_and_trace` | [RTJ-002](#desc-runtime-java-echo-logs-and-trace) | 観測性 | ✅ | ❌ | `runtime` suite は containerd に未割り当て |
| `e2e/scenarios/runtime/java/test_observability.py` | `test_java_cloudwatch_logs_passthrough` | [RTJ-003](#desc-runtime-java-cloudwatch-logs-passthrough) | 観測性 | ✅ | ❌ | `runtime` suite は containerd に未割り当て |

### runtime/python

| テストファイル | テストケース | 説明ID | 目的カテゴリ | Docker | Containerd | 備考 |
| --- | --- | --- | --- | :---: | :---: | --- |
| `e2e/scenarios/runtime/python/test_echo.py` | `test_python_echo_basic` | [RTP-001](#desc-runtime-python-echo-basic) | 機能系 | ✅ | ❌ | `runtime` suite は containerd に未割り当て |
| `e2e/scenarios/runtime/python/test_observability.py` | `test_structured_log_format` | [RTP-002](#desc-runtime-python-structured-log-format) | 観測性 | ✅ | ❌ | `runtime` suite は containerd に未割り当て |
| `e2e/scenarios/runtime/python/test_observability.py` | `test_cloudwatch_logs_passthrough` | [RTP-003](#desc-runtime-python-cloudwatch-logs-passthrough) | 観測性 | ✅ | ❌ | `runtime` suite は containerd に未割り当て |

### restart

| テストファイル | テストケース | 説明ID | 目的カテゴリ | Docker | Containerd | 備考 |
| --- | --- | --- | --- | :---: | :---: | --- |
| `e2e/scenarios/restart/test_restart.py` | `test_service_process_crash_recovers[gateway]` | [RST-001](#desc-restart-service-process-crash-recovers) | 耐障害 | ✅ | ❌ | `restart` suite は docker のみに割り当て |
| `e2e/scenarios/restart/test_restart.py` | `test_service_process_crash_recovers[agent]` | [RST-001](#desc-restart-service-process-crash-recovers) | 耐障害 | ✅ | ❌ | `restart` suite は docker のみに割り当て |


## テストケース詳細

### smoke

- <a id="desc-smoke-gateway-health"></a>`SMK-001`

  保証:
  Gatewayのヘルスチェック経路が有効で、最低限の制御プレーン疎通が成立していることを保証する。

  入力:
  `GET /health` を実行し、Gateway が `200` を返すことを確認する。

  合格条件:
  `GET /health` が `200` を返すこと。ステータスコード以外の追加条件は設けず、Gateway のHTTP疎通成立を確認すること。

  失敗時の示唆:
  Gatewayコンテナ起動状態、`/health` ルーティング、ネットワーク到達性を確認する。

- <a id="desc-smoke-victorialogs-health"></a>`SMK-002`

  保証:
  ログ基盤（VictoriaLogs）のヘルスエンドポイントが到達可能であることを保証する。

  入力:
  `VictoriaLogs /health` への接続可否を確認し、ステータスが `200` または `204` であることを検証する。接続例外は即失敗として扱う。

  合格条件:
  `VictoriaLogs /health` への接続で例外が発生せず、ステータスが `200` または `204` のいずれかであること。

  失敗時の示唆:
  VictoriaLogsコンテナ起動状態、ポート公開、composeネットワーク疎通を確認する。

- <a id="desc-smoke-runtime-echo-python"></a>`SMK-003`

  保証:
  Python runtimeのecho経路（Gateway -> connectivity/python）が正常であることを保証する。

  入力:
  `/api/connectivity/python` に `{"action":"echo","message":"smoke-python"}` を送信し、`200`、`success=true`、`action="echo"`、`message="Echo: smoke-python"` の一致を確認する。

  合格条件:
  レスポンスが `200` で、`success=true`、`action="echo"`、`message="Echo: smoke-python"` の4条件をすべて満たすこと。

  失敗時の示唆:
  Gatewayの `/api/connectivity/*` ルーティング、対象runtime関数デプロイ、レスポンス組み立て処理を確認する。

- <a id="desc-smoke-runtime-echo-java"></a>`SMK-004`

  保証:
  Java runtimeのecho経路（Gateway -> connectivity/java）が正常であることを保証する。

  入力:
  `/api/connectivity/java` に `{"action":"echo","message":"smoke-java"}` を送信し、`200`、`success=true`、`action="echo"`、`message="Echo: smoke-java"` の一致を確認する。

  合格条件:
  レスポンスが `200` で、`success=true`、`action="echo"`、`message="Echo: smoke-java"` の4条件をすべて満たすこと。

  失敗時の示唆:
  Gatewayの `/api/connectivity/*` ルーティング、対象runtime関数デプロイ、レスポンス組み立て処理を確認する。

- <a id="desc-smoke-runtime-dynamodb-put-python"></a>`SMK-005`

  保証:
  Python runtime経由のDynamoDB書き込み経路が疎通していることを保証する。

  入力:
  `/api/connectivity/python` に `{"action":"dynamodb_put","key":"smoke-python","value":"ok"}` を送信し、`200`、`success=true`、`action="dynamodb_put"` を確認する。

  合格条件:
  レスポンスが `200` で、`success=true`、`action="dynamodb_put"` を満たすこと。

  失敗時の示唆:
  Gatewayの `/api/connectivity/*` ルーティング、対象runtime関数デプロイ、レスポンス組み立て処理を確認する。

- <a id="desc-smoke-runtime-dynamodb-put-java"></a>`SMK-006`

  保証:
  Java runtime経由のDynamoDB書き込み経路が疎通していることを保証する。

  入力:
  `/api/connectivity/java` に `{"action":"dynamodb_put","key":"smoke-java","value":"ok"}` を送信し、`200`、`success=true`、`action="dynamodb_put"` を確認する。

  合格条件:
  レスポンスが `200` で、`success=true`、`action="dynamodb_put"` を満たすこと。

  失敗時の示唆:
  Gatewayの `/api/connectivity/*` ルーティング、対象runtime関数デプロイ、レスポンス組み立て処理を確認する。

- <a id="desc-smoke-runtime-s3-put-python"></a>`SMK-007`

  保証:
  Python runtime経由のS3書き込み経路が疎通していることを保証する。

  入力:
  `/api/connectivity/python` に `{"action":"s3_put","bucket":"e2e-test-bucket","key":"python-smoke.txt","content":"ok"}` を送信し、`200`、`success=true`、`action="s3_put"` を確認する。

  合格条件:
  レスポンスが `200` で、`success=true`、`action="s3_put"` を満たすこと。

  失敗時の示唆:
  Gatewayの `/api/connectivity/*` ルーティング、対象runtime関数デプロイ、レスポンス組み立て処理を確認する。

- <a id="desc-smoke-runtime-s3-put-java"></a>`SMK-008`

  保証:
  Java runtime経由のS3書き込み経路が疎通していることを保証する。

  入力:
  `/api/connectivity/java` に `{"action":"s3_put","bucket":"e2e-test-bucket","key":"java-smoke.txt","content":"ok"}` を送信し、`200`、`success=true`、`action="s3_put"` を確認する。

  合格条件:
  レスポンスが `200` で、`success=true`、`action="s3_put"` を満たすこと。

  失敗時の示唆:
  Gatewayの `/api/connectivity/*` ルーティング、対象runtime関数デプロイ、レスポンス組み立て処理を確認する。

- <a id="desc-smoke-runtime-chain-invoke-python"></a>`SMK-009`

  保証:
  Python runtimeのチェーン呼び出し（親子Lambda）が成立することを保証する。

  入力:
  `/api/connectivity/python` に `{"action":"chain_invoke","target":"lambda-echo","message":"from-smoke"}` を送信し、親レスポンスの `success=true` と `action="chain_invoke"` を確認する。加えて `child` が存在し、子ペイロードでも `success=true` になることを確認する。

  合格条件:
  親レスポンスが `200` かつ `success=true`・`action="chain_invoke"` を満たし、`child` が存在し、子ペイロードでも `success=true` が成立すること。

  失敗時の示唆:
  Gatewayの `/api/connectivity/*` ルーティング、対象runtime関数デプロイ、レスポンス組み立て処理を確認する。

- <a id="desc-smoke-runtime-chain-invoke-java"></a>`SMK-010`

  保証:
  Java runtimeのチェーン呼び出し（親子Lambda）が成立することを保証する。

  入力:
  `/api/connectivity/java` に `{"action":"chain_invoke","target":"lambda-echo","message":"from-smoke"}` を送信し、親レスポンスの `success=true` と `action="chain_invoke"` を確認する。加えて `child` が存在し、子ペイロードでも `success=true` になることを確認する。

  合格条件:
  親レスポンスが `200` かつ `success=true`・`action="chain_invoke"` を満たし、`child` が存在し、子ペイロードでも `success=true` が成立すること。

  失敗時の示唆:
  Gatewayの `/api/connectivity/*` ルーティング、対象runtime関数デプロイ、レスポンス組み立て処理を確認する。


### autoscaling

- <a id="desc-autoscaling-repeated-invocations"></a>`ASC-001`

  保証:
  連続呼び出し時にpoolが適切に確保され、上限超過せずに処理できることを保証する。

  入力:
  `/api/echo` を連続2回実行し、`message` が `Echo: autoscale-1` と `Echo: autoscale-2` になることを確認する。`/metrics/pools` の対象エントリで、1回目後に `total_workers>=1`、2回目後に `total_workers<=max_capacity` を確認する。

  合格条件:
  1回目・2回目の `/api/echo` がともに `200` で、`message` がそれぞれ `Echo: autoscale-1` / `Echo: autoscale-2` であること。pool指標は1回目後 `total_workers>=1`、2回目後 `total_workers<=max_capacity`（`max_capacity==1` なら `total_workers==1`）を満たすこと。

  失敗時の示唆:
  `/metrics/pools` 応答、poolサイズ制御、worker作成/回収ロジックを確認する。

- <a id="desc-autoscaling-concurrent-queueing"></a>`ASC-002`

  保証:
  同時実行時にキュー/ワーカ制御が破綻せず、全リクエストを捌けることを保証する。

  入力:
  3並列で `/api/echo` を実行し、全リクエストが `200` かつ `message` に `Echo: concurrent-` を含むことを確認する。あわせて `total_workers<=max_capacity` を満たすことを確認する。

  合格条件:
  3並列の全リクエストが `200` で、各 `message` に `Echo: concurrent-` を含むこと。pool指標で `total_workers<=max_capacity` を満たすこと。

  失敗時の示唆:
  `/metrics/pools` 応答、poolサイズ制御、worker作成/回収ロジックを確認する。

- <a id="desc-autoscaling-concurrent-stress-10"></a>`ASC-003`

  保証:
  10並列の中負荷でも成功率100%を維持し、pool上限制約を守ることを保証する。

  入力:
  10並列実行で全件 `200` を確認し、各レスポンスの `success=true` と `Echo: stress-` を検証する。`/metrics/pools` で `1<=total_workers<=max_capacity` を確認する。

  合格条件:
  10並列で成功件数がちょうど10件（全件 `200`）であること。各レスポンスで `success=true` かつ `message` に `Echo: stress-` を含むこと。pool指標は `1<=total_workers<=max_capacity` を満たすこと。

  失敗時の示唆:
  `/metrics/pools` 応答、poolサイズ制御、worker作成/回収ロジックを確認する。

- <a id="desc-autoscaling-concurrent-different-functions"></a>`ASC-004`

  保証:
  異なる関数の同時実行で関数間干渉が起きず、双方のpoolが起動することを保証する。

  入力:
  `/api/echo` と `/api/faulty(action=hello)` を同時実行し、全件 `200` を確認する。`echo` 系と `chaos/faulty` 系の両 pool で `total_workers>=1` を確認する。

  合格条件:
  `/api/echo` と `/api/faulty(action=hello)` を同時実行した全4リクエストが `200` であること。`echo` 系poolと`chaos/faulty` 系poolの両方で `total_workers>=1` が成立すること。

  失敗時の示唆:
  `/metrics/pools` 応答、poolサイズ制御、worker作成/回収ロジックを確認する。

- <a id="desc-autoscaling-invocation-after-idle-window"></a>`ASC-005`

  保証:
  アイドル時間経過後にscale-to-zeroされても次回呼び出しで復帰できることを保証する。

  入力:
  初回 `/api/scaling` 呼び出し（`message=\"scale-to-zero-test\"`）成功後にアイドル時間超過まで待機し、`/metrics/pools` で `total_workers==0` になったことを確認する。その後 `message=\"scale-to-zero-after\"` で再呼び出しし、`200` かつ `message=\"scaling-test\"` で復帰することを検証する。

  合格条件:
  初回 `/api/scaling` が `200` かつ `message="scaling-test"`。待機後に pool 指標 `total_workers==0`。その後の再呼び出しが再び `200` かつ `message="scaling-test"` になること。

  失敗時の示唆:
  `/metrics/pools` 応答、poolサイズ制御、worker作成/回収ロジックを確認する。

- <a id="desc-autoscaling-periodic-requests-across-idle-window"></a>`ASC-006`

  保証:
  アイドル境界を跨ぐ定期トラフィック中はワーカが維持されることを保証する。

  入力:
  アイドル時間を跨ぐ長さで定期的に `/api/scaling` を送信し、全リクエスト `200` を確認する。中間時点で `total_workers>=1` を検証し、定期アクセス中は実行基盤が維持されることを確認する。

  合格条件:
  初回呼び出しと定期呼び出しのすべてで `/api/scaling` が `200` かつ `message="scaling-test"` を返すこと。中間時点のpool確認で `total_workers>=1` を満たすこと。

  失敗時の示唆:
  `/metrics/pools` 応答、poolサイズ制御、worker作成/回収ロジックを確認する。


### standard

- <a id="desc-standard-dynamo-put-get"></a>`STD-001`

  保証:
  DynamoDB互換のput/get基本整合（書いたIDを読める）を保証する。

  入力:
  `/api/dynamo(action=put_get)` を実行する。一時エラー（`500/502/503/504`）時は規定回数内で再試行し、最終的に `200`、`success=true`、`item_id` と `retrieved_item` が存在し、取得 ID が投入 ID と一致することを確認する。

  合格条件:
  最終レスポンスが `200` かつ `success=true`。`item_id` と `retrieved_item` を持ち、`retrieved_item.id.S == item_id` が成立すること。

  失敗時の示唆:
  lambda-dynamo実装、ScyllaDB接続情報、テーブル初期化状態を確認する。

- <a id="desc-standard-dynamo-update-item"></a>`STD-002`

  保証:
  DynamoDB updateが反映され、後続getで更新値を取得できることを保証する。

  入力:
  `put -> update -> get` の順で実行し、`update` 後の `get` で `found=true` かつ `message="Updated message"` になっていることを確認する。

  合格条件:
  `put` が `200`、`update` が `200` かつ `success=true`、`get` が `200` かつ `found=true`、さらに `item.message.S == "Updated message"` が成立すること。

  失敗時の示唆:
  lambda-dynamo実装、ScyllaDB接続情報、テーブル初期化状態を確認する。

- <a id="desc-standard-dynamo-delete-item"></a>`STD-003`

  保証:
  DynamoDB delete後に対象アイテムが存在しない状態へ遷移することを保証する。

  入力:
  `put -> delete -> get` を実行し、`delete` の `deleted=true` を確認する。続く `get` で `found=false` を確認し、削除結果を検証する。

  合格条件:
  `put` が `200`、`delete` が `200` かつ `deleted=true`、続く `get` が `200` かつ `found=false` となること。

  失敗時の示唆:
  lambda-dynamo実装、ScyllaDB接続情報、テーブル初期化状態を確認する。

- <a id="desc-standard-dynamo-get-nonexistent"></a>`STD-004`

  保証:
  DynamoDBで非存在キー参照時に安定してnot found応答を返せることを保証する。

  入力:
  存在しない UUID で `get` を実行し、`200`、`success=true`、`found=false`、`item is None` を確認する。

  合格条件:
  存在しないIDでの `get` が `200`、`success=true`、`found=false`、`item is None` を満たすこと。

  失敗時の示唆:
  lambda-dynamo実装、ScyllaDB接続情報、テーブル初期化状態を確認する。

- <a id="desc-standard-gateway-basics-health"></a>`STD-005`

  保証:
  Gatewayの健全性応答が `status=\"healthy\"` で返ることを保証する。

  入力:
  `GET /health` が `200` を返し、レスポンス JSON の `status` が `healthy` であることを確認する。

  合格条件:
  `GET /health` が `200` を返し、レスポンスJSONの `status` が `healthy` であること。

  失敗時の示唆:
  Gateway認証/ルーティングミドルウェア、Bearerトークン処理、HTTPエラーハンドリングを確認する。

- <a id="desc-standard-gateway-basics-auth"></a>`STD-006`

  保証:
  E2E実行に必要な認証トークンが取得できることを保証する。

  入力:
  認証フィクスチャから取得したトークンが `None` ではなく、空文字でもないことを確認し、後続 API テストの前提を検証する。

  合格条件:
  取得した認証トークンが `None` ではなく、文字列長が1以上であること。

  失敗時の示唆:
  Gateway認証/ルーティングミドルウェア、Bearerトークン処理、HTTPエラーハンドリングを確認する。

- <a id="desc-standard-gateway-basics-routing-401"></a>`STD-007`

  保証:
  未認証アクセスが適切に拒否されることを保証する。

  入力:
  認証ヘッダなしで `/api/echo` を呼び出し、`401` が返ることを確認する。

  合格条件:
  認証ヘッダなしの `/api/echo` が `401` を返すこと。

  失敗時の示唆:
  Gateway認証/ルーティングミドルウェア、Bearerトークン処理、HTTPエラーハンドリングを確認する。

- <a id="desc-standard-gateway-basics-routing-404"></a>`STD-008`

  保証:
  未定義ルートアクセスが適切に404で処理されることを保証する。

  入力:
  有効トークンで未定義ルート `/api/nonexistent` を呼び出し、`404` が返ることを確認する。

  合格条件:
  有効トークン付きの未定義ルート `/api/nonexistent` が `404` を返すこと。

  失敗時の示唆:
  Gateway認証/ルーティングミドルウェア、Bearerトークン処理、HTTPエラーハンドリングを確認する。

- <a id="desc-standard-id-specs-id-propagation-with-chain"></a>`STD-009`

  保証:
  チェーン実行時のtrace伝播とrequest_id独立性が保たれることを保証する。

  入力:
  任意の `X-Amzn-Trace-Id` を付与してチェーン呼び出しを実行し、Gateway・lambda-integration・lambda-echo のログすべてで同一 trace root が観測されることを確認する。さらに両 Lambda の `aws_request_id` が UUID 形式かつ互いに重複しないことを検証する。

  合格条件:
  3コンポーネント（Gateway / lambda-integration / lambda-echo）のログで同一trace rootが観測されること。`aws_request_id` は両Lambdaで取得でき、全てUUID形式かつ両集合が重複しないこと。

  失敗時の示唆:
  trace header受け渡し、VictoriaLogs取り込み、`aws_request_id` 生成元を確認する。

- <a id="desc-standard-image-function-basic"></a>`STD-010`

  保証:
  Image関数の起動成功系/失敗系シグナルが仕様通りであることを保証する。

  入力:
  `/api/image` を最大20回まで再試行する。通常モードでは `200`（レスポンスに `success` が含まれる場合は `success=true`）を確認する。失敗期待モードでは `5xx` と `IMAGE_*` 系エラーコードを確認する。

  合格条件:
  通常モードではレスポンスが `200` で、`success` フィールドが存在する場合は `success=true` を満たすこと。失敗期待モードでは `5xx` かつ本文に `IMAGE_PULL_FAILED` / `IMAGE_AUTH_FAILED` / `IMAGE_PUSH_FAILED` / `IMAGE_DIGEST_MISMATCH` のいずれかを含むこと。

  失敗時の示唆:
  image pull/auth/push設定、レジストリ資格情報、エラーコード返却処理を確認する。

- <a id="desc-standard-image-function-chain-invoke"></a>`STD-027`

  保証:
  ImageUri指定の Lambda から他関数（`lambda-echo`）へのチェーン呼び出しが成立することを保証する。

  入力:
  `/api/image` に `{"action":"chain_invoke","target":"lambda-echo","message":"from-image-chain"}` を送信し、`200`、`success=true` を確認する。さらに `chain.status_code==200`、`child.statusCode==200`、`child.body` 展開後に `success=true` と `message="Echo: from-image-chain"` を検証する。

  合格条件:
  親レスポンスが `200` かつ `success=true`。`chain` が object で `status_code==200`。`child` が object で `statusCode==200`。`child.body` 展開後に `success=true` と `message="Echo: from-image-chain"` が成立すること。

  失敗時の示唆:
  Image Lambda 内の invoke 経路、`lambda-echo` 連携、親子レスポンス整形を確認する。

- <a id="desc-standard-image-function-s3-access"></a>`STD-028`

  保証:
  ImageUri指定の Lambda から S3 へ書き込み・読み戻し（roundtrip）できることを保証する。

  入力:
  `/api/image` に `{"action":"s3_roundtrip","bucket":"e2e-test-bucket","key":"image-<random>.txt","content":"from-image-s3"}` を送信し、`200`、`success=true` を確認する。返却 `s3` オブジェクトの `bucket`、`key`、`content` が入力と一致することを検証する。

  合格条件:
  レスポンスが `200` かつ `success=true`。`s3` が object で `bucket=="e2e-test-bucket"`、`key` が送信値一致、`content=="from-image-s3"` を満たすこと。

  失敗時の示唆:
  Image Lambda 内の S3 クライアント設定、バケット名解決、sitecustomize 経由のエンドポイント誘導を確認する。

- <a id="desc-standard-image-function-victorialogs"></a>`STD-029`

  保証:
  ImageUri指定の Lambda から CloudWatch 互換ログ出力が VictoriaLogs に取り込まれることを保証する。

  入力:
  `/api/image` に `{"action":"test_cloudwatch","marker":"image-cloudwatch-<uuid>"}` を送信し、`200`、`success=true` を確認する。返却された `cloudwatch.log_group` と `cloudwatch.log_stream` をキーに VictoriaLogs を検索し、marker を含むログを1件以上検出する。

  合格条件:
  レスポンスが `200` かつ `success=true`。`cloudwatch` が object で `log_group`/`log_stream` が非空文字列。VictoriaLogs検索（`container_name=lambda-image`, `logger=boto3.mock`, 該当 log_group/log_stream）で marker を含むヒットを1件以上得られること。

  失敗時の示唆:
  CloudWatch passthrough 実装、VictoriaLogs 取り込みパイプライン、ログフィルタ条件を確認する。

- <a id="desc-standard-lambda-sync-chain-invoke"></a>`STD-011`

  保証:
  同期チェーン呼び出しで親子レスポンス整合が取れていることを保証する。

  入力:
  `/api/lambda` の同期チェーン呼び出しで `200`、`success=true` を確認する。`child.statusCode==200` かつ `child.body` の `success=true` と `message="Echo: from-chain"` を検証する。

  合格条件:
  `/api/lambda` が `200` かつ `success=true`。`child.statusCode==200`。`child.body` を展開した結果で `success=true` と `message="Echo: from-chain"` が成立すること。

  失敗時の示唆:
  lambda-integration/lambda-echoの呼び出し経路、childレスポンス整形、非同期キュー処理を確認する。

- <a id="desc-standard-lambda-async-chain-invoke"></a>`STD-012`

  保証:
  非同期チェーン呼び出しで受付応答と実行ログ追跡が成立することを保証する。

  入力:
  非同期チェーン呼び出しで `child.status="async-started"`、`child.status_code=202` を確認する。返却 `trace_id` をキーに VictoriaLogs を検索し、子関数実行ログに `Echo: from-chain` が出ることを確認する。

  合格条件:
  非同期呼び出しの応答が `200` かつ `success=true`。`child.status=="async-started"` と `child.status_code==202`。返却trace_id由来のVictoriaLogs検索で `lambda-echo` の `Echo: from-chain` ログを1件以上検出すること。

  失敗時の示唆:
  lambda-integration/lambda-echoの呼び出し経路、childレスポンス整形、非同期キュー処理を確認する。

- <a id="desc-standard-metrics-api"></a>`STD-013`

  保証:
  Gateway metrics APIがruntime差異（200/501）を含め一貫した契約で応答することを保証する。

  入力:
  事前呼び出し後に `/metrics/pools` を検証し、対象 pool の必須フィールド存在を確認する。`/metrics/containers` は `200` の場合に `state`・`memory_max`・`memory_current`・`cpu_usage_ns` を検証し、`501` の場合はエラー本文に `metrics are not implemented` または `docker` を含むことを確認する。

  合格条件:
  `/metrics/pools` が `200` で対象poolが存在し、必須フィールド（function_name, total_workers, idle, busy, provisioning, max_capacity, min_capacity, acquire_timeout）を持つこと。`/metrics/containers` は `200` の場合 `lambda-echo` エントリで `state in {RUNNING,PAUSED}`、`memory_max==134217728`、`memory_current>=0`、`cpu_usage_ns>=0`。`501` の場合はエラー本文に `metrics are not implemented` または `docker` を含むこと。

  失敗時の示唆:
  Gateway metrics実装、runtime別分岐（200/501）、メモリ上限設定値を確認する。

- <a id="desc-standard-prometheus-metrics-endpoint"></a>`STD-014`

  保証:
  AgentのPrometheusメトリクスが公開され主要メトリクス名が露出することを保証する。

  入力:
  Agent の `/metrics` に接続し、`200` と `Content-Type: text/plain` を確認する。加えて `go_*` と `grpc_server_*` の代表メトリクス名が出力に含まれることを検証する。

  合格条件:
  `/metrics` が `200` かつ `Content-Type` が `text/plain` で始まること。本文に `go_goroutines`、`go_memstats_alloc_bytes`、`grpc_server_handled_total`、`grpc_server_handling_seconds_bucket` を含むこと。

  失敗時の示唆:
  Agent metrics endpoint公開設定、Prometheus exporter登録状況を確認する。

- <a id="desc-standard-reconciliation-grace-period"></a>`STD-015`

  保証:
  Gateway再起動直後でもgrace periodにより既存ワーカが過剰削除されないことを保証する。

  入力:
  `/api/echo` 成功後に Gateway を再起動し、復帰待ち後に再度 `/api/echo` が `200` で成功することを確認する。さらに追加3回の呼び出し成功を確認し、Grace Period 中にワーカーが過剰削除されないことを検証する。

  合格条件:
  初回 `/api/echo` が `200` かつ `success=true`。Gateway再起動コマンドが成功し、復帰後 `/api/echo` が `200` かつ `success=true`。追加3回の安定性確認呼び出しもすべて `200` であること。

  失敗時の示唆:
  Gateway再起動後のreconciliationタイミング、grace period設定値、worker採用(adoption)処理を確認する。

- <a id="desc-standard-resilience-orchestrator-restart-recovery"></a>`STD-016`

  保証:
  agent再起動後に呼び出し経路が自動回復することを保証する。

  入力:
  初回呼び出しでウォームアップ後、`agent` サービスを再起動する。システム安定化を待ってから `/api/echo` をリトライ付きで呼び、`200` と `success=true` で回復完了を確認する。

  合格条件:
  初回 `/api/echo` が `200` かつ `success=true`。`agent` 再起動後の呼び出しがリトライ内で `200` かつ `success=true` に復帰すること。

  失敗時の示唆:
  agent再起動手順、gRPC再接続、復帰待機時間、retry設定を確認する。

- <a id="desc-standard-resilience-gateway-cache-hit"></a>`STD-017`

  保証:
  連続呼び出し時にgateway側worker再利用経路が破綻しないことを保証する。

  入力:
  `/api/faulty(action=hello)` を3回連続実行し、全て `200` であることを確認する。連続成功により pool の再利用（ウォーム経路）を回帰確認する。

  合格条件:
  `/api/faulty(action=hello)` の3回連続呼び出しがすべて `200` であること。

  失敗時の示唆:
  worker pool再利用条件、faulty関数のウォーム経路、セッション再利用を確認する。

- <a id="desc-standard-resilience-circuit-breaker"></a>`STD-018`

  保証:
  連続障害でcircuit breakerが開き、待機後に閉じる回復シーケンスを保証する。

  入力:
  `action=crash` を3回実行して `502` を確認し、その後4回目を `action=hello` で呼んだ際に `502` かつ低遅延（1秒未満）で fail-fast することを確認する。待機後の再実行で `200` に復帰することを検証する。

  合格条件:
  `action=crash` を3回実行して毎回 `502`。4回目 `action=hello` も `502` かつ応答時間 < 1.0秒（fail-fast）。11秒待機後の `action=hello` が `200` に回復すること。

  失敗時の示唆:
  circuit breaker閾値・開閉時間、error分類、fail-fast分岐を確認する。

- <a id="desc-standard-s3-put-get"></a>`STD-019`

  保証:
  S3互換のput/get基本整合を保証する。

  入力:
  ランダムキーに `put` した後 `get` を実行し、両方 `200`、`success=true`、取得 `content` が投入文字列と一致することを確認する。

  合格条件:
  `put` と `get` がともに `200` かつ `success=true`。`get` の `content` が投入文字列と完全一致すること。

  失敗時の示唆:
  RustFS/S3互換API、バケット存在、キー操作ロジックを確認する。

- <a id="desc-standard-s3-list-objects"></a>`STD-020`

  保証:
  S3一覧取得APIが利用可能でオブジェクトリストを返すことを保証する。

  入力:
  `list` を実行し、`200`、`success=true`、レスポンスに `objects` フィールドが含まれることを確認する。

  合格条件:
  `list` が `200` かつ `success=true`。レスポンスに `objects` フィールドが存在すること。

  失敗時の示唆:
  RustFS/S3互換API、バケット存在、キー操作ロジックを確認する。

- <a id="desc-standard-s3-delete-object"></a>`STD-021`

  保証:
  S3 delete後に対象キーが取得不能になることを保証する。

  入力:
  `put -> delete -> get` を実行し、`delete` が `200`・`success=true` で成功することを確認する。削除後 `get` が `500`（NoSuchKey 相当）になることを確認する。

  合格条件:
  `delete` が `200` かつ `success=true`。削除後 `get` が `500`（NoSuchKey相当）になること。

  失敗時の示唆:
  RustFS/S3互換API、バケット存在、キー操作ロジックを確認する。

- <a id="desc-standard-s3-overwrite"></a>`STD-022`

  保証:
  S3同一キー上書き時に後勝ちデータが返ることを保証する。

  入力:
  同一キーに対して2回 `put`（上書き）した後 `get` を実行し、返却 `content` が後勝ちの `"overwritten"` になることを確認する。

  合格条件:
  同一キー上書き後の `get` が `200` で、`content == "overwritten"` を満たすこと。

  失敗時の示唆:
  RustFS/S3互換API、バケット存在、キー操作ロジックを確認する。

- <a id="desc-standard-s3-list-with-prefix"></a>`STD-023`

  保証:
  S3 prefix指定一覧のフィルタ経路が成立することを保証する。

  入力:
  同一プレフィックス下に複数オブジェクトを作成してから `prefix` 指定で `list` を実行し、`200` と `success=true` を確認する。

  合格条件:
  プレフィックス付き `list` が `200` かつ `success=true` を返すこと。

  失敗時の示唆:
  RustFS/S3互換API、バケット存在、キー操作ロジックを確認する。

- <a id="desc-standard-s3-bucket-lifecycle-configuration"></a>`STD-024`

  保証:
  S3バケットライフサイクル設定で `Status=Enabled` かつ `Expiration.Days=7` のルールが有効であることを保証する。

  入力:
  S3 クライアントで `get_bucket_lifecycle_configuration` を実行し、ルールが1件以上存在することを確認する。さらに `Status=Enabled` かつ `Expiration.Days=7` のルールが存在することを検証する。

  合格条件:
  `get_bucket_lifecycle_configuration` の結果で `Rules` が1件以上あり、`Status==Enabled` かつ `Expiration.Days==7` のルールを少なくとも1件含むこと。

  失敗時の示唆:
  RustFS/S3互換API、バケット存在、キー操作ロジックを確認する。

- <a id="desc-standard-scheduler-schedule-trigger"></a>`STD-025`

  保証:
  スケジューラが定期実行し、下流DynamoDBへ実行結果を書き込むことを保証する。

  入力:
  最大90秒ポーリングして `scheduled-run` レコードの出現を待ち、検出時に `event.scheduled.BOOL is True` を確認する。時間内に検出できなければ失敗とする。

  合格条件:
  最大90秒の待機中に `scheduled-run` レコードを検出できること。検出時に `event.scheduled.BOOL is True` が成立すること。

  失敗時の示唆:
  scheduler起動状態、定期トリガ設定、DynamoDB書き込み権限を確認する。

- <a id="desc-standard-trace-propagation-chained-trace-consistency"></a>`STD-026`

  保証:
  chained invokeでtrace rootが3コンポーネントへ欠落なく伝播することを保証する。

  入力:
  カスタム Trace ID を付与して `lambda-integration -> lambda-connectivity` を呼び出し、Lambda A レスポンス内 trace の Root 一致を確認する。VictoriaLogs を開始時刻以降で検索し、`esb-gateway`・`lambda-integration`・`lambda-connectivity` の全コンポーネントで同 Trace が観測されることを検証する。

  合格条件:
  チェーン呼び出し応答が `200`。Lambda A応答の `trace_id` が存在し `not-found` ではなく、送信したrootと一致すること。VictoriaLogsで `esb-gateway` / `lambda-integration` / `lambda-connectivity` 全てに同traceが出現すること。

  失敗時の示唆:
  trace header注入、Gateway/Lambdaログ転送、VictoriaLogs検索条件を確認する。


### runtime/java

- <a id="desc-runtime-java-echo-basic"></a>`RTJ-001`

  保証:
  Java echo runtimeの基本応答契約（status/message/user）を保証する。

  入力:
  `/api/echo-java` 呼び出しで `200`、`success=true`、`message="Echo: hello-java"`、`user==AUTH_USER` を確認する。

  合格条件:
  `/api/echo-java` が `200` かつ `success=true`、`message="Echo: hello-java"`、`user==AUTH_USER` を満たすこと。

  失敗時の示唆:
  Java runtimeイメージ、`/api/echo-java` ルーティング、認証ユーザー注入を確認する。

- <a id="desc-runtime-java-echo-logs-and-trace"></a>`RTJ-002`

  保証:
  Java runtimeログがtrace付きで出力され、構造化項目とレベルが揃うことを保証する。

  入力:
  Trace ID 付きで Java Echo を実行し、`lambda-echo-java` のログを最小2件取得する。ログ群に Echo 文言、`DEBUG` レベル、`_time` フィールドが含まれることを確認する。

  合格条件:
  Trace ID付き呼び出しが `200`。`lambda-echo-java` の該当ログを2件以上取得し、`Echo: Log quality test` を含むログ、`DEBUG` レベルログ、`_time` フィールドを確認できること。

  失敗時の示唆:
  Javaログフォーマッタ、trace_id埋め込み、VictoriaLogs転送設定を確認する。

- <a id="desc-runtime-java-cloudwatch-logs-passthrough"></a>`RTJ-003`

  保証:
  Java CloudWatch擬似ログがVictoriaLogsへ正しく転送・属性付与されることを保証する。

  入力:
  Java connectivity の CloudWatch 模擬呼び出しで `success=true` と `log_group/log_stream` を取得し、VictoriaLogs 側で該当ログが4件以上あることを確認する。全件 `container_name=lambda-connectivity-java`、かつ `DEBUG/INFO/ERROR` を含み、`CloudWatch Logs E2E verification successful!` を含むログがあることを検証する。

  合格条件:
  CloudWatch模擬呼び出しが `200` かつ `success=true`、`log_group/log_stream` を返すこと。VictoriaLogsで該当ログ4件以上、全件 `container_name=lambda-connectivity-java`、レベルに `DEBUG/INFO/ERROR` を含み、`CloudWatch Logs E2E verification successful!` を含むこと。

  失敗時の示唆:
  Java側CloudWatchモック処理、ログ転送先、container_name付与を確認する。


### runtime/python

- <a id="desc-runtime-python-echo-basic"></a>`RTP-001`

  保証:
  Python echo runtimeの基本応答契約（status/message/user）を保証する。

  入力:
  `/api/echo` 呼び出しで `200`、`success=true`、`message="Echo: hello-basic"`、`user==AUTH_USER` を確認する。

  合格条件:
  `/api/echo` が `200` かつ `success=true`、`message="Echo: hello-basic"`、`user==AUTH_USER` を満たすこと。

  失敗時の示唆:
  Python runtimeイメージ、`/api/echo` ルーティング、認証ユーザー注入を確認する。

- <a id="desc-runtime-python-structured-log-format"></a>`RTP-002`

  保証:
  Python runtimeログの構造化形式・DEBUGレベル・コンテナ名付与を保証する。

  入力:
  Trace ID 付き呼び出し後、ログをポーリングして構造化フィールド（`level` と `message/_msg`）、`_time`、`DEBUG` ログを確認する。さらに `container_name="UNKNOWN"` のログが出ていないことを検証する。

  合格条件:
  Trace ID付き呼び出し後、タイムアウト内に構造化ログ（`level` と `message`/`_msg`）・有効な `_time`・`DEBUG` レベルを確認できること。`container_name="UNKNOWN"` のログが0件であること。

  失敗時の示唆:
  Pythonログ出力設定(LOG_LEVEL含む)、構造化フォーマット、container_name注入を確認する。

- <a id="desc-runtime-python-cloudwatch-logs-passthrough"></a>`RTP-003`

  保証:
  Python CloudWatch擬似ログがVictoriaLogsへ重複なく転送され、属性付与されることを保証する。

  入力:
  Python connectivity の CloudWatch 模擬呼び出し後、`logger:boto3.mock` かつ `log_group/log_stream` 条件で VictoriaLogs を検索し、該当ログがちょうど4件であることを確認する。さらに、`(level, _msg)` の組が4件すべて一意であること（重複なし）を検証する。全件 `container_name=lambda-connectivity`、`DEBUG/INFO/ERROR` レベルを含み、`CloudWatch Logs E2E verification successful!` を含むことを確認する。

  合格条件:
  CloudWatch模擬呼び出しが `200` かつ `success=true`。VictoriaLogs検索で該当ログがちょうど4件であること。`(level, _msg)` の組が4件すべて一意であること。全件 `container_name=lambda-connectivity`。レベルに `DEBUG/INFO/ERROR` を含み、`CloudWatch Logs E2E verification successful!` を含むこと。

  失敗時の示唆:
  Python側CloudWatchモック処理、ログ転送先、container_name付与を確認する。


### restart

- <a id="desc-restart-service-process-crash-recovers"></a>`RST-001`

  保証:
  gateway/agentプロセスクラッシュ後にDockerの自動再起動と復旧後疎通が成立することを保証する。

  入力:
  `gateway`/`agent` ごとに実行し、事前疎通後に `docker inspect` で `RestartPolicy.Name=="unless-stopped"` を確認する。対象プロセスを `TERM` で停止し、`RestartCount` 増加と `StartedAt` 更新を待って再起動を検証する。復帰後に再度 `/api/echo` が `200` で成功することを確認する。

  合格条件:
  `gateway` / `agent` それぞれで、事前 `/api/echo` が `200` かつ `success=true`。`RestartPolicy.Name=="unless-stopped"`。プロセス停止後に `RestartCount` が増加し `StartedAt` が更新され、状態が `running` に戻ること。復帰後 `/api/echo` が再び `200` かつ `success=true` であること。

  失敗時の示唆:
  Docker restart policy(`unless-stopped`)、プロセス停止シグナル、再起動検知(`RestartCount/StartedAt`)を確認する。



---


## Implementation references
- `e2e/environments/test_matrix.yaml`
- `e2e/scenarios/`
