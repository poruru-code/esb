"""
E2E テスト共通 Fixture

tests/ ディレクトリのルートに配置し、全テストで利用可能にする。
"""

# tests/fixtures/conftest.py から fixture と定数を再エクスポート
from tests.fixtures.conftest import (
    gateway_health,  # noqa: F401
    get_auth_token,  # noqa: F401
    query_victorialogs,  # noqa: F401
    GATEWAY_URL,  # noqa: F401
    VICTORIALOGS_URL,  # noqa: F401
    VERIFY_SSL,  # noqa: F401
    API_KEY,  # noqa: F401
    AUTH_USER,  # noqa: F401
    AUTH_PASS,  # noqa: F401
    DEFAULT_REQUEST_TIMEOUT,  # noqa: F401
    HEALTH_CHECK_RETRIES,  # noqa: F401
    HEALTH_CHECK_INTERVAL,  # noqa: F401
    VICTORIALOGS_QUERY_TIMEOUT,  # noqa: F401
    LOG_WAIT_TIMEOUT,  # noqa: F401
    SCYLLA_WAIT_RETRIES,  # noqa: F401
    SCYLLA_WAIT_INTERVAL,  # noqa: F401
    ASYNC_WAIT_RETRIES,  # noqa: F401
    MANAGER_RESTART_WAIT,  # noqa: F401
    STABILIZATION_WAIT,  # noqa: F401
)
from services.gateway.config import config  # noqa: F401
