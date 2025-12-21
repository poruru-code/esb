from typing import Optional, Dict
import httpx
import os
import logging
from .core.exceptions import (
    FunctionNotFoundError,
    ManagerError,
    ManagerTimeoutError,
    ManagerUnreachableError,
)
from services.common.core.request_context import get_request_id

logger = logging.getLogger("gateway.client")

MANAGER_URL = os.getenv("MANAGER_URL", "http://manager:8081")


class ManagerClient:
    def __init__(self, http_client: httpx.AsyncClient):
        self.client = http_client

    async def ensure_container(
        self, function_name: str, image: Optional[str] = None, env: Optional[Dict[str, str]] = None
    ) -> str:
        """
        Calls Manager Service to ensure container is running and get its host/IP.

        Raises:
            FunctionNotFoundError: 関数/イメージが存在しない (404)
            ManagerError: Docker API エラーなど (400, 409など)
            ManagerTimeoutError: タイムアウト (408)
            ManagerUnreachableError: Manager への接続失敗
        """
        url = f"{MANAGER_URL}/containers/ensure"
        payload = {"function_name": function_name, "image": image, "env": env or {}}

        # X-Request-Id ヘッダーを伝播
        headers = {}
        request_id = get_request_id()
        if request_id:
            headers["X-Request-Id"] = request_id

        try:
            resp = await self.client.post(
                url,
                json=payload,
                headers=headers,
                timeout=30.0,
            )
            resp.raise_for_status()
            data = resp.json()
            return data["host"]

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
                # 400: Docker API エラー
                # 408: タイムアウト
                # 409: コンテナ競合
                raise ManagerError(status, detail) from e
            else:
                # その他のエラー
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
