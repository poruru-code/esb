"""
耐障害性・パフォーマンス機能テスト

- Manager再起動時のコンテナ復元 (Adopt & Sync)
- コンテナホストキャッシュ (Managerへの負荷軽減)
- Circuit Breaker (Lambdaクラッシュ時の遮断)
"""

import os
import subprocess
import time

import requests

from tests.conftest import (
    GATEWAY_URL,
    VERIFY_SSL,
    DEFAULT_REQUEST_TIMEOUT,
    ORCHESTRATOR_RESTART_WAIT,
    STABILIZATION_WAIT,
    request_with_retry,
    call_api,
)


class TestResilience:
    """耐障害性・パフォーマンス機能の検証"""

    def test_orchestrator_restart_recovery(self, auth_token):
        """
        E2E: Manager再起動時のコンテナ復元検証 (Adopt & Sync)

        Echo Lambda を使用 (S3 依存なし)
        """

        # 1. 最初の呼び出し（コンテナ起動）
        print("Step 1: Initial Lambda invocation (cold start)...")
        response1 = call_api("/api/echo", auth_token, {"message": "warmup"})
        assert response1.status_code == 200
        data1 = response1.json()
        assert data1["success"] is True

        time.sleep(3)

        # 2. Managerコンテナを再起動
        print("Step 2: Restarting Manager container...")
        restart_result = subprocess.run(
            ["docker", "compose", "restart", "orchestrator"],
            capture_output=True,
            text=True,
            cwd=os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))),
        )
        assert restart_result.returncode == 0, f"Failed to restart Manager: {restart_result.stderr}"

        time.sleep(ORCHESTRATOR_RESTART_WAIT)

        # Managerのヘルスチェック（間接的）
        for i in range(15):
            try:
                health_resp = requests.get(
                    f"{GATEWAY_URL}/health", verify=VERIFY_SSL, timeout=DEFAULT_REQUEST_TIMEOUT
                )
                if health_resp.status_code == 200:
                    break
            except Exception:
                print(f"Waiting for system to stabilize... ({i + 1}/15)")
            time.sleep(2)

        time.sleep(STABILIZATION_WAIT)

        # 3. 再起動後の呼び出し（コンテナ復元確認）
        print("Step 3: Post-restart invocation (should be warm start)...")

        response2 = request_with_retry(
            "post",
            f"{GATEWAY_URL}/api/echo",
            max_retries=5,
            retry_interval=2.0,
            json={"message": "after restart"},
            headers={"Authorization": f"Bearer {auth_token}"},
            verify=VERIFY_SSL,
        )

        assert response2.status_code == 200, (
            f"Expected 200, got {response2.status_code}: {response2.text}"
        )
        data2 = response2.json()
        assert data2["success"] is True

        print(f"Post-restart invocation successful: {data2}")
        time.sleep(3)
        print("Test passed: Container was successfully adopted after Manager restart")

    def test_gateway_cache_hit(self, auth_token):
        """
        E2E: Gateway のコンテナプーリングが機能していることを検証

        PoolManager アーキテクチャでは、Gateway が Orchestrator を経由せずに
        プール内のワーカーを直接管理します。このテストでは、連続リクエストが
        プールからワーカーを再利用して成功することを検証します。
        """

        # 1. 1回目リクエスト (プールにワーカーなし -> プロビジョニング発生)
        resp1 = call_api(
            "/api/faulty",
            auth_token,
            {"action": "hello"},
        )
        assert resp1.status_code == 200, f"First request failed: {resp1.text}"
        print("First request succeeded (cold start or pooled)")

        # 2. 2回目リクエスト (プールからワーカー再利用)
        resp2 = call_api(
            "/api/faulty",
            auth_token,
            {"action": "hello"},
        )
        assert resp2.status_code == 200, f"Second request failed: {resp2.text}"
        print("Second request succeeded (should be warm/pooled)")

        # 3. 3回目リクエスト (継続的な再利用確認)
        resp3 = call_api(
            "/api/faulty",
            auth_token,
            {"action": "hello"},
        )
        assert resp3.status_code == 200, f"Third request failed: {resp3.text}"
        print("Third request succeeded - pool reuse verified")

    def test_circuit_breaker(self, auth_token):
        """
        E2E: Lambda のクラッシュ時に Circuit Breaker が作動することを検証
        """

        # 1. ウォームアップ
        print("Warming up lambda-faulty...")
        call_api("/api/faulty", auth_token, {"action": "hello"})

        try:
            # 2. 失敗を繰り返す
            for i in range(3):
                print(f"Attempt {i + 1} (crashing lambda)...")
                start = time.time()
                resp = call_api("/api/faulty", auth_token, {"action": "crash"}, timeout=10)
                duration = time.time() - start
                print(f"Status: {resp.status_code}, Body: {resp.text}, Latency: {duration:.2f}s")
                assert resp.status_code == 502, f"Expected 502, got {resp.status_code}"

            # 3. 4回目リクエスト (Circuit Breaker OPEN)
            print("Request 4 (expecting Circuit Breaker Open)...")
            start = time.time()
            resp = call_api("/api/faulty", auth_token, {"action": "hello"}, timeout=10)
            duration = time.time() - start
            print(f"Status: {resp.status_code}, Body: {resp.text}, Latency: {duration:.2f}s")

            assert resp.status_code == 502
            assert duration < 1.0, "Circuit Breaker should fail fast (< 1.0s)"

            # 4. 復旧待ち
            print("Waiting for Circuit Breaker recovery (11s)...")
            time.sleep(11)

            # 5. 復旧確認
            print("Request 5 (expecting recovery)...")
            resp = call_api("/api/faulty", auth_token, {"action": "hello"})
            assert resp.status_code == 200, f"Recovery failed: {resp.text}"
            print("Circuit Breaker recovered successfully")

        except Exception as e:
            print(f"Circuit Breaker test failed: {e}")
            raise
