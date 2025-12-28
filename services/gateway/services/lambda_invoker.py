"""
Lambda Invoker Service.

Acquires workers via the InvocationBackend strategy and sends invoke requests to Lambda RIE.
Business logic layer for boto3.client('lambda').invoke()-compatible endpoints.
"""

import logging
import json
import base64
import httpx
from typing import Dict, Optional, Protocol, List
from dataclasses import dataclass
from services.common.core.request_context import get_trace_id
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.config import GatewayConfig
from services.gateway.core.circuit_breaker import CircuitBreaker, CircuitBreakerOpenError
from services.gateway.core.exceptions import (
    ContainerStartError,
    LambdaExecutionError,
)
from services.common.models.internal import WorkerInfo

logger = logging.getLogger("gateway.lambda_invoker")


@dataclass
class WorkerState:
    """Worker state for Janitor inspection"""

    container_id: str
    function_name: str
    status: str  # "RUNNING", "PAUSED", "STOPPED", "UNKNOWN"
    last_used_at: int  # Unix timestamp in seconds


class InvocationBackend(Protocol):
    """
    Abstract interface for execution backends.
    Implemented by PoolManager (Python) and future AgentClient (Go/gRPC).
    """

    async def acquire_worker(self, function_name: str) -> WorkerInfo:
        """Acquire a worker for function execution."""
        ...

    async def release_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """Release a worker."""
        ...

    async def evict_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """Evict a worker."""
        ...

    async def list_workers(self) -> List[WorkerState]:
        """Get state of all workers (for Janitor)."""
        ...


class LambdaInvoker:
    def __init__(
        self,
        client: httpx.AsyncClient,
        registry: FunctionRegistry,
        config: GatewayConfig,
        backend: InvocationBackend,
    ):
        """
        Args:
            client: Shared httpx.AsyncClient
            registry: FunctionRegistry instance
            config: GatewayConfig instance
            backend: InvocationBackend implementing Strategy
        """
        self.client = client
        self.registry = registry
        self.config = config
        self.backend = backend
        # Store per-function breakers.
        self.breakers: Dict[str, CircuitBreaker] = {}

    async def invoke_function(
        self, function_name: str, payload: bytes, timeout: int = 300
    ) -> httpx.Response:
        """Invoke the specified Lambda."""
        func_config = self.registry.get_function_config(function_name)
        if not func_config:
            raise LambdaExecutionError(function_name, "Function not found in registry")

        # Circuit Breaker (State management is done inside breaker.call)
        breaker = self._get_breaker(function_name)

        # Trace ID Propagation
        trace_id = get_trace_id()
        logger.debug(f"Trace ID in Invoker: {trace_id}")

        worker: Optional[WorkerInfo] = None
        try:
            # 1. Acquire worker from backend (strategy pattern).
            try:
                worker = await self.backend.acquire_worker(function_name)
                host = worker.ip_address
                port = worker.port or self.config.LAMBDA_PORT
            except Exception as e:
                raise ContainerStartError(function_name, e) from e

            # 2. POST to Lambda RIE
            rie_url = f"http://{host}:{port}/2015-03-31/functions/function/invocations"
            logger.info(f"Invoking {function_name} at {rie_url} (trace_id: {trace_id})")

            # 3. Execute request via breaker.
            async def do_post():
                headers = {
                    "Content-Type": "application/json",
                }
                if trace_id:
                    headers["X-Amzn-Trace-Id"] = trace_id
                    # RIE workaround: embed Trace ID in ClientContext.
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

                # Determine whether the response counts as a failure.
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
            logger.error(
                f"Lambda invocation failed for function '{function_name}': {e}",
                extra={
                    "function_name": function_name,
                    "target_url": rie_url,
                    "error_type": type(e).__name__,
                    "error_detail": str(e),
                },
            )
            if worker is not None:
                await self.backend.evict_worker(function_name, worker)
                worker = None  # prevent release in finally
            raise LambdaExecutionError(function_name, e) from e
        except (httpx.RequestError, httpx.HTTPStatusError) as e:
            logger.error(
                f"Lambda invocation failed for function '{function_name}': {e}",
                extra={
                    "function_name": function_name,
                    "target_url": rie_url,
                    "error_type": type(e).__name__,
                    "error_detail": str(e),
                },
            )
            raise LambdaExecutionError(function_name, e) from e
        except Exception as e:
            logger.exception(
                f"Unexpected error during invocation of {function_name}: {e}",
                extra={
                    "function_name": function_name,
                    "target_url": rie_url if "rie_url" in locals() else "N/A",
                    "error_type": type(e).__name__,
                    "error_detail": str(e),
                },
            )
            raise LambdaExecutionError(function_name, e) from e
        finally:
            # Always return to pool (worker=None if evicted).
            if worker is not None:
                try:
                    await self.backend.release_worker(function_name, worker)
                except Exception as e:
                    logger.error(f"Failed to release worker for {function_name}: {e}")

    def _get_breaker(self, function_name: str) -> CircuitBreaker:
        """Get or create a circuit breaker per function."""
        if function_name not in self.breakers:
            self.breakers[function_name] = CircuitBreaker(
                failure_threshold=self.config.CIRCUIT_BREAKER_THRESHOLD,
                recovery_timeout=self.config.CIRCUIT_BREAKER_RECOVERY_TIMEOUT,
            )
        return self.breakers[function_name]
