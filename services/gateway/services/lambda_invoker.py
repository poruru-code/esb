"""
Lambda Invoker Service.

Acquires workers via the InvocationBackend strategy and sends invoke requests to Lambda RIE.
Business logic layer for boto3.client('lambda').invoke()-compatible endpoints.
"""

import base64
import json
import logging
from dataclasses import dataclass
from typing import Dict, List, Optional, Protocol

import httpx
from grpc import StatusCode
from grpc.aio import AioRpcError

from services.common.core.request_context import get_trace_id
from services.common.models.internal import WorkerInfo
from services.gateway.config import GatewayConfig
from services.gateway.core.circuit_breaker import CircuitBreaker
from services.gateway.core.exceptions import ContainerStartError
from services.gateway.models.result import InvocationResult
from services.gateway.services.agent_invoke import AgentInvokeClient
from services.gateway.services.function_registry import FunctionRegistry

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
        agent_invoker: Optional[AgentInvokeClient] = None,
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
        self.agent_invoker = agent_invoker
        # Store per-function breakers.
        self.breakers: Dict[str, CircuitBreaker] = {}

    async def invoke_function(
        self, function_name: str, payload: bytes, timeout: int | float = 300
    ) -> InvocationResult:
        """
        Invoke the specified Lambda using the composed method pattern.
        """
        func_entity = self.registry.get_function_config(function_name)
        if not func_entity:
            # Fallback for 404
            return InvocationResult(
                success=False, status_code=404, error=f"Function {function_name} not found"
            )

        breaker = self._get_breaker(function_name)
        trace_id = get_trace_id()
        worker: Optional[WorkerInfo] = None
        worker_evicted = False
        retry_attempted = False

        try:
            # 1. Resource Acquisition
            worker = await self._acquire_worker(function_name)

            # 2. Execution with Resilience
            async def do_invoke() -> InvocationResult:
                nonlocal worker, worker_evicted, retry_attempted
                headers = self._prepare_headers(trace_id)
                try:
                    if worker is None:
                        raise RuntimeError("worker is not available for invocation") from None
                    response = await self._execute_call(worker, payload, headers, timeout)
                    return self._process_response(response)
                except Exception as e:
                    if retry_attempted or not self._should_retry(e):
                        raise
                    retry_attempted = True
                    if worker:
                        await self.backend.evict_worker(function_name, worker)
                    worker_evicted = True
                    worker = None
                    worker = await self._acquire_worker(function_name)
                    worker_evicted = False
                    if worker is None:
                        raise RuntimeError("worker is not available for invocation") from None
                    response = await self._execute_call(worker, payload, headers, timeout)
                    return self._process_response(response)

            return await breaker.call(do_invoke)

        except Exception as e:
            result, evicted = await self._handle_error(e, worker, function_name)
            worker_evicted = evicted
            return result
        finally:
            if worker and not worker_evicted:
                await self._release_worker(function_name, worker)

    async def _acquire_worker(self, function_name: str) -> WorkerInfo:
        """Acquire a worker from the backend."""
        try:
            return await self.backend.acquire_worker(function_name)
        except Exception as e:
            raise ContainerStartError(function_name, e) from e

    def _prepare_headers(self, trace_id: Optional[str]) -> Dict[str, str]:
        """Prepare RIE compatible headers."""
        headers = {"Content-Type": "application/json"}
        if trace_id:
            headers["X-Amzn-Trace-Id"] = trace_id
            # RIE workaround: embed Trace ID in ClientContext
            client_context = {"custom": {"trace_id": trace_id}}
            json_ctx = json.dumps(client_context)
            b64_ctx = base64.b64encode(json_ctx.encode("utf-8")).decode("utf-8")
            headers["X-Amz-Client-Context"] = b64_ctx
        return headers

    async def _execute_call(
        self, worker: WorkerInfo, payload: bytes, headers: Dict[str, str], timeout: float
    ) -> httpx.Response:
        """Directly POST to the worker (RIE or Agent)."""
        if self.agent_invoker:
            return await self.agent_invoker.invoke(
                worker=worker, payload=payload, headers=headers, timeout=timeout
            )

        host = worker.ip_address
        port = worker.port or self.config.LAMBDA_PORT
        rie_url = f"http://{host}:{port}/2015-03-31/functions/function/invocations"
        return await self.client.post(rie_url, content=payload, headers=headers, timeout=timeout)

    def _process_response(self, response: httpx.Response) -> InvocationResult:
        """Transform HTTP response into InvocationResult and detect logical errors."""
        is_failure = False
        error_msg = None

        if response.status_code >= 500:
            is_failure = True
            error_msg = f"Server Error: {response.status_code}"
        elif response.headers.get("X-Amz-Function-Error"):
            is_failure = True
            error_msg = f"Function Error: {response.headers.get('X-Amz-Function-Error')}"
        elif response.status_code == 200:
            # Check for logical errors in 200 OK (unhandled exceptions in Lambda)
            try:
                if len(response.content) < 10240:  # 10KB sanity check
                    data = response.json()
                    if isinstance(data, dict) and ("errorType" in data or "errorMessage" in data):
                        is_failure = True
                        error_msg = data.get("errorMessage", data.get("errorType"))
            except (ValueError, json.JSONDecodeError):
                pass

        if is_failure:
            # We raise here because the circuit breaker needs an exception to count failures.
            # InvocationResult itself can represent failure, but breaker.call expects Exception.
            # However, for consistency with the new pattern, we can also return it if NOT
            # using breaker.
            # But the plan says: "breaker needs it".
            raise httpx.HTTPStatusError(
                f"Lambda Logic Error: {error_msg}",
                request=response.request,
                response=response,
            )

        return InvocationResult(
            success=True,
            status_code=response.status_code,
            payload=response.content,
            headers=dict(response.headers),
            multi_headers={k: response.headers.get_list(k) for k in response.headers.keys()},
        )

    async def _handle_error(
        self, e: Exception, worker: Optional[WorkerInfo], function_name: str
    ) -> tuple[InvocationResult, bool]:
        """Centralized error handling with self-healing."""
        logger.error(
            f"Invocation error for {function_name}: {e}",
            extra={
                "function_name": function_name,
                "error_type": type(e).__name__,
                "error_detail": str(e),
                "worker_id": worker.id if worker else None,
                "target_url": f"http://{worker.ip_address}:{worker.port}" if worker else None,
            },
        )

        worker_evicted = False
        # Self-healing: Evict worker on connection or infrastructure errors
        if isinstance(e, (httpx.ConnectError, AioRpcError)) and worker:
            await self.backend.evict_worker(function_name, worker)
            worker_evicted = True

        is_retryable = isinstance(e, (httpx.ConnectError, ContainerStartError))
        if isinstance(e, AioRpcError) and self._is_retryable_grpc_error(e):
            is_retryable = True

        return (
            InvocationResult(
                success=False,
                status_code=502 if not isinstance(e, ContainerStartError) else 503,
                error=str(e),
                is_retryable=is_retryable,
            ),
            worker_evicted,
        )

    async def _release_worker(self, function_name: str, worker: WorkerInfo):
        """Release worker back to pool."""
        try:
            # If the worker was evicted in _handle_error, this will be bypassed
            # if we manage the local 'worker' variable correctly in invoke_function.
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

    def _should_retry(self, error: Exception) -> bool:
        if isinstance(error, httpx.ConnectError):
            return True
        if isinstance(error, AioRpcError):
            return self._is_retryable_grpc_error(error)
        return False

    @staticmethod
    def _is_retryable_grpc_error(error: AioRpcError) -> bool:
        return error.code() in {
            StatusCode.UNAVAILABLE,
            StatusCode.DEADLINE_EXCEEDED,
            StatusCode.NOT_FOUND,
        }
