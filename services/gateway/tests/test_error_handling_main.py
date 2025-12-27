from fastapi.testclient import TestClient
from services.gateway.main import app
from services.gateway.core.exceptions import ResourceExhaustedError
from unittest.mock import patch


def test_resource_exhausted_error_returns_429():
    """
    ResourceExhaustedError が発生したときに HTTP 429 が返されることを確認する。
    """
    # 既存のエンドポイント (例: /health) で例外を強制的に発生させる
    # 実装前（RED）なので、この例外は global_exception_handler で 500 になるはず
    with patch(
        "services.gateway.main.health_check", side_effect=ResourceExhaustedError("Queue full")
    ):
        client = TestClient(app)
        response = client.get("/health")

        # 期待値: 429
        # 実装前は 500 (または例外未処理)
        assert response.status_code == 429
        assert response.json()["message"] == "Too Many Requests"
