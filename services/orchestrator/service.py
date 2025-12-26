import docker.errors
import time
import logging
import asyncio
import uuid
from typing import Dict, List, Optional

import httpx
from .docker_adaptor import DockerAdaptor
from .config import config
from services.common.core.http_client import HttpClientFactory
from services.common.models.internal import WorkerInfo

logger = logging.getLogger("orchestrator.service")

# ラベル用の短縮名（ブランド統一）
PROJECT_LABEL = "esb"


class ContainerOrchestrator:
    """
    Manages lifecycle of Lambda containers.
    """

    def __init__(self, network: Optional[str] = None):
        self.docker = DockerAdaptor()
        self.last_accessed: Dict[str, float] = {}
        # Per-container lock management
        self.locks: Dict[str, asyncio.Lock] = {}
        self._locks_lock = asyncio.Lock()

        # 共有 HTTP Client（startup() で初期化）
        self._http_factory = HttpClientFactory(config)
        self._http_client: Optional[httpx.AsyncClient] = None

        # Use config or default to 'bridge' if not specified.
        self.network = network or config.CONTAINERS_NETWORK or "bridge"
        logger.info(f"ContainerOrchestrator initialized with network: {self.network}")

    async def startup(self):
        """ライフサイクル開始時に呼び出し、共有 HTTP Client を初期化"""
        self._http_client = self._http_factory.create_async_client()
        logger.info("ContainerOrchestrator HTTP client initialized")

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
                # Cold start diagnostics
                logger.info(f"Cold Start: Creating and starting container {name}...")

                # Prevent log buffering
                env = env or {}
                env["PYTHONUNBUFFERED"] = "1"

                # VictoriaLogs URL を Lambda に注入（直接ログ送信用）
                import os

                vl_host = os.environ.get("VICTORIALOGS_HOST", "victorialogs")
                vl_port = os.environ.get("VICTORIALOGS_PORT", "9428")
                env["VICTORIALOGS_URL"] = f"http://{vl_host}:{vl_port}/insert/jsonline"

                # AWS Lambda 互換環境変数を設定（sitecustomize.py での動的ログ処理に使用）
                env["AWS_LAMBDA_FUNCTION_NAME"] = name

                logger.info(f"Environment variables for {name}: {env}")

                try:
                    container = await self.docker.run_container(
                        image,
                        name=name,
                        detach=True,
                        environment=env or {},
                        network=self.network,
                        restart_policy={"Name": "no"},
                        labels={"created_by": PROJECT_LABEL},
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
        timeout = timeout or int(config.CONTAINER_READINESS_TIMEOUT)
        url = f"http://{host}:{port}/2015-03-31/functions/function/invocations"
        start = time.time()

        # 共有クライアントがなければフォールバック
        if self._http_client is None:
            factory = HttpClientFactory(config)
            async with factory.create_async_client() as client:
                await self._poll_readiness(client, url, start, timeout, host)
        else:
            await self._poll_readiness(self._http_client, url, start, timeout, host)

    async def _poll_readiness(
        self,
        client: httpx.AsyncClient,
        url: str,
        start: float,
        timeout: int,
        host: str,
    ) -> None:
        """Readiness ポーリングの内部実装"""
        while time.time() - start < timeout:
            try:
                response = await client.post(url, json={"ping": True}, timeout=config.PING_TIMEOUT)
                if response.status_code == 200:
                    return
            except (httpx.RequestError, httpx.TimeoutException):
                await asyncio.sleep(config.READINESS_POLL_INTERVAL)

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

    async def stop_container(self, container_id: str) -> None:
        """
        指定されたコンテナを停止

        Gatewayからの明示的な終了要求(Draining)などで使用
        """
        try:
            # DockerAdaptor expects a Container object, not a string ID
            container = await self.docker.get_container(container_id)
            await self.docker.stop_container(container)
            await self.docker.remove_container(container, force=True)  # Ensure removal
        except docker.errors.NotFound:
            logger.warning(f"Container {container_id} not found during stop request.")
        except Exception as e:
            logger.error(f"Failed to stop container {container_id}: {e}")

        # last_accessed からの削除
        if container_id in self.last_accessed:
            del self.last_accessed[container_id]

    async def list_managed_containers(self) -> List[WorkerInfo]:
        """
        管理下の全コンテナ一覧を取得

        Gateway起動時のSync(Adoption)に使用
        """
        containers = await self.docker.list_containers(
            all=True, filters={"label": f"created_by={PROJECT_LABEL}"}
        )
        workers = []
        for c in containers:
            try:
                # ネットワーク情報の取得
                ip = (
                    c.attrs.get("NetworkSettings", {})
                    .get("Networks", {})
                    .get(self.network, {})
                    .get("IPAddress", c.name)
                )

                # 作成時間のパース (RFC3339)
                created_str = c.attrs.get("Created", "")
                created_at = 0.0
                if created_str:
                    try:
                        import iso8601

                        created_at = iso8601.parse_date(created_str).timestamp()
                    except ImportError:
                        pass  # iso8601 がない場合は 0.0 のまま

                workers.append(
                    WorkerInfo(
                        id=c.id,
                        name=c.name,
                        ip_address=ip,
                        port=config.LAMBDA_PORT,
                        created_at=created_at,
                        last_used_at=self.last_accessed.get(c.name, 0.0),
                    )
                )
            except Exception as e:
                logger.error(f"Failed to map container {c.name} to WorkerInfo: {e}")
        return workers

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
                all=True, filters={"label": f"created_by={PROJECT_LABEL}"}
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

    async def shutdown(self):
        """Clean up resources (HTTP client, thread pools, etc.)"""
        logger.info("Shutting down ContainerOrchestrator...")
        if self._http_client:
            await self._http_client.aclose()
            self._http_client = None
        self.docker.shutdown()

    # =========================================================================
    # Auto-Scaling Methods
    # =========================================================================

    async def provision_containers(
        self,
        function_name: str,
        count: int = 1,
        image: Optional[str] = None,
        env: Optional[Dict[str, str]] = None,
    ) -> List[WorkerInfo]:
        """
        指定された数のコンテナをプロビジョニング

        コンテナ名: lambda-{function_name}-{uuid4()[:8]}
        """
        workers: List[WorkerInfo] = []

        if image is None:
            image = f"{function_name}:latest"

        try:
            for _ in range(count):
                suffix = uuid.uuid4().hex[:8]
                container_name = f"lambda-{function_name}-{suffix}"

                # Cold start - 新規コンテナ作成
                logger.info(f"Provisioning container: {container_name}")

                container_env = (env or {}).copy()
                container_env["PYTHONUNBUFFERED"] = "1"

                # VictoriaLogs URL を Lambda に注入
                import os

                vl_host = os.environ.get("VICTORIALOGS_HOST", "victorialogs")
                vl_port = os.environ.get("VICTORIALOGS_PORT", "9428")
                container_env["VICTORIALOGS_URL"] = f"http://{vl_host}:{vl_port}/insert/jsonline"
                container_env["AWS_LAMBDA_FUNCTION_NAME"] = function_name

                container = await self.docker.run_container(
                    image,
                    name=container_name,
                    detach=True,
                    environment=container_env,
                    network=self.network,
                    restart_policy={"Name": "no"},
                    labels={"created_by": PROJECT_LABEL},
                )

                # Reload container to get IP
                await self.docker.reload_container(container)
                try:
                    ip = container.attrs["NetworkSettings"]["Networks"][self.network]["IPAddress"]
                    if not ip:
                        ip = container_name
                except KeyError:
                    ip = container_name

                # Wait for readiness
                await self._wait_for_readiness(ip)

                # Track last access
                self.last_accessed[container_name] = time.time()

                workers.append(
                    WorkerInfo(
                        id=container.id,
                        name=container_name,
                        ip_address=ip,
                        port=config.LAMBDA_PORT,
                        created_at=time.time(),
                    )
                )
        except Exception as e:
            logger.error(f"Provisioning failed for {function_name}: {e}")
            # Cleanup created containers in this batch
            for w in workers:
                try:
                    logger.info(f"Cleanup: Removing failed provision container {w.name}")
                    await self.docker.remove_container_by_name(w.name)
                    self.last_accessed.pop(w.name, None)
                except Exception as ce:
                    logger.warning(f"Cleanup failed for {w.name}: {ce}")
            raise

        return workers

    async def update_heartbeat(self, function_name: str, container_names: List[str]) -> None:
        """
        Gateway からの Heartbeat を受信

        受信したコンテナ名はアクティブとみなし、last_accessed を更新する。
        """
        now = time.time()
        for name in container_names:
            # 存在チェックをして更新 (Adoption も兼ねる)
            self.last_accessed[name] = now

        logger.debug(f"Heartbeat updated for {function_name}: {len(container_names)} containers")
