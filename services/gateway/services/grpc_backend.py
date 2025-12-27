import grpc
import logging

from services.common.models.internal import WorkerInfo
from services.gateway.pb import agent_pb2, agent_pb2_grpc
from services.gateway.core.exceptions import (
    OrchestratorUnreachableError,
    OrchestratorTimeoutError,
    ContainerStartError,
)

logger = logging.getLogger("gateway.grpc_backend")


class GrpcBackend:
    def __init__(self, agent_address: str):
        self.channel = grpc.aio.insecure_channel(agent_address)
        self.stub = agent_pb2_grpc.AgentServiceStub(self.channel)

    async def acquire_worker(self, function_name: str) -> WorkerInfo:
        """
        gRPC 経由でエージェントからワーカー（コンテナ）を取得
        """
        # TODO: Image/Env support
        req = agent_pb2.EnsureContainerRequest(
            function_name=function_name,
            image="",  # Phase 1: Agent side defaults to latest
            env={},
        )
        try:
            resp = await self.stub.EnsureContainer(req)
            return WorkerInfo(
                id=resp.id, name=resp.name, ip_address=resp.ip_address, port=resp.port
            )
        except grpc.RpcError as e:
            self._handle_grpc_error(e, function_name)

    async def release_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """
        ワーカーを返却（Agent側で管理するため、何もしない場合が多い）
        """
        pass

    async def evict_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """
        ワーカーを明示的に破壊
        """
        req = agent_pb2.DestroyContainerRequest(function_name=function_name, container_id=worker.id)
        try:
            await self.stub.DestroyContainer(req)
        except grpc.RpcError as e:
            # Eviction errors are logged but usually don't block
            logger.error(f"Failed to evict worker {worker.id}: {e}")

    def _handle_grpc_error(self, e: grpc.RpcError, function_name: str):
        code = e.code()
        if code == grpc.StatusCode.UNAVAILABLE:
            raise OrchestratorUnreachableError(e)
        elif code == grpc.StatusCode.DEADLINE_EXCEEDED:
            raise OrchestratorTimeoutError(str(e))
        elif code == grpc.StatusCode.RESOURCE_EXHAUSTED:
            raise ContainerStartError(function_name, e)
        else:
            logger.error(f"Unexpected gRPC error: {e}")
            raise e

    async def close(self):
        await self.channel.close()
