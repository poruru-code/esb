import docker.errors
import time
import logging
import asyncio
from typing import Dict, Optional

import httpx
from .docker_adaptor import DockerAdaptor
from .config import config

logger = logging.getLogger("manager.service")


class ContainerManager:
    """
    Manages lifecycle of Lambda containers.
    """

    def __init__(self, network: Optional[str] = None):
        self.docker = DockerAdaptor()
        self.last_accessed: Dict[str, float] = {}
        # Per-container lock management
        self.locks: Dict[str, asyncio.Lock] = {}
        self._locks_lock = asyncio.Lock()

        # Use config or default to 'bridge' if not specified.
        self.network = network or config.CONTAINERS_NETWORK or "bridge"
        logger.info(f"ContainerManager initialized with network: {self.network}")

    async def ensure_container_running(
        self, name: str, image: Optional[str] = None, env: Optional[Dict[str, str]] = None
    ) -> str:
        """
        Ensures the container is running. Returns the hostname (container name) or IP.
        """
        self.last_accessed[name] = time.time()

        if image is None:
            image = f"{name}:latest"

        # Thread-safe acquisition of the per-container lock
        async with self._locks_lock:
            if name not in self.locks:
                self.locks[name] = asyncio.Lock()
            lock = self.locks[name]

        # Use name-based lock to prevent race conditions (TOCTOU)
        async with lock:
            try:
                container = await self.docker.get_container(name)

                if container.status == "running":
                    pass  # Already running

                elif container.status == "exited":
                    logger.info(f"Warm-up: Restarting container {name}...")
                    # container.start() is blocking? DockerAdaptor doesn't have start() yet?
                    # Adaptor should have start helpers or we use generic run_in_executor
                    # Let's assume container object methods are blocking.
                    loop = asyncio.get_running_loop()
                    await loop.run_in_executor(None, container.start)
                else:
                    logger.info(f"Container {name} in state {container.status}, removing...")
                    await self.docker.remove_container(container, force=True)
                    raise docker.errors.NotFound(f"Removed {name}")

            except docker.errors.NotFound:
                logger.info(f"Cold Start: Creating and starting container {name}...")

                # Configure Fluentd logging driver
                from docker.types import LogConfig

                log_config = LogConfig(
                    type=LogConfig.types.FLUENTD,
                    config={
                        "fluentd-address": "localhost:24224",
                        "tag": f"lambda.{name}",
                        "fluentd-async": "true",
                    },
                )

                try:
                    container = await self.docker.run_container(
                        image,
                        name=name,
                        detach=True,
                        environment=env or {},
                        network=self.network,
                        restart_policy={"Name": "no"},
                        labels={"created_by": "sample-dind"},
                        log_config=log_config,
                    )
                except docker.errors.APIError as e:
                    # 409 Conflict: コンテナが既に存在する（競合による作成）
                    if e.status_code == 409:
                        logger.warning(
                            f"Container {name} conflict detected (already exists). Adopting it."
                        )
                        # 既存コンテナを取得して続行
                        container = await self.docker.get_container(name)
                    else:
                        raise e

            # Reload container to get latest attributes (IP) and check readiness
            await self.docker.reload_container(container)
            try:
                ip = container.attrs["NetworkSettings"]["Networks"][self.network]["IPAddress"]
                if not ip:
                    # Fallback to name if IP is not yet assigned (rare in bridge network)
                    logger.warning(f"IP address not found for {name}. Falling back to hostname.")
                    ip = name
            except KeyError:
                logger.warning(
                    f"Network {self.network} not found for {name}. Falling back to host."
                )
                ip = name

            # Use IP address for readiness check to avoid DNS lag
            await self._wait_for_readiness(ip)
            return name

    async def _wait_for_readiness(
        self,
        host: str,
        port: int | None = None,
        timeout: int | None = None,
    ) -> None:
        """POST /invocations でRIEの起動を確認（AWS RIE標準方式）"""
        port = port or config.LAMBDA_PORT
        timeout = timeout or config.READINESS_TIMEOUT
        url = f"http://{host}:{port}/2015-03-31/functions/function/invocations"
        start = time.time()

        async with httpx.AsyncClient() as client:
            while time.time() - start < timeout:
                try:
                    response = await client.post(url, json={"ping": True}, timeout=1.0)
                    if response.status_code == 200:
                        return
                except (httpx.RequestError, httpx.TimeoutException):
                    await asyncio.sleep(0.5)

        logger.warning(f"Container {host} did not become ready in {timeout}s")

    async def stop_idle_containers(self, timeout_seconds: int = 900) -> None:
        now = time.time()
        to_remove = []

        for name, last_access in list(self.last_accessed.items()):  # Iterate copy
            if now - last_access > timeout_seconds:
                try:
                    logger.info(f"Scale-down: Stopping idle container {name}")
                    try:
                        container = await self.docker.get_container(name)
                        if container.status == "running":
                            await self.docker.stop_container(container)
                    except docker.errors.NotFound:
                        pass

                    to_remove.append(name)
                except Exception as e:
                    logger.error(f"Error stopping/checking {name}: {e}", exc_info=True)

        async with self._locks_lock:
            for name in to_remove:
                if name in self.locks:
                    del self.locks[name]
                self.last_accessed.pop(name, None)

        if to_remove:
            logger.info(f"Cleanup completed. Removed: {to_remove}")

    async def prune_managed_containers(self):
        """
        Kills and removes containers managed by this service (zombies).
        Now delegates to DockerAdaptor to avoid direct _client access.

        Deprecated: Use sync_with_docker() instead for graceful restart.
        """
        logger.info("Pruning zombie containers...")
        await self.docker.prune_containers()

    async def sync_with_docker(self):
        """
        Startup Logic:
        Docker上の既存コンテナを確認し、実行中のものは管理下(last_accessed)に戻し、
        停止中のものや異常なものはクリーンアップする。
        """
        logger.info("Syncing managed containers with Docker...")
        try:
            containers = await self.docker.list_containers(
                all=True, filters={"label": "created_by=sample-dind"}
            )

            now = time.time()
            synced_count = 0
            removed_count = 0

            for container in containers:
                try:
                    if container.status == "running":
                        # 管理下に復帰させる
                        self.last_accessed[container.name] = now
                        synced_count += 1
                        logger.debug(f"Adopted running container: {container.name}")
                    else:
                        # 停止しているコンテナは掃除
                        logger.info(
                            f"Removing stale container: {container.name} (status: {container.status})"
                        )
                        await self.docker.remove_container(container, force=True)
                        removed_count += 1
                except Exception as e:
                    logger.error(f"Error syncing container {container.name}: {e}")

            logger.info(f"Sync completed. Adopted: {synced_count}, Removed: {removed_count}")

        except Exception as e:
            logger.error(f"Failed to sync with Docker: {e}", exc_info=True)
