"""
Where: e2e/scenarios/standard/test_java_echo.py
What: Java echo Lambda E2E tests.
Why: Validate Java runtime fixtures and basic invocation behavior.
"""

from e2e.conftest import AUTH_USER, call_api


class TestJavaEcho:
    """Verify Java echo Lambda invocation."""

    def test_java_echo_basic(self, auth_token):
        """E2E: basic Java echo invocation via Gateway."""
        response = call_api("/api/echo-java", auth_token, {"message": "hello-java"})

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["message"] == "Echo: hello-java"
        assert data["user"] == AUTH_USER
