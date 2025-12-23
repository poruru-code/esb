"""
Lambda 呼び出しテスト

- 基本的な Lambda 呼び出し
- 同期関数間呼び出し
- 非同期関数間呼び出し
"""

import json
import time
import requests
import pytest
from tests.fixtures.conftest import (
    GATEWAY_URL,
    VERIFY_SSL,
    AUTH_USER,
    ASYNC_WAIT_RETRIES,
    get_auth_token,
)


class TestLambdaInvoke:
    """Lambda 呼び出し機能の検証"""

    def test_lambda_invocation(self, gateway_health):
        """E2E: 認証 → ルーティング → Lambda呼び出し"""
        token = get_auth_token()

        response = requests.post(
            f"{GATEWAY_URL}/api/s3/test",
            json={"action": "test", "bucket": "e2e-test-bucket"},
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )

        # Lambda RIEが起動していない場合は502になる可能性がある
        if response.status_code == 502:
            pytest.skip("Lambda RIE not available in Gateway container")

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["user"] == AUTH_USER

    def test_function_invocation_sync(self, gateway_health):
        """E2E: 同期呼び出し検証 (invoke-test -> hello)"""
        token = get_auth_token()

        response = requests.post(
            f"{GATEWAY_URL}/api/invoke/test",
            json={
                "target": "lambda-hello",
                "type": "RequestResponse",
            },
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )

        # Check invoke-test execution
        if response.status_code != 200:
            print(f"Sync Invoke Failed: {response.text}")
        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        # Check inner response (hello function)
        inner_resp = data["response"]
        assert inner_resp["statusCode"] == 200
        inner_body = json.loads(inner_resp["body"])
        assert "Hello" in inner_body["message"]

    def test_function_invocation_async(self, gateway_health):
        """E2E: 非同期呼び出し検証 (invoke-test -> s3-test)"""
        token = get_auth_token()
        bucket = "async-test-bucket"
        key = f"test-{int(time.time())}.txt"

        # 1. Create bucket (Sync)
        requests.post(
            f"{GATEWAY_URL}/api/s3/test",
            json={"action": "create_bucket", "bucket": bucket},
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )

        # 2. Invoke Async (invoke-test -> s3-test)
        response = requests.post(
            f"{GATEWAY_URL}/api/invoke/test",
            json={
                "target": "lambda-s3-test",
                "type": "Event",
                "payload": {
                    "body": {"action": "put", "bucket": bucket, "key": key, "data": "Async Data"}
                },
            },
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        # Async invocation returns 202 from Gateway to invoke-test
        assert data["status_code"] == 202

        # 3. Verify Side Effect (Poll S3)
        print("Waiting for async execution...")
        time.sleep(2)  # Initial wait

        max_retries = ASYNC_WAIT_RETRIES
        found = False
        for i in range(max_retries):
            check_resp = requests.post(
                f"{GATEWAY_URL}/api/s3/test",
                json={"action": "get", "bucket": bucket, "key": key},
                headers={"Authorization": f"Bearer {token}"},
                verify=VERIFY_SSL,
            )
            check_data = check_resp.json()
            if check_data.get("success") is True:
                found = True
                assert check_data["content"] == "Async Data"
                break
            time.sleep(1)

        assert found, "Async execution failed: File not found in S3"
