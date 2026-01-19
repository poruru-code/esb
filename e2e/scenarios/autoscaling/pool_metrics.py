"""
Autoscaling pool metrics helpers for E2E tests.

Provides a small wrapper around /metrics/pools so tests can assert
pool state without touching internal container APIs.
"""

import time
from typing import Callable, Iterable, Optional

import requests

from e2e.conftest import DEFAULT_REQUEST_TIMEOUT, GATEWAY_URL, VERIFY_SSL


def _fetch_pool_metrics(auth_token: str) -> dict:
    headers = {"Authorization": f"Bearer {auth_token}"}
    response = requests.get(
        f"{GATEWAY_URL}/metrics/pools",
        headers=headers,
        verify=VERIFY_SSL,
        timeout=DEFAULT_REQUEST_TIMEOUT,
    )
    response.raise_for_status()
    return response.json()


def get_pool_entry(auth_token: str, function_names: Iterable[str]) -> Optional[dict]:
    names = set(function_names)
    data = _fetch_pool_metrics(auth_token)
    return next(
        (item for item in data.get("pools", []) if item.get("function_name") in names),
        None,
    )


def wait_for_pool_entry(
    auth_token: str,
    function_names: Iterable[str],
    predicate: Optional[Callable[[dict], bool]] = None,
    timeout_seconds: float = 10.0,
    interval_seconds: float = 1.0,
) -> dict:
    deadline = time.time() + timeout_seconds
    last_entry = None

    while time.time() < deadline:
        try:
            entry = get_pool_entry(auth_token, function_names)
        except requests.HTTPError as exc:
            raise AssertionError(f"Pool metrics request failed: {exc}") from exc
        except requests.RequestException:
            entry = None
        if entry is not None:
            last_entry = entry
            if predicate is None or predicate(entry):
                return entry
        time.sleep(interval_seconds)

    if last_entry is None:
        names = ", ".join(sorted(set(function_names)))
        raise AssertionError(f"Pool metrics entry not found for: {names}")
    if predicate is not None:
        raise AssertionError(f"Pool metrics entry did not satisfy predicate: {last_entry}")
    return last_entry
