"""
E2E テスト共通 Fixture とヘルパー関数

テストファイル分割のための共通設定。
各テストファイルはこの conftest.py から fixture と定数を利用する。
"""

import os
from pathlib import Path
import time
import json

import pytest
import requests
from dotenv import load_dotenv

# .env.test をロード (run_tests.py を経由しない場合でもテスト可能にする)
env_file = Path(__file__).parent.parent / ".env.test"
if env_file.exists():
    load_dotenv(env_file, override=False)

from services.common.core.http_client import HttpClientFactory  # noqa: E402
from services.gateway.config import config  # noqa: E402

# Global SSL configuration
factory = HttpClientFactory(config)
factory.configure_global_settings()
VERIFY_SSL = config.VERIFY_SSL

# テスト用設定
GATEWAY_PORT = os.getenv("GATEWAY_PORT", "443")
GATEWAY_URL = os.getenv("GATEWAY_URL", f"https://localhost:{GATEWAY_PORT}")

VICTORIALOGS_PORT = os.getenv("VICTORIALOGS_PORT", "9428")
VICTORIALOGS_URL = os.getenv("VICTORIALOGS_URL", f"http://localhost:{VICTORIALOGS_PORT}")
API_KEY = config.X_API_KEY

# 認証情報は環境変数から取得 (.env.test でロード済み)
AUTH_USER = os.environ.get("AUTH_USER", "")
AUTH_PASS = os.environ.get("AUTH_PASS", "")

# Timeouts & Retries
DEFAULT_REQUEST_TIMEOUT = 5
HEALTH_CHECK_RETRIES = 10
HEALTH_CHECK_INTERVAL = 3
VICTORIALOGS_QUERY_TIMEOUT = 30
LOG_WAIT_TIMEOUT = 45
SCYLLA_WAIT_RETRIES = 40
SCYLLA_WAIT_INTERVAL = 5
ASYNC_WAIT_RETRIES = 60
MANAGER_RESTART_WAIT = 8
STABILIZATION_WAIT = 3


@pytest.fixture(scope="module")
def gateway_health():
    """Gateway のヘルスチェック (module スコープ)"""
    for i in range(HEALTH_CHECK_RETRIES):
        try:
            response = requests.get(
                f"{GATEWAY_URL}/health", timeout=DEFAULT_REQUEST_TIMEOUT, verify=VERIFY_SSL
            )
            if response.status_code == 200:
                return True
            print(f"Gateway returned status: {response.status_code}")
        except Exception as e:
            print(f"Waiting for Gateway... ({i + 1}/{HEALTH_CHECK_RETRIES}) Error: {e}")
            time.sleep(HEALTH_CHECK_INTERVAL)
    pytest.skip(
        f"Gateway is not running on {GATEWAY_URL}. Start with: docker compose up -d gateway"
    )


def get_auth_token() -> str:
    """認証して JWT トークンを取得"""
    response = requests.post(
        f"{GATEWAY_URL}{config.AUTH_ENDPOINT_PATH}",
        json={"AuthParameters": {"USERNAME": AUTH_USER, "PASSWORD": AUTH_PASS}},
        headers={"x-api-key": API_KEY},
        verify=VERIFY_SSL,
    )
    assert response.status_code == 200, f"Auth failed: {response.text}"
    return response.json()["AuthenticationResult"]["IdToken"]


def query_victorialogs(request_id: str, timeout: int = VICTORIALOGS_QUERY_TIMEOUT) -> dict:
    """
    VictoriaLogs から RequestID を含むログをクエリ

    Args:
        request_id: 検索する RequestID
        timeout: タイムアウト秒数

    Returns:
        クエリ結果の dict (hits フィールドにログが含まれる)
    """
    query = f'request_id:"{request_id}"'
    params = {"query": query, "limit": 100}

    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            response = requests.post(
                f"{VICTORIALOGS_URL}/select/logsql/query",
                data=params,
                timeout=DEFAULT_REQUEST_TIMEOUT,
            )

            if response.status_code == 200:
                lines = response.text.strip().split("\n")
                hits = []
                for line in lines:
                    if line:
                        try:
                            hits.append(json.loads(line))
                        except json.JSONDecodeError:
                            continue

                if hits:
                    return {"hits": hits}

            time.sleep(1)

        except Exception as e:
            print(f"VictoriaLogs query error: {e}")
            time.sleep(1)

    return {"hits": []}
