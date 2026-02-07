"""
Image-based Lambda E2E tests.

Verifies that a SAM Image function can be invoked through Gateway.
"""

import os
import time

import pytest
from requests import Response
from requests import exceptions as requests_exceptions

from e2e.conftest import call_api


class TestImageFunction:
    """Verify image-based Lambda invocation."""

    def test_image_function_basic(self, auth_token):
        expect_success = os.getenv("E2E_IMAGE_TEST_EXPECT_SUCCESS", "1").strip().lower() not in {
            "0",
            "false",
            "no",
            "off",
        }
        max_retries = 20
        response: Response | None = None

        for _ in range(max_retries):
            try:
                response = call_api("/api/image", auth_token, {"message": "hello-image"})
            except requests_exceptions.RequestException:
                # Gateway/transport can transiently break while the first invoke warms up.
                time.sleep(2)
                continue
            if response.status_code == 200:
                break

            # During first invocation the runtime may still be provisioning.
            if response.status_code not in [500, 502, 503, 504]:
                break

            time.sleep(2)

        if response is None:
            pytest.fail("Image function failed: no response")
        if expect_success:
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
