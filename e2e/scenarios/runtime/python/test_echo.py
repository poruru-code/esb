"""
Where: e2e/scenarios/runtime/python/test_echo.py
What: Python echo Lambda E2E tests.
Why: Validate Python runtime fixtures and basic invocation behavior.
"""

from e2e.conftest import AUTH_USER, call_api


class TestPythonEcho:
    """Verify Python echo Lambda invocation."""

    def test_python_echo_basic(self, auth_token):
        """E2E: basic Python echo invocation via Gateway."""
        response = call_api("/api/echo", auth_token, {"message": "hello-basic"})

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["message"] == "Echo: hello-basic"
        assert data["user"] == AUTH_USER
