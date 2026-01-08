"""
Container Host Cache - TTL-based LRU cache for container hosts.

Reduces latency by caching container host information from Manager,
avoiding redundant HTTP calls on warm starts.
"""

import logging
import os
from typing import Optional

from cachetools import TTLCache

logger = logging.getLogger("gateway.container_cache")


class ContainerHostCache:
    """
    TTL-based LRU cache for container host names using cachetools.

    Note: This cache is designed for single-threaded async environments (FastAPI/uvicorn).
    All operations are atomic in this context, so no locking is required.
    """

    def __init__(
        self,
        max_size: int = 100,
        ttl_seconds: Optional[float] = None,
    ):
        """
        Initialize the cache.

        Args:
            max_size: Maximum number of entries (default: 100)
            ttl_seconds: Time-to-live in seconds (default: 30, or from CONTAINER_CACHE_TTL env)
        """
        self.max_size = max_size

        # TTL from env or default
        if ttl_seconds is not None:
            self.ttl_seconds = ttl_seconds
        else:
            self.ttl_seconds = float(os.getenv("CONTAINER_CACHE_TTL", "30"))

        # TTLCache: Handles both LRU eviction and TTL expiration automatically
        self._cache = TTLCache(maxsize=self.max_size, ttl=self.ttl_seconds)

        logger.debug(
            f"ContainerHostCache initialized (cachetools): "
            f"max_size={max_size}, ttl={self.ttl_seconds}s"
        )

    def get(self, function_name: str) -> Optional[str]:
        """
        Get cached host for function.

        Args:
            function_name: Lambda function name

        Returns:
            Cached host string, or None if not found or expired
        """
        # TTLCache returns None or raises KeyError depending on usage.
        # .get() is safe and handles expiration automatically.
        return self._cache.get(function_name)

    def set(self, function_name: str, host: str) -> None:
        """
        Cache host for function.

        Args:
            function_name: Lambda function name
            host: Container host name or IP
        """
        self._cache[function_name] = host

    def invalidate(self, function_name: str) -> None:
        """
        Remove specific entry from cache.

        Args:
            function_name: Lambda function name to invalidate
        """
        if function_name in self._cache:
            del self._cache[function_name]
            logger.debug(f"Cache invalidated: {function_name}")

    def clear(self) -> None:
        """Clear all cache entries."""
        self._cache.clear()
        logger.debug("Cache cleared")
