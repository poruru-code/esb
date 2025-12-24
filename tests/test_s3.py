"""
S3 互換テスト (RustFS/MinIO)

- S3 API の互換性検証
- RustFS バックエンドでの動作確認
"""

import uuid

from tests.fixtures.conftest import call_api


class TestS3:
    """S3 互換性の検証"""

    def test_put_get(self, auth_token):
        """E2E: S3 PutObject/GetObject 互換テスト"""
        test_key = f"test-object-{uuid.uuid4().hex[:8]}.txt"
        test_content = "Hello from E2E test!"

        # 1. PutObject
        put_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "put", "bucket": "e2e-test-bucket", "key": test_key, "body": test_content},
        )
        assert put_response.status_code == 200, f"PutObject failed: {put_response.text}"
        assert put_response.json()["success"] is True

        # 2. GetObject
        get_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "get", "bucket": "e2e-test-bucket", "key": test_key},
        )
        assert get_response.status_code == 200, f"GetObject failed: {get_response.text}"
        get_data = get_response.json()
        assert get_data["success"] is True
        assert get_data["content"] == test_content

    def test_list_objects(self, auth_token):
        """E2E: S3 ListObjects 互換テスト"""
        response = call_api(
            "/api/s3",
            auth_token,
            {"action": "list", "bucket": "e2e-test-bucket"},
        )
        assert response.status_code == 200, f"ListObjects failed: {response.text}"
        data = response.json()
        assert data["success"] is True
        assert "objects" in data

    def test_delete_object(self, auth_token):
        """E2E: S3 DeleteObject 互換テスト"""
        test_key = f"test-delete-{uuid.uuid4().hex[:8]}.txt"

        # 1. PutObject
        call_api(
            "/api/s3",
            auth_token,
            {
                "action": "put",
                "bucket": "e2e-test-bucket",
                "key": test_key,
                "body": "to be deleted",
            },
        )

        # 2. DeleteObject
        delete_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "delete", "bucket": "e2e-test-bucket", "key": test_key},
        )
        assert delete_response.status_code == 200, f"DeleteObject failed: {delete_response.text}"
        assert delete_response.json()["success"] is True

        # 3. GetObject → エラー (NoSuchKey)
        get_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "get", "bucket": "e2e-test-bucket", "key": test_key},
        )
        assert get_response.status_code == 500  # NoSuchKey → 500 error

    def test_overwrite(self, auth_token):
        """E2E: S3 同一キー上書きテスト"""
        test_key = f"test-overwrite-{uuid.uuid4().hex[:8]}.txt"

        # 1. 初回 PutObject
        call_api(
            "/api/s3",
            auth_token,
            {"action": "put", "bucket": "e2e-test-bucket", "key": test_key, "body": "original"},
        )

        # 2. 上書き PutObject
        call_api(
            "/api/s3",
            auth_token,
            {"action": "put", "bucket": "e2e-test-bucket", "key": test_key, "body": "overwritten"},
        )

        # 3. GetObject → 上書き内容を確認
        get_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "get", "bucket": "e2e-test-bucket", "key": test_key},
        )
        assert get_response.status_code == 200
        assert get_response.json()["content"] == "overwritten"

    def test_list_with_prefix(self, auth_token):
        """E2E: S3 Prefix 付き ListObjects テスト"""
        prefix = f"prefix-test-{uuid.uuid4().hex[:8]}/"

        # テスト用オブジェクト作成
        for i in range(3):
            call_api(
                "/api/s3",
                auth_token,
                {
                    "action": "put",
                    "bucket": "e2e-test-bucket",
                    "key": f"{prefix}file{i}.txt",
                    "body": f"content{i}",
                },
            )

        # Prefix 付き ListObjects
        response = call_api(
            "/api/s3",
            auth_token,
            {"action": "list", "bucket": "e2e-test-bucket", "prefix": prefix},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert len(data["objects"]) >= 3
