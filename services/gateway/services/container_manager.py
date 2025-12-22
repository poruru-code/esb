from typing import Protocol, Dict, Optional
import httpx
import logging
from ..config import GatewayConfig
from ..core.exceptions import (
    FunctionNotFoundError,
    ManagerError,
    ManagerTimeoutError,
    ManagerUnreachableError,
)
from services.common.models.internal import ContainerEnsureRequest, ContainerInfoResponse
from services.common.core.request_context import get_request_id

logger = logging.getLogger("gateway.container_manager")


class ContainerManagerProtocol(Protocol):
    async def get_lambda_host(
        self, function_name: str, image: Optional[str], env: Dict[str, str]
    ) -> str: ...


class HttpContainerManager:
    """ManagerサービスとHTTP通信を行う実装"""

    def __init__(self, config: GatewayConfig, client: httpx.AsyncClient):
        self.config = config
        self.client = client

    async def get_lambda_host(
        self, function_name: str, image: Optional[str], env: Dict[str, str]
    ) -> str:
        url = f"{self.config.MANAGER_URL}/containers/ensure"

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
                timeout=self.config.MANAGER_TIMEOUT,
            )
            resp.raise_for_status()

            # レスポンスをモデルでバリデーション
            response_model = ContainerInfoResponse.model_validate(resp.json())
            return response_model.host

        except httpx.TimeoutException as e:
            logger.error(f"Manager request timed out: {e}")
            raise ManagerTimeoutError(f"Container startup timeout for {function_name}") from e

        except httpx.RequestError as e:
            # 接続失敗
            logger.error(f"Failed to connect to Manager: {e}")
            raise ManagerUnreachableError(e) from e

        except httpx.HTTPStatusError as e:
            # Manager からの HTTP エラーレスポンス
            status = e.response.status_code
            detail = e.response.text

            logger.error(f"Manager returned {status}: {detail}")

            if status == 404:
                raise FunctionNotFoundError(function_name) from e
            elif status in [400, 408, 409]:
                raise ManagerError(status, detail) from e
            else:
                raise ManagerError(status, detail) from e
