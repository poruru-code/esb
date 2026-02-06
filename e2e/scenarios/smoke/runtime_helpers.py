"""
Where: e2e/scenarios/smoke/runtime_helpers.py
What: Shared smoke helpers for runtime connectivity tests.
Why: Keep smoke behavior identical across runtimes while grouping by language.
"""

import json

import pytest

from e2e.conftest import call_api


def assert_success(response, action):
    assert response.status_code == 200, f"{action} failed: {response.text}"
    data = response.json()
    assert data.get("success") is True, f"{action} returned error: {data}"
    assert data.get("action") == action, f"Unexpected action: {data}"
    return data


def assert_child_success(child):
    if isinstance(child, str):
        try:
            child = json.loads(child)
        except json.JSONDecodeError:
            pytest.fail(f"Child payload is not JSON: {child}")

    if isinstance(child, dict) and "body" in child and isinstance(child["body"], str):
        try:
            body = json.loads(child["body"])
        except json.JSONDecodeError:
            pytest.fail(f"Child body is not JSON: {child['body']}")
        assert body.get("success") is True, f"Child invocation failed: {body}"
        return

    if isinstance(child, dict):
        assert child.get("success") is True, f"Child invocation failed: {child}"
        return

    pytest.fail(f"Unexpected child payload type: {type(child)}")


def run_echo(runtime, auth_token):
    runtime_id = runtime["id"]
    message = f"smoke-{runtime_id}"
    response = call_api(
        runtime["path"],
        auth_token,
        {"action": "echo", "message": message},
    )
    data = assert_success(response, "echo")
    assert data.get("message") == f"Echo: {message}", f"Echo mismatch: {data}"


def run_dynamodb_put(runtime, auth_token):
    response = call_api(
        runtime["path"],
        auth_token,
        {"action": "dynamodb_put", "key": f"smoke-{runtime['id']}", "value": "ok"},
    )
    assert_success(response, "dynamodb_put")


def run_s3_put(runtime, auth_token):
    response = call_api(
        runtime["path"],
        auth_token,
        {
            "action": "s3_put",
            "bucket": "e2e-test-bucket",
            "key": f"{runtime['id']}-smoke.txt",
            "content": "ok",
        },
    )
    assert_success(response, "s3_put")


def run_chain_invoke(runtime, auth_token):
    response = call_api(
        runtime["path"],
        auth_token,
        {"action": "chain_invoke", "target": "lambda-echo", "message": "from-smoke"},
    )
    data = assert_success(response, "chain_invoke")
    child = data.get("child")
    assert child is not None, f"Missing child payload: {data}"
    assert_child_success(child)
