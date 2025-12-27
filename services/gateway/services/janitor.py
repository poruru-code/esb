"""
HeartbeatJanitor - Periodic heartbeat sender from Gateway to Manager

Keeps Manager informed of active containers to prevent zombie cleanup.
"""

import asyncio
import logging
import time
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .pool_manager import PoolManager
    from .grpc_backend import GrpcBackend

logger = logging.getLogger("gateway.janitor")


class HeartbeatJanitor:
    """
    Gateway → Manager への定期的な Heartbeat 送信

    保持しているワーカーIDリストを送信し、
    Manager側でOrphanコンテナを検出・削除させる。
    """

    def __init__(
        self,
        pool_manager: "PoolManager",
        manager_client,  # ManagerClient or mock
        interval: int = 30,
        idle_timeout: float = 300.0,
    ):
        self.pool_manager = pool_manager
        self.manager_client = manager_client
        self.interval = interval
        self.idle_timeout = idle_timeout
        self._task: asyncio.Task | None = None

    async def start(self) -> None:
        """Heartbeat Loop 開始"""
        self._task = asyncio.create_task(self._loop())
        logger.info(
            f"Heartbeat Janitor started (interval: {self.interval}s, idle_timeout: {self.idle_timeout}s)"
        )

    async def stop(self) -> None:
        """Heartbeat Loop 停止"""
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
        logger.info("Heartbeat Janitor stopped")

    async def _loop(self) -> None:
        """定期実行ループ"""
        while True:
            try:
                await asyncio.sleep(self.interval)
                await self._send_heartbeat()
            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Heartbeat failed: {e}")

    async def _send_heartbeat(self) -> None:
        """Pruning 後に Heartbeat 送信"""
        # 1. まず Pruning を実行
        try:
            pruned = await self.pool_manager.prune_all_pools(self.idle_timeout)
            for fname, workers in pruned.items():
                logger.info(f"Pruned {len(workers)} idle workers from {fname}")
        except Exception as e:
            logger.error(f"Pruning failed: {e}")

        # 2. 残っているワーカーの名前リストを送信 (Manager がいる場合のみ)
        if self.manager_client:
            worker_names = self.pool_manager.get_all_worker_names()
            for function_name, names in worker_names.items():
                if names:  # Only send if there are workers
                    await self.manager_client.heartbeat(function_name, names)
                    logger.debug(f"Heartbeat sent: {function_name} ({len(names)} workers)")


class ResourceJanitor:
    """
    Phase 3: gRPC Agent 用のリソース管理 Janitor

    - cleanup_on_startup(): Gateway 起動時に Paused コンテナを一掃
    - run_loop(): 定期的にアイドルコンテナを削除
    """

    def __init__(
        self,
        backend: "GrpcBackend",
        idle_timeout: int = 600,  # seconds
        cleanup_interval: int = 60,  # seconds
    ):
        self.backend = backend
        self.idle_timeout = idle_timeout
        self.cleanup_interval = cleanup_interval
        self._task: asyncio.Task | None = None

    async def cleanup_on_startup(self) -> int:
        """
        Gateway 起動時のクリーンアップ

        Option B: Paused なコンテナを全て削除する
        Gateway 再起動後、メモリ上の状態は消失しているため
        Paused コンテナは再利用できない (ゾンビ扱い)

        Returns:
            削除したコンテナ数
        """
        logger.info("Starting startup cleanup...")
        workers = await self.backend.list_workers()
        removed_count = 0

        for w in workers:
            if w.status == "PAUSED":
                try:
                    from services.common.models.internal import WorkerInfo

                    worker_info = WorkerInfo(
                        id=w.container_id,
                        name=w.function_name,
                        ip_address="",
                        port=0,
                    )
                    await self.backend.evict_worker(w.function_name, worker_info)
                    removed_count += 1
                    logger.info(f"Startup cleanup: removed paused container {w.container_id}")
                except Exception as e:
                    # Don't stop the loop on individual failures
                    logger.error(f"Failed to remove container {w.container_id}: {e}")

        logger.info(f"Startup cleanup completed: removed {removed_count} containers")
        return removed_count

    async def start(self) -> None:
        """定期クリーンアップループ開始"""
        self._task = asyncio.create_task(self._run_loop())
        logger.info(
            f"Resource Janitor started (interval: {self.cleanup_interval}s, idle_timeout: {self.idle_timeout}s)"
        )

    async def stop(self) -> None:
        """定期クリーンアップループ停止"""
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
        logger.info("Resource Janitor stopped")

    async def _run_loop(self) -> None:
        """
        定期実行ループ

        PAUSED かつ idle_timeout を超過したコンテナを削除する
        """
        while True:
            try:
                await asyncio.sleep(self.cleanup_interval)
                await self._cleanup_idle_containers()
            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Periodic cleanup failed: {e}")

    async def _cleanup_idle_containers(self) -> None:
        """アイドルコンテナのクリーンアップ"""
        now = int(time.time())
        workers = await self.backend.list_workers()
        evicted_count = 0

        for w in workers:
            # Condition: PAUSED or RUNNING and exceeding idle_timeout
            # In Phase 1/Go Agent, we may not pause containers, so we check RUNNING too.
            if (w.status == "PAUSED" or w.status == "RUNNING") and w.last_used_at > 0:
                idle_seconds = now - w.last_used_at
                if idle_seconds > self.idle_timeout:
                    try:
                        from services.common.models.internal import WorkerInfo

                        worker_info = WorkerInfo(
                            id=w.container_id,
                            name=w.function_name,
                            ip_address="",
                            port=0,
                        )
                        await self.backend.evict_worker(w.function_name, worker_info)
                        evicted_count += 1
                        logger.info(
                            f"Evicting idle container {w.container_id} "
                            f"(function: {w.function_name}, idle: {idle_seconds}s)"
                        )
                    except Exception as e:
                        # Don't stop the loop on individual failures
                        logger.error(f"Failed to evict container {w.container_id}: {e}")

        if evicted_count > 0:
            logger.info(f"Periodic cleanup: evicted {evicted_count} idle containers")
