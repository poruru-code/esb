import httpx
import logging
from typing import List, Any
from services.common.models.internal import WorkerInfo

logger = logging.getLogger("gateway.clients")

class ProvisionClient:
    """Wrapper for Manager provision API"""

    def __init__(
        self,
        http_client: httpx.AsyncClient,
        manager_url: str,
        lambda_port: int,
        orchestrator_timeout: float,
        function_registry: Any,
    ):
        self.client = http_client
        self.manager_url = manager_url
        self.lambda_port = lambda_port
        self.orchestrator_timeout = orchestrator_timeout
        self.function_registry = function_registry

    async def provision(self, function_name: str) -> List[WorkerInfo]:
        """Provision a container and return WorkerInfo list"""
        func_config = self.function_registry.get_function_config(function_name)
        image = func_config.get("image") if func_config else None
        env = func_config.get("environment", {}) if func_config else {}

        response = await self.client.post(
            f"{self.manager_url}/containers/provision",
            json={
                "function_name": function_name,
                "count": 1,
                "image": image,
                "env": env,
            },
            timeout=self.orchestrator_timeout,
        )
        response.raise_for_status()
        data = response.json()
        return [
            WorkerInfo(
                id=w["id"],
                name=w["name"],
                ip_address=w["ip_address"],
                port=w.get("port", self.lambda_port),
                created_at=w.get("created_at", 0.0),
                last_used_at=w.get("last_used_at", 0.0),
            )
            for w in data["workers"]
        ]

    async def delete_container(self, container_id: str):
        """Delete a container"""
        url = f"{self.manager_url}/containers/{container_id}"
        response = await self.client.delete(url, timeout=self.orchestrator_timeout)
        response.raise_for_status()

    async def list_containers(self) -> List[WorkerInfo]:
        """List all managed containers"""
        url = f"{self.manager_url}/containers/sync"
        response = await self.client.get(url, timeout=self.orchestrator_timeout)
        response.raise_for_status()
        data = response.json()
        return [
            WorkerInfo(
                id=w["id"],
                name=w["name"],
                ip_address=w["ip_address"],
                port=w.get("port", self.lambda_port),
                created_at=w.get("created_at", 0.0),
                last_used_at=w.get("last_used_at", 0.0),
            )
            for w in data["containers"]
        ]

class HeartbeatClient:
    """Wrapper for Manager heartbeat API"""

    def __init__(self, http_client: httpx.AsyncClient, manager_url: str):
        self.client = http_client
        self.manager_url = manager_url

    async def heartbeat(self, function_name: str, container_names: list):
        await self.client.post(
            f"{self.manager_url}/containers/heartbeat",
            json={"function_name": function_name, "container_names": container_names},
            timeout=10.0,
        )
