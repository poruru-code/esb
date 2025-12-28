import grpc
import logging
from typing import List, Optional

from services.common.models.internal import WorkerInfo
from services.gateway.pb import agent_pb2, agent_pb2_grpc
from services.gateway.core.exceptions import (
    OrchestratorUnreachableError,
    OrchestratorTimeoutError,
    ContainerStartError,
)
from services.gateway.services.lambda_invoker import WorkerState
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.core.concurrency import ConcurrencyManager

logger = logging.getLogger(__name__)


class GrpcBackend:
    def __init__(
        self,
        agent_address: str,
        function_registry: Optional[FunctionRegistry] = None,
        concurrency_manager: Optional[ConcurrencyManager] = None,
    ):
        self.channel = grpc.aio.insecure_channel(agent_address)
        self.stub = agent_pb2_grpc.AgentServiceStub(self.channel)
        self.function_registry = function_registry
        self.concurrency_manager = concurrency_manager

    async def acquire_worker(self, function_name: str) -> WorkerInfo:
        """
        Acquire a worker (container) from the agent via gRPC
        (with flow control applied).
        """
        if self.concurrency_manager:
            throttle = self.concurrency_manager.get_throttle(function_name)
            await throttle.acquire()

        try:
            worker = await self._ensure_container(function_name)
            logger.info(f"Acquired worker {worker.id} at {worker.ip_address} for {function_name}")
            # Readiness Check: Wait for port 8080 to be available
            port = worker.port or 8080
            await self._wait_for_readiness(function_name, worker.ip_address, port)
            logger.debug(f"Readiness check passed for {worker.ip_address}:{port}")
            return worker
        except Exception:
            if self.concurrency_manager:
                throttle = self.concurrency_manager.get_throttle(function_name)
                await throttle.release()
            raise

    async def _wait_for_readiness(
        self, function_name: str, host: str, port: int, timeout: float = 10.0
    ):
        """Confirm readiness by attempting to establish a TCP connection."""
        import asyncio
        import time

        start_time = time.time()
        last_error = None
        while time.time() - start_time < timeout:
            try:
                # Check TCP connectivity with asyncio.open_connection.
                _, writer = await asyncio.wait_for(asyncio.open_connection(host, port), timeout=1.0)
                writer.close()
                await writer.wait_closed()
                return
            except (asyncio.TimeoutError, ConnectionRefusedError, OSError) as e:
                last_error = e
                await asyncio.sleep(0.1)

        raise ContainerStartError(
            function_name,
            last_error
            or Exception(f"Port {port} on {host} did not become ready within {timeout}s"),
        )

    async def _ensure_container(self, function_name: str) -> WorkerInfo:
        # Get environment variables from FunctionRegistry
        env = {}
        image = ""
        if self.function_registry:
            func_config = self.function_registry.get_function_config(function_name)
            if func_config:
                env = func_config.get("environment", {})
                image = func_config.get("image", "")

        req = agent_pb2.EnsureContainerRequest(
            function_name=function_name,
            image=image,
            env=env,
        )
        try:
            resp = await self.stub.EnsureContainer(req)
            logger.debug(f"Agent EnsureContainer response: {resp.id} / {resp.ip_address}")
            return WorkerInfo(
                id=resp.id, name=resp.name, ip_address=resp.ip_address, port=resp.port
            )
        except grpc.RpcError as e:
            self._handle_grpc_error(e, function_name)

    async def release_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """
        Release a worker.
        """
        if self.concurrency_manager:
            throttle = self.concurrency_manager.get_throttle(function_name)
            await throttle.release()

    async def evict_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """
        Explicitly evict a worker.
        """
        req = agent_pb2.DestroyContainerRequest(function_name=function_name, container_id=worker.id)
        try:
            await self.stub.DestroyContainer(req)
        except grpc.RpcError as e:
            # Eviction errors are logged but usually don't block
            logger.error(f"Failed to evict worker {worker.id}: {e}")

    async def list_workers(self) -> List[WorkerState]:
        """
        Get the state of all workers from Agent (for Janitor).
        """
        req = agent_pb2.ListContainersRequest()
        try:
            resp = await self.stub.ListContainers(req)
            return [
                WorkerState(
                    container_id=c.container_id,
                    function_name=c.function_name,
                    status=c.status,
                    last_used_at=c.last_used_at,
                )
                for c in resp.containers
            ]
        except grpc.RpcError as e:
            logger.error(f"Failed to list workers: {e}")
            return []

    def _handle_grpc_error(self, e: grpc.RpcError, function_name: str):
        code = e.code()
        # gRPC aio errors often have a .details() method
        details = getattr(e, "details", lambda: str(e))()

        if code == grpc.StatusCode.UNAVAILABLE:
            raise OrchestratorUnreachableError(f"Agent unavailable: {details}")
        elif code == grpc.StatusCode.DEADLINE_EXCEEDED:
            raise OrchestratorTimeoutError(f"Agent request timed out: {details}")
        elif code == grpc.StatusCode.RESOURCE_EXHAUSTED:
            raise ContainerStartError(function_name, f"Agent resource exhausted: {details}")
        else:
            logger.error(f"Unexpected gRPC error: {code} - {details}")
            raise e

    async def close(self):
        await self.channel.close()
