import os
import pytest

# インポート時に Config が初期化されるため、トップレベルで環境変数を設定する
# JWT_SECRET_KEYは32文字以上が必須
os.environ["JWT_SECRET_KEY"] = "test-secret-key-must-be-at-least-32-chars"
os.environ["X_API_KEY"] = "test-api-key"
os.environ["AUTH_USER"] = "test-user"
os.environ["AUTH_PASS"] = "test-password"
os.environ["CONTAINERS_NETWORK"] = "test-network"
os.environ["MANAGER_URL"] = "http://test-manager:8081"
os.environ["GATEWAY_INTERNAL_URL"] = "https://test-gateway"


@pytest.fixture(scope="session", autouse=True)
def set_test_env():
    """
    テスト実行時に必要な環境変数を設定するフィクスチャ
    （明示的な依存関係のために残すが、実質的な設定はロード時に行われる）
    """
    yield
