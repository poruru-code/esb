"""
耐障害性・パフォーマンス機能テスト

- Manager再起動時のコンテナ復元 (Adopt & Sync)
- コンテナホストキャッシュ (Managerへの負荷軽減)
- Circuit Breaker (Lambdaクラッシュ時の遮断)
"""

import os
import subprocess
import time
import uuid

import requests

from tests.fixtures.conftest import (
    GATEWAY_URL,
    VERIFY_SSL,
    DEFAULT_REQUEST_TIMEOUT,
    MANAGER_RESTART_WAIT,
    STABILIZATION_WAIT,
    query_victorialogs,
    request_with_retry,
)


class TestResilience:
    """耐障害性・パフォーマンス機能の検証"""

    def test_manager_restart_recovery(self, auth_token):
        """
        E2E: Manager再起動時のコンテナ復元検証 (Adopt & Sync)

        Echo Lambda を使用 (S3 依存なし)
        """

        # 1. 最初の呼び出し（コンテナ起動）
        print("Step 1: Initial Lambda invocation (cold start)...")
        response1 = requests.post(
            f"{GATEWAY_URL}/api/echo",
            json={"message": "warmup"},
            headers={"Authorization": f"Bearer {auth_token}"},
            verify=VERIFY_SSL,
        )
        assert response1.status_code == 200
        data1 = response1.json()
        assert data1["success"] is True

        time.sleep(3)

        # 2. Managerコンテナを再起動
        print("Step 2: Restarting Manager container...")
        restart_result = subprocess.run(
            ["docker", "compose", "restart", "manager"],
            capture_output=True,
            text=True,
            cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
        )
        assert restart_result.returncode == 0, f"Failed to restart Manager: {restart_result.stderr}"

        time.sleep(MANAGER_RESTART_WAIT)

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
        E2E: Gateway のコンテナホストキャッシュが機能していることを検証
        """

        # 1. 1回目リクエスト (キャッシュなし -> Manager 問い合わせ発生)
        epoch_hex_1 = hex(int(time.time()))[2:]
        unique_id_1 = uuid.uuid4().hex[:24]
        trace_id_1 = f"Root=1-{epoch_hex_1}-{unique_id_1};Sampled=1"
        root_id_1 = f"1-{epoch_hex_1}-{unique_id_1}"

        resp1 = requests.post(
            f"{GATEWAY_URL}/api/faulty",
            json={"action": "hello"},
            headers={"Authorization": f"Bearer {auth_token}", "X-Amzn-Trace-Id": trace_id_1},
            verify=VERIFY_SSL,
        )
        assert resp1.status_code == 200, f"First request failed: {resp1.text}"

        # 2. 2回目リクエスト (Gateway キャッシュヒット -> Manager 問い合わせなし)
        epoch_hex_2 = hex(int(time.time()) + 1)[2:]
        unique_id_2 = uuid.uuid4().hex[:24]
        trace_id_2 = f"Root=1-{epoch_hex_2}-{unique_id_2};Sampled=1"
        root_id_2 = f"1-{epoch_hex_2}-{unique_id_2}"

        resp2 = requests.post(
            f"{GATEWAY_URL}/api/faulty",
            json={"action": "hello"},
            headers={"Authorization": f"Bearer {auth_token}", "X-Amzn-Trace-Id": trace_id_2},
            verify=VERIFY_SSL,
        )
        assert resp2.status_code == 200, f"Second request failed: {resp2.text}"

        # 3. ログを確認
        time.sleep(5)

        result_1 = query_victorialogs(root_id_1)
        logs_1 = result_1.get("hits", [])
        manager_req_1 = [
            log_entry for log_entry in logs_1 if "manager.main" in str(log_entry.get("logger", ""))
        ]

        result_2 = query_victorialogs(root_id_2)
        logs_2 = result_2.get("hits", [])
        manager_req_2 = [
            log_entry for log_entry in logs_2 if "manager.main" in str(log_entry.get("logger", ""))
        ]

        print(f"Initial Manager Logs: {len(manager_req_1)}")
        print(f"Second Manager Logs: {len(manager_req_2)}")

        assert len(manager_req_1) > 0, "Initial request must involve Manager"
        assert len(manager_req_2) == 0, "Second request should use Gateway cache and SKIP Manager"

    def test_circuit_breaker(self, auth_token):
        """
        E2E: Lambda のクラッシュ時に Circuit Breaker が作動することを検証
        """

        # 1. ウォームアップ
        print("Warming up lambda-faulty...")
        requests.post(
            f"{GATEWAY_URL}/api/faulty",
            json={"action": "hello"},
            headers={"Authorization": f"Bearer {auth_token}"},
            verify=VERIFY_SSL,
        )

        try:
            # 2. 失敗を繰り返す
            for i in range(3):
                print(f"Attempt {i + 1} (crashing lambda)...")
                start = time.time()
                resp = requests.post(
                    f"{GATEWAY_URL}/api/faulty",
                    json={"action": "crash"},
                    headers={"Authorization": f"Bearer {auth_token}"},
                    verify=VERIFY_SSL,
                    timeout=10,
                )
                duration = time.time() - start
                print(f"Status: {resp.status_code}, Body: {resp.text}, Latency: {duration:.2f}s")
                assert resp.status_code == 502, f"Expected 502, got {resp.status_code}"

            # 3. 4回目リクエスト (Circuit Breaker OPEN)
            print("Request 4 (expecting Circuit Breaker Open)...")
            start = time.time()
            resp = requests.post(
                f"{GATEWAY_URL}/api/faulty",
                json={"action": "hello"},
                headers={"Authorization": f"Bearer {auth_token}"},
                verify=VERIFY_SSL,
                timeout=10,
            )
            duration = time.time() - start
            print(f"Status: {resp.status_code}, Body: {resp.text}, Latency: {duration:.2f}s")

            assert resp.status_code == 502
            assert duration < 1.0, "Circuit Breaker should fail fast (< 1.0s)"

            # 4. 復旧待ち
            print("Waiting for Circuit Breaker recovery (11s)...")
            time.sleep(11)

            # 5. 復旧確認
            print("Request 5 (expecting recovery)...")
            resp = requests.post(
                f"{GATEWAY_URL}/api/faulty",
                json={"action": "hello"},
                headers={"Authorization": f"Bearer {auth_token}"},
                verify=VERIFY_SSL,
            )
            assert resp.status_code == 200, f"Recovery failed: {resp.text}"
            print("Circuit Breaker recovered successfully")

        except Exception as e:
            print(f"Circuit Breaker test failed: {e}")
            raise
