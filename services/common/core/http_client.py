import logging

import httpx
import urllib3

from .config import BaseAppConfig

logger = logging.getLogger(__name__)


class HttpClientFactory:
    """
    HTTP Client Factory for centralized SSL verification handling.
    """

    def __init__(self, config: BaseAppConfig):
        self.config = config

    def configure_global_settings(self):
        """
        Configure global settings like urllib3 warnings.
        """
        if not self.config.VERIFY_SSL:
            urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
            logger.debug("InsecureRequestWarning disabled (VERIFY_SSL=False)")

    def create_async_client(self, **kwargs) -> httpx.AsyncClient:
        """
        Create an httpx.AsyncClient with configured SSL verification.

        Args:
            **kwargs: Additional arguments for httpx.AsyncClient
        """
        verify = kwargs.pop("verify", None)

        # If verify is not explicitly provided, use config default
        if verify is None:
            verify = self.config.VERIFY_SSL

        # Default limits for high throughput (can be overridden by caller)
        if "limits" not in kwargs:
            kwargs["limits"] = httpx.Limits(max_keepalive_connections=20, max_connections=100)
        # Avoid leaking host HTTP(S)_PROXY/NO_PROXY into internal calls unless explicitly requested.
        kwargs.setdefault("trust_env", False)

        return httpx.AsyncClient(verify=verify, **kwargs)

    def create_sync_client(self, **kwargs) -> httpx.Client:
        """
        Create an httpx.Client with configured SSL verification.
        """
        verify = kwargs.pop("verify", None)

        if verify is None:
            verify = self.config.VERIFY_SSL

        return httpx.Client(verify=verify, **kwargs)
