import os
import pytest

# インポート時に Config が初期化されるため、トップレベルで環境変数を設定する
os.environ["JWT_SECRET_KEY"] = "test-secret-key"
os.environ["X_API_KEY"] = "test-api-key"
os.environ["CONTAINERS_NETWORK"] = "test-net"


@pytest.fixture(scope="session", autouse=True)
def set_test_env():
    """
    テスト実行時に必要な環境変数を設定するフィクスチャ
    （明示的な依存関係のために残すが、実質的な設定はロード時に行われる）
    """
    yield
