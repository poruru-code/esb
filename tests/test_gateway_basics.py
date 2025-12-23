"""
Gateway 基本機能テスト

- ヘルスチェック
- 認証フロー
- 基本的なルーティング (401, 404)
"""

import requests
from tests.fixtures.conftest import (
    GATEWAY_URL,
    VERIFY_SSL,
    get_auth_token,
)


class TestGatewayBasics:
    """Gateway 基本機能の検証"""

    def test_health(self, gateway_health):
        """E2E: ヘルスチェック"""
        response = requests.get(f"{GATEWAY_URL}/health", verify=VERIFY_SSL)
        assert response.status_code == 200
        assert response.json()["status"] == "healthy"

    def test_auth(self, gateway_health):
        """E2E: 認証フロー"""
        token = get_auth_token()
        assert token is not None
        assert len(token) > 0

    def test_routing_401(self, gateway_health):
        """E2E: 認証なし → 401"""
        response = requests.post(
            f"{GATEWAY_URL}/api/s3/test",
            json={"action": "test", "bucket": "e2e-test-bucket"},
            verify=VERIFY_SSL,
        )
        if response.status_code != 401:
            print(f"Debug 401 Error: {response.status_code} - {response.text}")
        assert response.status_code == 401

    def test_routing_404(self, gateway_health):
        """E2E: 存在しないルート → 404"""
        token = get_auth_token()
        response = requests.post(
            f"{GATEWAY_URL}/api/nonexistent",
            json={"action": "test"},
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )
        assert response.status_code == 404
