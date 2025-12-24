"""
DynamoDB 互換テスト (ScyllaDB)

- DynamoDB API の互換性検証
- ScyllaDB バックエンドでの動作確認
"""

import time
import uuid

import pytest

from tests.fixtures.conftest import (
    SCYLLA_WAIT_INTERVAL,
    SCYLLA_WAIT_RETRIES,
    call_api,
)


class TestDynamo:
    """DynamoDB 互換性の検証"""

    def test_put_get(self, auth_token):
        """E2E: DynamoDB PutItem/GetItem 互換テスト (ScyllaDB)"""
        max_retries = SCYLLA_WAIT_RETRIES
        response = None

        for i in range(max_retries):
            response = call_api(
                "/api/dynamo",
                auth_token,
                {"action": "put_get"},
            )

            if response.status_code == 200:
                break

            print(f"Status: {response.status_code}, Body: {response.text}")
            if response.status_code not in [500, 502, 503, 504]:
                break

            print(f"Waiting for Lambda/ScyllaDB... ({i + 1}/{max_retries})")
            time.sleep(SCYLLA_WAIT_INTERVAL)

        if response is None:
            pytest.fail("Lambda integration failed: No response received")

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert "item_id" in data
        assert "retrieved_item" in data
        assert data["retrieved_item"]["id"]["S"] == data["item_id"]

    def test_update_item(self, auth_token):
        """E2E: DynamoDB UpdateItem 互換テスト"""
        # 1. PutItem
        put_response = call_api(
            "/api/dynamo",
            auth_token,
            {"action": "put", "message": "Original message"},
        )
        assert put_response.status_code == 200
        item_id = put_response.json()["item_id"]

        # 2. UpdateItem
        update_response = call_api(
            "/api/dynamo",
            auth_token,
            {"action": "update", "id": item_id, "message": "Updated message"},
        )
        assert update_response.status_code == 200
        assert update_response.json()["success"] is True

        # 3. GetItem → 更新を確認
        get_response = call_api(
            "/api/dynamo",
            auth_token,
            {"action": "get", "id": item_id},
        )
        assert get_response.status_code == 200
        data = get_response.json()
        assert data["found"] is True
        assert data["item"]["message"]["S"] == "Updated message"

    def test_delete_item(self, auth_token):
        """E2E: DynamoDB DeleteItem 互換テスト"""
        # 1. PutItem
        put_response = call_api(
            "/api/dynamo",
            auth_token,
            {"action": "put", "message": "To be deleted"},
        )
        assert put_response.status_code == 200
        item_id = put_response.json()["item_id"]

        # 2. DeleteItem
        delete_response = call_api(
            "/api/dynamo",
            auth_token,
            {"action": "delete", "id": item_id},
        )
        assert delete_response.status_code == 200
        assert delete_response.json()["deleted"] is True

        # 3. GetItem → 見つからないことを確認
        get_response = call_api(
            "/api/dynamo",
            auth_token,
            {"action": "get", "id": item_id},
        )
        assert get_response.status_code == 200
        data = get_response.json()
        assert data["found"] is False

    def test_get_nonexistent(self, auth_token):
        """E2E: DynamoDB 存在しないアイテム取得テスト"""
        fake_id = str(uuid.uuid4())

        response = call_api(
            "/api/dynamo",
            auth_token,
            {"action": "get", "id": fake_id},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["found"] is False
        assert data["item"] is None
