import asyncio
import logging
from typing import Optional, Dict, Any

import httpx

from .core.exceptions import (
    FunctionNotFoundError,
    OrchestratorError,
    OrchestratorTimeoutError,
    OrchestratorUnreachableError,
)
from .services.container_cache import ContainerHostCache
from services.common.core.request_context import get_trace_id

logger = logging.getLogger("gateway.client")


class OrchestratorClient:
    """
    Legacy HTTP client for container ensure API (Manager/Orchestrator).
    """

    def __init__(
        self,
        client: httpx.AsyncClient,
        manager_url: str = "",
        cache: Optional[ContainerHostCache] = None,
        timeout: Optional[float] = None,
    ):
        self.client = client
        self.manager_url = manager_url.rstrip("/")
        self.cache = cache
        self.timeout = timeout
        self._inflight: Dict[str, asyncio.Future] = {}
        self._lock = asyncio.Lock()

    def invalidate_cache(self, function_name: str) -> None:
        if not self.cache:
            return
        self.cache.invalidate(function_name)

    async def ensure_container(
        self,
        function_name: str,
        image: Optional[str] = None,
        env: Optional[Dict[str, str]] = None,
    ) -> str:
        if self.cache:
            cached = self.cache.get(function_name)
            if cached:
                return cached

        async with self._lock:
            if self.cache:
                cached = self.cache.get(function_name)
                if cached:
                    return cached

            existing = self._inflight.get(function_name)
            if existing:
                future = existing
                is_leader = False
            else:
                loop = asyncio.get_running_loop()
                future = loop.create_future()
                self._inflight[function_name] = future
                is_leader = True

        if not is_leader:
            return await future

        try:
            host = await self._ensure_container_remote(function_name, image=image, env=env)
            if self.cache:
                self.cache.set(function_name, host)
            future.set_result(host)
            return host
        except Exception as exc:
            future.set_exception(exc)
            future.exception()
            raise
        finally:
            async with self._lock:
                current = self._inflight.get(function_name)
                if current is future:
                    del self._inflight[function_name]

    async def _ensure_container_remote(
        self,
        function_name: str,
        image: Optional[str],
        env: Optional[Dict[str, str]],
    ) -> str:
        url = f"{self.manager_url}/containers/ensure" if self.manager_url else "/containers/ensure"
        payload: Dict[str, Any] = {"function_name": function_name}
        if image is not None:
            payload["image"] = image
        if env:
            payload["env"] = env

        headers = {}
        trace_id = get_trace_id()
        if trace_id:
            headers["X-Amzn-Trace-Id"] = trace_id

        post_kwargs: Dict[str, Any] = {"json": payload}
        if headers:
            post_kwargs["headers"] = headers
        if self.timeout is not None:
            post_kwargs["timeout"] = self.timeout

        try:
            response = await self.client.post(url, **post_kwargs)
            response.raise_for_status()
        except httpx.TimeoutException as exc:
            raise OrchestratorTimeoutError(str(exc)) from exc
        except httpx.RequestError as exc:
            raise OrchestratorUnreachableError(exc) from exc
        except httpx.HTTPStatusError as exc:
            status_code = exc.response.status_code if exc.response else 500
            detail = exc.response.text if exc.response else str(exc)
            if status_code == 404:
                raise FunctionNotFoundError(function_name) from exc
            raise OrchestratorError(status_code, detail) from exc

        data = response.json()
        host = data.get("host")
        if not host:
            raise OrchestratorError(response.status_code, "Missing host in response")
        return host
