import logging
from typing import List, Any
from services.common.models.internal import WorkerInfo
from services.gateway.pb import agent_pb2

logger = logging.getLogger("gateway.grpc_provision")


class GrpcProvisionClient:
    """gRPC implementation for Agent provisioning, compatible with PoolManager's ProvisionClient interface."""

    def __init__(
        self,
        stub,  # AgentServiceStub
        function_registry: Any,
    ):
        self.stub = stub
        self.function_registry = function_registry

    async def provision(self, function_name: str) -> List[WorkerInfo]:
        """Provision a container via gRPC Agent and return WorkerInfo list"""
        func_config = self.function_registry.get_function_config(function_name)
        func_config = self.function_registry.get_function_config(function_name)
        image = func_config.get("image") if func_config else None

        logger.info(f"Provisioning via gRPC Agent: {function_name}")

        from services.gateway.config import config

        # Base env from function config
        env = func_config.get("environment", {}) if func_config else {}

        # Inject RIE & Observability Variables
        env["AWS_LAMBDA_FUNCTION_NAME"] = function_name
        env["AWS_LAMBDA_FUNCTION_VERSION"] = "$LATEST"
        env["AWS_REGION"] = env.get("AWS_REGION", "ap-northeast-1")

        if config.VICTORIALOGS_URL:
            env["VICTORIALOGS_URL"] = config.VICTORIALOGS_URL

        # Inject LOG_LEVEL for sitecustomize.py logging filter
        import os

        log_level = os.environ.get("LOG_LEVEL", "INFO")
        env["LOG_LEVEL"] = log_level

        # Inject GATEWAY_INTERNAL_URL for chain invocations
        if config.GATEWAY_INTERNAL_URL:
            env["GATEWAY_INTERNAL_URL"] = config.GATEWAY_INTERNAL_URL

        # Inject Timeout & Memory from config
        if func_config:
            if "timeout" in func_config:
                env["AWS_LAMBDA_FUNCTION_TIMEOUT"] = str(func_config["timeout"])
            if "memory_size" in func_config:
                env["AWS_LAMBDA_FUNCTION_MEMORY_SIZE"] = str(func_config["memory_size"])

        req = agent_pb2.EnsureContainerRequest(
            function_name=function_name,
            image=image or "",
            env=env,
        )

        try:
            resp = await self.stub.EnsureContainer(req)
            worker = WorkerInfo(
                id=resp.id,
                name=resp.name,
                ip_address=resp.ip_address,
                port=resp.port or 8080,
                created_at=0.0,
                last_used_at=0.0,
            )

            # Readiness Check: Wait for port 8080 to be available
            await self._wait_for_readiness(function_name, worker.ip_address, worker.port)

            return [worker]
        except Exception as e:
            # gRPC RpcError details extraction
            if hasattr(e, "details"):
                details = e.details()
                logger.error(f"Failed to provision via Agent: {e} (Details: {details})")
            else:
                logger.error(f"Failed to provision via Agent: {e}")
            raise

    async def _wait_for_readiness(
        self, function_name: str, host: str, port: int, timeout: float = 10.0
    ):
        """Confirm readiness by attempting to establish a TCP connection."""
        import asyncio
        import time
        from services.gateway.core.exceptions import ContainerStartError

        start_time = time.time()
        last_error = None
        while time.time() - start_time < timeout:
            try:
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

    async def delete_container(self, container_id: str):
        """Delete a container via gRPC Agent"""
        req = agent_pb2.DestroyContainerRequest(container_id=container_id)
        try:
            await self.stub.DestroyContainer(req)
        except Exception as e:
            logger.error(f"Failed to delete container {container_id} via Agent: {e}")
            raise

    async def list_containers(self) -> List[WorkerInfo]:
        """List all containers via gRPC Agent"""
        req = agent_pb2.ListContainersRequest()
        try:
            resp = await self.stub.ListContainers(req)
            return [
                WorkerInfo(
                    id=c.container_id,
                    name=c.container_name,
                    ip_address="",
                    port=8080,
                    created_at=float(c.created_at),
                    last_used_at=c.last_used_at,
                )
                for c in resp.containers
            ]
        except Exception as e:
            logger.error(f"Failed to list containers via Agent: {e}")
            return []
