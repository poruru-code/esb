"""
Image-based Lambda E2E tests.

Verifies that a SAM Image function can be invoked through Gateway.
"""

import json
import os
import time
import uuid

import pytest
from requests import Response
from requests import exceptions as requests_exceptions

from e2e.conftest import LOG_WAIT_TIMEOUT, call_api, wait_for_victorialogs_hits


def _expect_success() -> bool:
    return os.getenv("E2E_IMAGE_TEST_EXPECT_SUCCESS", "1").strip().lower() not in {
        "0",
        "false",
        "no",
        "off",
    }


def _expected_cloudwatch_logger() -> str:
    return (
        os.getenv("E2E_IMAGE_EXPECTED_LOGGER", "cloudwatch.logs.python").strip()
        or "cloudwatch.logs.python"
    )


def _invoke_image(auth_token: str, payload: dict) -> Response:
    max_retries = 20
    response: Response | None = None

    for _ in range(max_retries):
        try:
            response = call_api("/api/image", auth_token, payload)
        except requests_exceptions.RequestException:
            # Gateway/transport can transiently break while the first invoke warms up.
            time.sleep(2)
            continue
        if response.status_code == 200:
            return response

        # During first invocation the runtime may still be provisioning.
        if response.status_code not in [500, 502, 503, 504]:
            return response
        time.sleep(2)

    if response is None:
        pytest.fail("Image function failed: no response")
    return response


class TestImageFunction:
    """Verify image-based Lambda invocation."""

    def test_image_function_basic(self, auth_token):
        response = _invoke_image(auth_token, {"message": "hello-image"})
        if _expect_success():
            assert response.status_code == 200, f"Image function failed: {response.text}"
            data = response.json()
            if isinstance(data, dict) and "success" in data:
                assert data["success"] is True
            return

        assert response.status_code >= 500, (
            f"Expected failure but got: {response.status_code} {response.text}"
        )
        error_markers = [
            "IMAGE_PULL_FAILED",
            "IMAGE_AUTH_FAILED",
            "IMAGE_PUSH_FAILED",
            "IMAGE_DIGEST_MISMATCH",
        ]
        assert any(marker in response.text for marker in error_markers), (
            f"Expected image sync error code in response, got: {response.text}"
        )

    def test_image_function_chain_invoke(self, auth_token):
        if not _expect_success():
            pytest.skip("Image failure mode is enabled.")

        response = _invoke_image(
            auth_token,
            {
                "action": "chain_invoke",
                "target": "lambda-echo",
                "message": "from-image-chain",
            },
        )

        assert response.status_code == 200, response.text
        data = response.json()
        assert data.get("success") is True

        chain = data.get("chain")
        assert isinstance(chain, dict), f"chain result missing: {data}"
        assert chain.get("status_code") == 200, f"unexpected chain status: {chain}"

        child = chain.get("child")
        assert isinstance(child, dict), f"child payload missing: {chain}"
        assert child.get("statusCode") == 200, f"unexpected child status: {child}"

        body_raw = child.get("body", "{}")
        body = json.loads(body_raw) if isinstance(body_raw, str) else body_raw
        assert isinstance(body, dict), f"unexpected child body: {body_raw}"
        assert body.get("success") is True
        assert body.get("message") == "Echo: from-image-chain"

    def test_image_function_s3_access(self, auth_token):
        if not _expect_success():
            pytest.skip("Image failure mode is enabled.")

        key = f"image-{uuid.uuid4().hex[:8]}.txt"
        content = "from-image-s3"
        response = _invoke_image(
            auth_token,
            {
                "action": "s3_roundtrip",
                "bucket": "e2e-test-bucket",
                "key": key,
                "content": content,
            },
        )

        assert response.status_code == 200, response.text
        data = response.json()
        assert data.get("success") is True

        result = data.get("s3")
        assert isinstance(result, dict), f"s3 result missing: {data}"
        assert result.get("bucket") == "e2e-test-bucket"
        assert result.get("key") == key
        assert result.get("content") == content

    def test_image_function_victorialogs(self, auth_token):
        if not _expect_success():
            pytest.skip("Image failure mode is enabled.")

        marker = f"image-cloudwatch-{uuid.uuid4().hex}"
        response = _invoke_image(
            auth_token,
            {
                "action": "test_cloudwatch",
                "marker": marker,
            },
        )

        assert response.status_code == 200, response.text
        data = response.json()
        assert data.get("success") is True

        cloudwatch = data.get("cloudwatch")
        assert isinstance(cloudwatch, dict), f"cloudwatch result missing: {data}"
        log_group = cloudwatch.get("log_group")
        log_stream = cloudwatch.get("log_stream")
        assert isinstance(log_group, str) and log_group
        assert isinstance(log_stream, str) and log_stream

        hits, found = wait_for_victorialogs_hits(
            filters={
                "container_name": "lambda-image",
                "logger": _expected_cloudwatch_logger(),
                "log_group": log_group,
                "log_stream": log_stream,
            },
            timeout=LOG_WAIT_TIMEOUT,
            min_hits=1,
            matcher=lambda hit: marker in str(hit.get("message", ""))
            or marker in str(hit.get("_msg", "")),
        )
        assert found, (
            "CloudWatch passthrough log was not found in VictoriaLogs "
            f"(marker={marker}, log_group={log_group}, log_stream={log_stream}, hits={hits})"
        )
