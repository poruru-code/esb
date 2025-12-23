"""
AWS サービス互換テスト

- DynamoDB 互換 (ScyllaDB)
"""

import time
import requests
import pytest
from tests.fixtures.conftest import (
    GATEWAY_URL,
    VERIFY_SSL,
    SCYLLA_WAIT_RETRIES,
    SCYLLA_WAIT_INTERVAL,
    get_auth_token,
)


class TestAWSCompat:
    """AWS サービス互換性の検証"""

    def test_scylla_integration(self, gateway_health):
        """E2E: ScyllaDB連携テスト"""
        token = get_auth_token()

        # ScyllaDBの起動待ち（Lambdaが起動するまでリトライ）
        # WindowsのDocker Desktop (WSL2) ではScyllaDBの起動に3-5分かかる場合がある
        max_retries = SCYLLA_WAIT_RETRIES
        response = None

        for i in range(max_retries):
            try:
                response = requests.post(
                    f"{GATEWAY_URL}/api/scylla/test",
                    json={"action": "test", "bucket": "e2e-test-bucket"},
                    headers={"Authorization": f"Bearer {token}"},
                    verify=VERIFY_SSL,
                )

                if response.status_code == 200:
                    break

                print(f"Status: {response.status_code}, Body: {response.text}")

                # 500 (Application Error/DB Not Ready) or 502 (Bad Gateway) -> Retry
                if response.status_code not in [500, 502, 503, 504]:
                    break

            except requests.exceptions.ConnectionError:
                print(f"Connection error (Gateway restarting?)... ({i + 1}/{max_retries})")
                response = None  # Reset response

            print(f"Waiting for Lambda/ScyllaDB... ({i + 1}/{max_retries})")
            time.sleep(SCYLLA_WAIT_INTERVAL)

        if response is None:
            pytest.fail("Lambda integration failed: No response received")

        if response.status_code != 200:
            print(f"Final Failure Response: {response.text}")

        assert response.status_code == 200
        data = response.json()
        print(f"Response Data: {data}")
        assert data["success"] is True
        assert "item_id" in data
        assert "retrieved_item" in data
        assert data["retrieved_item"]["id"]["S"] == data["item_id"]
