"""
Lambda Invoker Service

ManagerClientを通じてコンテナを起動し、Lambda RIEに対してInvokeリクエストを送信します。
boto3.client('lambda').invoke() 互換のエンドポイント用のビジネスロジック層です。
"""

import logging
import httpx
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.core.exceptions import (
    FunctionNotFoundError,
    ContainerStartError,
    LambdaExecutionError,
)
from ..client import get_lambda_host
from ..config import config

logger = logging.getLogger("gateway.lambda_invoker")


class LambdaInvoker:
    def __init__(self, client: httpx.AsyncClient, registry: FunctionRegistry):
        """
        Args:
            client: Shared httpx.AsyncClient
            registry: FunctionRegistry instance
        """
        self.client = client
        self.registry = registry

    async def invoke_function(
        self, function_name: str, payload: bytes, timeout: int = 300
    ) -> httpx.Response:
        """
        Lambda関数を呼び出す

        Args:
            function_name: 呼び出す関数名
            payload: リクエストボディ
            timeout: リクエストタイムアウト

        Returns:
            Lambda RIEからのレスポンス

        Raises:
            ContainerStartError: コンテナ起動失敗
            LambdaExecutionError: Lambda実行失敗
        """
        # config check
        func_config = self.registry.get_function_config(function_name)
        if func_config is None:
            raise FunctionNotFoundError(function_name)

        # Prepare env
        env = func_config.get("environment", {}).copy()

        # Resolve Gateway URL (Simple approach for now)
        gateway_internal_url = config.GATEWAY_INTERNAL_URL
        env["GATEWAY_INTERNAL_URL"] = gateway_internal_url

        # Ensure container (via Manager)
        try:
            host = await get_lambda_host(
                function_name=function_name,
                image=func_config.get("image"),
                env=env,
            )
        except Exception as e:
            raise ContainerStartError(function_name, e) from e

        # POST to Lambda RIE
        rie_url = f"http://{host}:{config.LAMBDA_PORT}/2015-03-31/functions/function/invocations"
        logger.info(f"Invoking {function_name} at {rie_url}")

        try:
            response = await self.client.post(
                rie_url,
                content=payload,
                headers={"Content-Type": "application/json"},
                timeout=timeout,
            )
            return response
        except httpx.RequestError as e:
            raise LambdaExecutionError(function_name, e) from e


# Backward compatibility or helper if needed? No, we are fully refactoring to DI.
