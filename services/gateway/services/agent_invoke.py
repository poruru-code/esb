import logging
from typing import Dict

import httpx

from services.common.models.internal import WorkerInfo
from services.gateway.pb import agent_pb2

logger = logging.getLogger("gateway.agent_invoke")


class AgentInvokeClient:
    """Invoke Lambda RIE via Agent (L7 proxy)."""

    def __init__(
        self,
        stub,
        owner_id: str,
        path: str = "/2015-03-31/functions/function/invocations",
    ):
        self.stub = stub
        self.path = path
        if not owner_id:
            raise ValueError("owner_id is required")
        self.owner_id = owner_id

    async def invoke(
        self,
        worker: WorkerInfo,
        payload: bytes,
        headers: Dict[str, str],
        timeout: float,
    ) -> httpx.Response:
        timeout_ms = int(timeout * 1000) if timeout and timeout > 0 else 0

        req = agent_pb2.InvokeWorkerRequest(  # type: ignore[attr-defined]
            container_id=worker.id,
            path=self.path,
            payload=payload,
            headers=headers,
            timeout_ms=timeout_ms,
            owner_id=self.owner_id,
        )

        resp = await self.stub.InvokeWorker(req)
        port = worker.port or 8080
        url = f"http://{worker.ip_address}:{port}{self.path}"
        request = httpx.Request("POST", url)
        return httpx.Response(
            status_code=resp.status_code,
            headers=resp.headers,
            content=resp.body,
            request=request,
        )
