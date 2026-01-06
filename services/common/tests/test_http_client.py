import pytest
from unittest.mock import patch
from services.common.core.config import BaseAppConfig

try:
    from services.common.core.http_client import HttpClientFactory
except ImportError:
    HttpClientFactory = None


class TestHttpClientFactory:
    def test_factory_import(self):
        """Factory class should be importable (Checks existence)"""
        assert HttpClientFactory is not None, "HttpClientFactory class not found"

    @patch("httpx.AsyncClient")
    def test_create_async_client_verify_false(self, mock_client):
        """VERIFY_SSL=False should produce client with verify=False"""
        if HttpClientFactory is None:
            pytest.skip("HttpClientFactory not implemented yet")

        config = BaseAppConfig(VERIFY_SSL=False)
        factory = HttpClientFactory(config)
        factory.create_async_client()

        mock_client.assert_called_once()
        _, kwargs = mock_client.call_args
        assert kwargs["verify"] is False
        assert kwargs["trust_env"] is False
        assert "proxies" not in kwargs

    @patch("httpx.AsyncClient")
    def test_create_async_client_verify_true(self, mock_client):
        """VERIFY_SSL=True should produce client with verify=True"""
        if HttpClientFactory is None:
            pytest.skip("HttpClientFactory not implemented yet")

        config = BaseAppConfig(VERIFY_SSL=True)
        factory = HttpClientFactory(config)
        factory.create_async_client()

        mock_client.assert_called_once()
        _, kwargs = mock_client.call_args
        assert kwargs["verify"] is True
        assert kwargs["trust_env"] is False
        assert "proxies" not in kwargs

    @patch("urllib3.disable_warnings")
    def test_configure_global_settings_disable_warnings(self, mock_disable):
        """VERIFY_SSL=False should trigger disable_warnings"""
        if HttpClientFactory is None:
            pytest.skip("HttpClientFactory not implemented yet")

        config = BaseAppConfig(VERIFY_SSL=False)
        factory = HttpClientFactory(config)
        factory.configure_global_settings()

        mock_disable.assert_called_once()

    @patch("urllib3.disable_warnings")
    def test_configure_global_settings_no_disable_warnings(self, mock_disable):
        """VERIFY_SSL=True should NOT trigger disable_warnings"""
        if HttpClientFactory is None:
            pytest.skip("HttpClientFactory not implemented yet")

        config = BaseAppConfig(VERIFY_SSL=True)
        factory = HttpClientFactory(config)
        factory.configure_global_settings()

        mock_disable.assert_not_called()
