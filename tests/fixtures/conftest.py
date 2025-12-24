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


def query_victorialogs_by_filter(
    filters: dict[str, str] | None = None,
    raw_query: str | None = None,
    start: str | None = None,
    end: str | None = None,
    timeout: int = VICTORIALOGS_QUERY_TIMEOUT,
    limit: int = 100,
    min_hits: int = 1,
    poll_interval: float = 1.0,
) -> dict:
    """
    VictoriaLogs から任意のフィルタ条件でログをクエリ

    VictoriaLogs LogsQL API を使用:
    - フィルタ: `field:"value"` 形式で AND 結合
    - 時間フィルタ: start/end パラメータ (ISO8601/RFC3339 形式)

    Args:
        filters: フィールド名と値の辞書 (例: {"trace_id": "xxx", "container_name": "gateway"})
        raw_query: 直接指定する LogsQL クエリ (filters と排他)
        start: 検索開始時刻 (ISO8601/RFC3339 形式, 例: "2025-12-24T01:00:00Z")
        end: 検索終了時刻 (ISO8601/RFC3339 形式)
        timeout: ポーリングタイムアウト秒数
        limit: 取得件数上限
        min_hits: 最小ヒット数 (この数以上のログが取得できるまでポーリング)
        poll_interval: ポーリング間隔 (秒)

    Returns:
        クエリ結果の dict (hits フィールドにログリストが含まれる)

    Example:
        # trace_id で検索
        query_victorialogs_by_filter(filters={"trace_id": "1-abc123"})

        # 複数フィルタ + 時間フィルタ
        query_victorialogs_by_filter(
            filters={"logger": "boto3.mock", "log_group": "/aws/lambda/test"},
            start="2025-12-24T00:00:00Z",
            min_hits=4,
        )

        # 直接 LogsQL を指定
        query_victorialogs_by_filter(raw_query='level:ERROR AND container_name:"gateway"')
    """
    # クエリ文字列の構築
    if raw_query:
        query = raw_query
    elif filters:
        query_parts = [f'{k}:"{v}"' for k, v in filters.items()]
        query = " AND ".join(query_parts)
    else:
        raise ValueError("Either 'filters' or 'raw_query' must be provided")

    params: dict[str, str | int] = {"query": query, "limit": limit}

    # 時間フィルタを追加 (VictoriaLogs HTTP API の start/end パラメータ)
    if start:
        params["start"] = start
    if end:
        params["end"] = end

    poll_start_time = time.time()
    while time.time() - poll_start_time < timeout:
        try:
            response = requests.get(
                f"{VICTORIALOGS_URL}/select/logsql/query",
                params=params,
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

                if len(hits) >= min_hits:
                    return {"hits": hits}

            time.sleep(poll_interval)

        except Exception as e:
            print(f"VictoriaLogs query error: {e}")
            time.sleep(poll_interval)

    return {"hits": []}


def query_victorialogs(
    request_id: str,
    timeout: int = VICTORIALOGS_QUERY_TIMEOUT,
    start: str | None = None,
) -> dict:
    """
    VictoriaLogs から Trace ID を含むログをクエリ (後方互換ラッパー)

    Args:
        request_id: 検索する Trace ID (root 部分)
        timeout: タイムアウト秒数
        start: 検索開始時刻 (ISO8601/RFC3339 形式)

    Returns:
        クエリ結果の dict (hits フィールドにログが含まれる)
    """
    return query_victorialogs_by_filter(
        filters={"trace_id": request_id},
        start=start,
        timeout=timeout,
    )


@pytest.fixture(scope="module")
def auth_token(gateway_health) -> str:
    """認証トークンを取得 (モジュールスコープでキャッシュ)"""
    return get_auth_token()


def request_with_retry(
    method: str,
    url: str,
    max_retries: int = 5,
    retry_interval: float = 2.0,
    retry_on_status: tuple[int, ...] = (500, 502, 503, 504),
    **kwargs,
) -> requests.Response:
    """
    リトライ付き HTTP リクエスト

    Args:
        method: HTTP メソッド (get, post, etc.)
        url: リクエスト先 URL
        max_retries: 最大リトライ回数
        retry_interval: リトライ間隔 (秒)
        retry_on_status: リトライ対象のステータスコード
        **kwargs: requests に渡す追加パラメータ

    Returns:
        requests.Response オブジェクト
    """
    response = None
    for i in range(max_retries):
        try:
            response = getattr(requests, method.lower())(url, **kwargs)
            if response.status_code not in retry_on_status:
                return response
            print(f"Retry {i + 1}/{max_retries}: Status {response.status_code}")
        except requests.exceptions.ConnectionError as e:
            print(f"Connection error ({i + 1}/{max_retries}): {e}")
            response = None

        time.sleep(retry_interval)

    if response is None:
        raise requests.exceptions.ConnectionError(f"Failed to connect after {max_retries} retries")
    return response


def call_api(
    path: str,
    auth_token: str,
    payload: dict | None = None,
    method: str = "post",
    timeout: int = DEFAULT_REQUEST_TIMEOUT,
    **kwargs,
) -> requests.Response:
    """
    Gateway 経由で API を呼び出す共通ヘルパー

    Args:
        path: API パス (例: "/api/echo", "/api/chain")
        auth_token: 認証トークン
        payload: リクエストボディ (JSON)
        method: HTTP メソッド (デフォルト: post)
        timeout: リクエストタイムアウト
        **kwargs: requests に渡す追加パラメータ

    Returns:
        requests.Response オブジェクト

    Example:
        response = call_api("/api/echo", auth_token, {"message": "hello"})
        data = response.json()
    """
    url = f"{GATEWAY_URL}{path}"
    headers = {"Authorization": f"Bearer {auth_token}"}

    if "headers" in kwargs:
        headers.update(kwargs.pop("headers"))

    return getattr(requests, method.lower())(
        url,
        json=payload,
        headers=headers,
        verify=VERIFY_SSL,
        timeout=timeout,
        **kwargs,
    )
