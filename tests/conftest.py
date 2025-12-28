"""
Shared fixtures and helpers for E2E tests.

Common settings for split test files.
Each test file uses fixtures and constants from this conftest.py.
"""

import os
from pathlib import Path
import time
import json

import pytest
import requests
from dotenv import load_dotenv

# Load .env.test (enable tests even without run_tests.py).
env_file = Path(__file__).parent / ".env.test"
if env_file.exists():
    print(f"Loading .env.test from {env_file} (base/defaults only)")
    load_dotenv(env_file, override=False)
else:
    print(f".env.test not found at {env_file}")

from services.common.core.http_client import HttpClientFactory  # noqa: E402
from services.gateway.config import config  # noqa: E402

# Global SSL configuration
factory = HttpClientFactory(config)
factory.configure_global_settings()
VERIFY_SSL = config.VERIFY_SSL

# Test settings
GATEWAY_PORT = os.getenv("GATEWAY_PORT", "443")
GATEWAY_URL = os.getenv("GATEWAY_URL", f"https://localhost:{GATEWAY_PORT}")

VICTORIALOGS_PORT = os.getenv("VICTORIALOGS_PORT", "9428")
VICTORIALOGS_URL = os.getenv("VICTORIALOGS_URL", f"http://localhost:{VICTORIALOGS_PORT}")
API_KEY = config.X_API_KEY

# Auth info is read from environment variables (loaded from .env.test).
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
ORCHESTRATOR_RESTART_WAIT = 8
STABILIZATION_WAIT = 3


@pytest.fixture(scope="module")
def gateway_health():
    """Gateway health check (module scope)."""
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
    """Authenticate and get a JWT token."""
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
    Query logs from VictoriaLogs using filter conditions.

    Uses VictoriaLogs LogsQL API:
    - Filters: `field:"value"` format combined with AND
    - Time filters: start/end parameters (ISO8601/RFC3339)

    Args:
        filters: dict of field names and values (e.g., {"trace_id": "xxx", "container_name": "gateway"})
        raw_query: direct LogsQL query (exclusive with filters)
        start: search start time (ISO8601/RFC3339, e.g., "2025-12-24T01:00:00Z")
        end: search end time (ISO8601/RFC3339)
        timeout: polling timeout in seconds
        limit: max number of hits
        min_hits: minimum hits to stop polling
        poll_interval: polling interval (seconds)

    Returns:
        Dict of query results (hits contains logs)

    Example:
        # Search by trace_id.
        query_victorialogs_by_filter(filters={"trace_id": "1-abc123"})

        # Multiple filters + time filter.
        query_victorialogs_by_filter(
            filters={"logger": "boto3.mock", "log_group": "/aws/lambda/test"},
            start="2025-12-24T00:00:00Z",
            min_hits=4,
        )

        # Direct LogsQL query.
        query_victorialogs_by_filter(raw_query='level:ERROR AND container_name:"gateway"')
    """
    # Build query string.
    if raw_query:
        query = raw_query
    elif filters:
        query_parts = [f'{k}:"{v}"' for k, v in filters.items()]
        query = " AND ".join(query_parts)
    else:
        raise ValueError("Either 'filters' or 'raw_query' must be provided")

    params: dict[str, str | int] = {"query": query, "limit": limit}

    # Add time filters (VictoriaLogs HTTP API start/end params).
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
    trace_id_root: str,
    timeout: int = VICTORIALOGS_QUERY_TIMEOUT,
    start: str | None = None,
) -> dict:
    """
    Query logs containing Trace ID from VictoriaLogs (backward-compatible wrapper).

    Args:
        trace_id_root: Trace ID to search for (root portion)
        timeout: timeout in seconds
        start: search start time (ISO8601/RFC3339)

    Returns:
        Dict of query results (hits contains logs)
    """
    return query_victorialogs_by_filter(
        filters={"trace_id": trace_id_root},
        start=start,
        timeout=timeout,
    )


@pytest.fixture(scope="module")
def auth_token(gateway_health) -> str:
    """Get auth token (cached at module scope)."""
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
    HTTP request with retries.

    Args:
        method: HTTP method (get, post, etc.)
        url: request URL
        max_retries: max retry count
        retry_interval: retry interval (seconds)
        retry_on_status: status codes that trigger retries
        **kwargs: additional params for requests

    Returns:
        requests.Response object
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
    auth_token: str | None = None,
    payload: dict | None = None,
    method: str = "post",
    timeout: int = DEFAULT_REQUEST_TIMEOUT,
    **kwargs,
) -> requests.Response:
    """
    Common helper to call APIs via Gateway.

    Args:
        path: API path (e.g., "/api/echo", "/api/call")
        auth_token: auth token (no auth when None)
        payload: request body (JSON)
        method: HTTP method (default: post)
        timeout: request timeout
        **kwargs: additional params for requests

    Returns:
        requests.Response object

    Example:
        # With authentication.
        response = call_api("/api/echo", auth_token, {"message": "hello"})

        # Without authentication (for 401 tests).
        response = call_api("/api/echo", payload={"message": "hello"})
    """
    url = f"{GATEWAY_URL}{path}"
    headers = {}

    if auth_token:
        headers["Authorization"] = f"Bearer {auth_token}"

    if "headers" in kwargs:
        headers.update(kwargs.pop("headers"))

    return getattr(requests, method.lower())(
        url,
        json=payload,
        headers=headers if headers else None,
        verify=VERIFY_SSL,
        timeout=timeout,
        **kwargs,
    )
