import os
import pytest

# インポート時に Config が初期化されるため、トップレベルで環境変数を設定する
os.environ["CONTAINERS_NETWORK"] = "test-net"
# Configのロードに失敗しないよう、他に必要な変数があれば設定する
# ManagerConfigでは CONTAINERS_NETWORK が必須になった


@pytest.fixture(scope="session", autouse=True)
def set_test_env():
    """
    テスト実行時に必要な環境変数を設定するフィクスチャ
    """
    yield
