from typing import Optional, Dict
import asyncio
import httpx
import logging
from .core.exceptions import (
    FunctionNotFoundError,
    ManagerError,
    ManagerTimeoutError,
    ManagerUnreachableError,
)
from .services.container_cache import ContainerHostCache
from services.common.core.request_context import get_request_id
from services.common.models.internal import ContainerEnsureRequest, ContainerInfoResponse
from .config import config

logger = logging.getLogger("gateway.client")


class ManagerClient:
    def __init__(self, http_client: httpx.AsyncClient, cache: Optional[ContainerHostCache] = None):
        self.client = http_client
        self.cache = cache or ContainerHostCache()
        # Singleflight: 進行中のリクエストを管理
        self._pending_requests: Dict[str, asyncio.Future] = {}

    def invalidate_cache(self, function_name: str) -> None:
        """
        Invalidate cache for a specific function.

        Call this when Lambda connection fails to ensure next request
        re-fetches container info from Manager.
        """
        self.cache.invalidate(function_name)
        logger.debug(f"Cache invalidated for {function_name}")

    async def ensure_container(
        self, function_name: str, image: Optional[str] = None, env: Optional[Dict[str, str]] = None
    ) -> str:
        """
        Calls Manager Service to ensure container is running and get its host/IP.

        Uses TTL-based cache and Singleflight pattern to avoid redundant Manager calls.

        Raises:
            FunctionNotFoundError: 関数/イメージが存在しない (404)
            ManagerError: Docker API エラーなど (400, 409など)
            ManagerTimeoutError: タイムアウト (408)
            ManagerUnreachableError: Manager への接続失敗
        """
        # 1. キャッシュチェック
        cached_host = self.cache.get(function_name)
        if cached_host:
            logger.debug(f"Cache hit for {function_name}: {cached_host}")
            return cached_host

        # 2. Singleflight: 進行中のリクエストがあれば待機
        if function_name in self._pending_requests:
            logger.debug(f"Singleflight: waiting for pending request for {function_name}")
            return await self._pending_requests[function_name]

        # 3. 自分が代表して問い合わせ
        loop = asyncio.get_running_loop()
        future: asyncio.Future[str] = loop.create_future()
        self._pending_requests[function_name] = future

        try:
            host = await self._fetch_from_manager(function_name, image, env)
            self.cache.set(function_name, host)
            if not future.done():
                future.set_result(host)
            return host
        except Exception as e:
            # 待機者に例外を伝播
            if not future.done():
                future.set_exception(e)
            # Future.exception() を呼ぶことで "never retrieved" 警告を抑制
            # (リーダーが例外を処理したことを示す)
            try:
                future.exception()
            except asyncio.InvalidStateError:
                pass
            raise
        finally:
            self._pending_requests.pop(function_name, None)

    async def _fetch_from_manager(
        self, function_name: str, image: Optional[str], env: Optional[Dict[str, str]]
    ) -> str:
        """Manager に問い合わせてホストを取得"""
        url = f"{config.MANAGER_URL}/containers/ensure"

        # モデルを作成
        request_model = ContainerEnsureRequest(
            function_name=function_name, image=image, env=env or {}
        )

        # X-Request-Id ヘッダーを伝播
        headers = {}
        request_id = get_request_id()
        if request_id:
            headers["X-Request-Id"] = request_id

        try:
            resp = await self.client.post(
                url,
                json=request_model.model_dump(),
                headers=headers,
                timeout=config.MANAGER_TIMEOUT,
            )
            resp.raise_for_status()

            # レスポンスをモデルでバリデーション
            response_model = ContainerInfoResponse.model_validate(resp.json())

            logger.debug(f"Fetched from Manager: {function_name} -> {response_model.host}")
            return response_model.host

        except httpx.TimeoutException as e:
            logger.error(f"Manager request timed out: {e}")
            raise ManagerTimeoutError(f"Container startup timeout for {function_name}") from e

        except httpx.RequestError as e:
            # 接続失敗 (ネットワークエラー、DNS解決失敗など)
            logger.error(f"Failed to connect to Manager: {e}")
            raise ManagerUnreachableError(e) from e

        except httpx.HTTPStatusError as e:
            # Manager からの HTTP エラーレスポンス
            status = e.response.status_code
            detail = e.response.text

            logger.error(f"Manager returned {status}: {detail}")

            # エラーコードマッピング
            if status == 404:
                raise FunctionNotFoundError(function_name) from e
            elif status in [400, 408, 409]:
                raise ManagerError(status, detail) from e
            else:
                raise ManagerError(status, detail) from e


# Backward compatibility (optional, or just remove)
async def get_lambda_host(
    function_name: str, image: Optional[str] = None, env: Optional[Dict[str, str]] = None
) -> str:
    """
    Deprecated: Use ManagerClient instead.
    Temporarily kept for un-refactored code in main.py if any.
    But we will refactor main.py to use ManagerClient.
    """
    async with httpx.AsyncClient() as client:
        manager = ManagerClient(client)
        return await manager.ensure_container(function_name, image, env)
