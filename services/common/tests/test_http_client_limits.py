import httpx
from unittest.mock import patch
from services.common.core.config import BaseAppConfig
from services.common.core.http_client import HttpClientFactory


class TestHttpClientLimits:
    @patch("httpx.AsyncClient")
    def test_create_async_client_defaults_limits(self, mock_client):
        """Ensure extended default Limits are applied when creating AsyncClient."""
        config = BaseAppConfig(VERIFY_SSL=True)
        factory = HttpClientFactory(config)

        factory.create_async_client()

        # Check call arguments.
        args, kwargs = mock_client.call_args
        limits = kwargs.get("limits")

        assert isinstance(limits, httpx.Limits)
        assert limits.max_keepalive_connections == 20
        assert limits.max_connections == 100

    @patch("httpx.AsyncClient")
    def test_create_async_client_override_limits(self, mock_client):
        """Ensure provided Limits override the defaults."""
        config = BaseAppConfig(VERIFY_SSL=True)
        factory = HttpClientFactory(config)

        custom_limits = httpx.Limits(max_connections=500)
        factory.create_async_client(limits=custom_limits)

        args, kwargs = mock_client.call_args
        limits = kwargs.get("limits")

        assert limits == custom_limits
        assert limits.max_connections == 500
