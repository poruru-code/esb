"""
Lambda Invoker Service

ManagerClientを通じてコンテナを起動し、Lambda RIEに対してInvokeリクエストを送信します。
boto3.client('lambda').invoke() 互換のエンドポイント用のビジネスロジック層です。
"""

import logging
import json
import base64
import httpx
from typing import Dict, Optional, TYPE_CHECKING
from services.common.core.request_context import get_trace_id
from services.gateway.services.function_registry import FunctionRegistry

if TYPE_CHECKING:
    from .pool_manager import PoolManager


from services.gateway.services.container_manager import ContainerManagerProtocol
from services.gateway.config import GatewayConfig
from services.gateway.core.circuit_breaker import CircuitBreaker, CircuitBreakerOpenError
from services.gateway.core.exceptions import (
    FunctionNotFoundError,
    ContainerStartError,
    LambdaExecutionError,
)


logger = logging.getLogger("gateway.lambda_invoker")


class LambdaInvoker:
    def __init__(
        self,
        client: httpx.AsyncClient,
        registry: FunctionRegistry,
        container_manager: ContainerManagerProtocol,
        config: GatewayConfig,
        pool_manager: Optional["PoolManager"] = None,
    ):
        """
        Args:
            client: Shared httpx.AsyncClient
            registry: FunctionRegistry instance
            container_manager: ContainerManagerProtocol instance (legacy mode)
            config: GatewayConfig instance
            pool_manager: Optional PoolManager for pool-based invocation
        """
        self.client = client
        self.registry = registry
        self.container_manager = container_manager
        self.config = config
        self.pool_manager = pool_manager
        # 関数名ごとのブレーカーを保持
        self.breakers: Dict[str, CircuitBreaker] = {}

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

        # Resolve Gateway URL using injected config
        gateway_internal_url = self.config.GATEWAY_INTERNAL_URL
        env["GATEWAY_INTERNAL_URL"] = gateway_internal_url

        # Inject _HANDLER env var for sitecustomize.py wrapper
        # This enables auto trace ID hydration via sitecustomize.py
        env.setdefault("_HANDLER", "lambda_function.lambda_handler")

        # Trace ID Propagation
        trace_id = get_trace_id()
        logger.debug(f"Trace ID in Invoker: {trace_id}")
        if trace_id:
            env["_X_AMZN_TRACE_ID"] = trace_id

        logger.debug(f"Passing env to manager for {function_name}: {env}")

        # === POOL MODE vs LEGACY MODE ===
        worker = None
        if self.pool_manager is not None:
            # Pool Mode: acquire worker from PoolManager
            try:
                worker = await self.pool_manager.acquire_worker(function_name)
                host = worker.ip_address
            except Exception as e:
                raise ContainerStartError(function_name, e) from e
        else:
            # Legacy Mode: use ContainerManager
            try:
                host = await self.container_manager.get_lambda_host(
                    function_name=function_name,
                    image=func_config.get("image"),
                    env=env,
                )
            except Exception as e:
                raise ContainerStartError(function_name, e) from e

        # POST to Lambda RIE
        rie_url = (
            f"http://{host}:{self.config.LAMBDA_PORT}/2015-03-31/functions/function/invocations"
        )
        logger.info(f"Invoking {function_name} at {rie_url} (trace_id: {trace_id})")

        # ブレーカー取得または作成
        if function_name not in self.breakers:
            self.breakers[function_name] = CircuitBreaker(
                failure_threshold=self.config.CIRCUIT_BREAKER_THRESHOLD,
                recovery_timeout=self.config.CIRCUIT_BREAKER_RECOVERY_TIMEOUT,
            )

        breaker = self.breakers[function_name]

        try:
            # ブレーカー経由で実行
            async def do_post():
                headers = {
                    "Content-Type": "application/json",
                }
                if trace_id:
                    # header value should be the full string (Root=...)
                    headers["X-Amzn-Trace-Id"] = trace_id

                    # RIE 対策: ClientContext に Trace ID を埋め込む
                    client_context = {"custom": {"trace_id": trace_id}}
                    json_ctx = json.dumps(client_context)
                    b64_ctx = base64.b64encode(json_ctx.encode("utf-8")).decode("utf-8")
                    headers["X-Amz-Client-Context"] = b64_ctx

                logger.debug(f"Sending request to RIE with headers: {headers}")

                response = await self.client.post(
                    rie_url,
                    content=payload,
                    headers=headers,
                    timeout=timeout,
                )

                # 判定: 回路を遮断すべき「失敗」かどうか
                is_failure = False
                if response.status_code >= 500:
                    is_failure = True
                elif response.headers.get("X-Amz-Function-Error"):
                    is_failure = True
                elif response.status_code == 200:
                    try:
                        if len(response.content) < 1024 * 10:
                            data = response.json()
                            if isinstance(data, dict) and (
                                "errorType" in data or "errorMessage" in data
                            ):
                                is_failure = True
                    except (ValueError, json.JSONDecodeError):
                        pass

                if is_failure:
                    if response.status_code >= 400:
                        response.raise_for_status()
                    else:
                        raise httpx.HTTPStatusError(
                            f"Lambda Logical Error: {response.text[:100]}",
                            request=response.request,
                            response=response,
                        )

                return response

            result = await breaker.call(do_post)
            return result

        except CircuitBreakerOpenError as e:
            logger.error(f"Circuit breaker open for {function_name}: {e}")
            raise LambdaExecutionError(function_name, "Circuit Breaker Open") from e
        except httpx.ConnectError as e:
            # Self-Healing: Evict dead worker on connection error
            logger.warning(f"Connection error to worker, evicting: {e}")
            if worker is not None and self.pool_manager is not None:
                await self.pool_manager.evict_worker(function_name, worker)
                worker = None  # prevent release in finally
            raise LambdaExecutionError(function_name, e) from e
        except (httpx.RequestError, httpx.HTTPStatusError) as e:
            logger.error(f"Lambda invocation failed for function '{function_name}': {e}")
            raise LambdaExecutionError(function_name, e) from e
        except Exception as e:
            logger.exception(f"Unexpected error during invocation of {function_name}: {e}")
            raise LambdaExecutionError(function_name, e) from e
        finally:
            # 常にプールへ返却 (evict済みの場合は worker=None)
            if worker is not None and self.pool_manager is not None:
                try:
                    await self.pool_manager.release_worker(function_name, worker)
                except Exception as e:
                    logger.error(f"Failed to release worker for {function_name}: {e}")


# Backward compatibility or helper if needed? No, we are fully refactoring to DI.
