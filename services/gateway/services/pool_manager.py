"""
PoolManager - Manages ContainerPools for all functions

Provides a unified interface for acquiring/releasing workers across multiple
Lambda functions. Each function gets its own ContainerPool with independent
capacity management.
"""

import asyncio
import logging
from typing import Callable, Dict, List, Any, Optional

from .container_pool import ContainerPool
from services.common.models.internal import WorkerInfo

logger = logging.getLogger("gateway.pool_manager")


class PoolManager:
    """
    全関数のプールを統括管理

    - プール作成は遅延初期化 (get_pool で初めて作成)
    - 関数ごとに独立した max_capacity を設定可能
    """

    def __init__(
        self,
        provision_client: Any,
        config_loader: Callable[[str], Dict[str, Any]],
    ):
        """
        Args:
            provision_client: Manager への provision リクエストを送信するクライアント
            config_loader: 関数名から設定を取得するコールバック (function_name -> config dict)
        """
        self._pools: Dict[str, ContainerPool] = {}
        self._lock = asyncio.Lock()
        self.provision_client = provision_client
        self.config_loader = config_loader

    async def get_pool(self, function_name: str) -> ContainerPool:
        """関数名からプールを取得（なければ作成）"""
        if function_name not in self._pools:
            async with self._lock:
                if function_name not in self._pools:
                    config = self.config_loader(function_name)
                    scaling = config.get("scaling", {})
                    self._pools[function_name] = ContainerPool(
                        function_name=function_name,
                        max_capacity=scaling.get("max_capacity", 1),
                        min_capacity=scaling.get("min_capacity", 0),
                        acquire_timeout=scaling.get("acquire_timeout", 5.0),
                    )
                    logger.info(
                        f"Created pool for {function_name}: "
                        f"max_capacity={self._pools[function_name].max_capacity}"
                    )
        return self._pools[function_name]

    async def _provision_wrapper(self, function_name: str) -> List[WorkerInfo]:
        """Provision API ラッパー (List[WorkerInfo] を返す)"""
        return await self.provision_client.provision(function_name)

    async def acquire_worker(self, function_name: str) -> WorkerInfo:
        """ワーカーを取得"""
        pool = await self.get_pool(function_name)
        return await pool.acquire(self._provision_wrapper)

    async def release_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """ワーカーを返却"""
        if function_name in self._pools:
            await self._pools[function_name].release(worker)

    async def evict_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """死んだワーカーを除外"""
        if function_name in self._pools:
            await self._pools[function_name].evict(worker)

    def get_all_worker_names(self) -> Dict[str, List[str]]:
        """Heartbeat用: 全プールの全Worker Nameを収集 (Busy + Idle)"""
        result = {}
        for fname, pool in self._pools.items():
            result[fname] = pool.get_all_names()
        return result

    def _extract_function_name(self, name: str) -> Optional[str]:
        """
        コンテナ名から関数名を抽出
        Format: lambda-{function_name}-{suffix}
        """
        if not name.startswith("lambda-"):
            return None

        parts = name.split("-")
        if len(parts) < 3:
            return None

        # parts[0] is "lambda"
        # parts[-1] is suffix (uuid)
        # function_name is in between
        return "-".join(parts[1:-1])

    async def cleanup_all_containers(self) -> int:
        """Agent から全コンテナを取得し、すべて削除する（起動時のクリーンアップ用）"""
        try:
            containers = await self.provision_client.list_containers()
            count = 0
            for worker in containers:
                try:
                    await self.provision_client.delete_container(worker.id)
                    count += 1
                except Exception as e:
                    logger.error(f"Failed to delete orphan container {worker.id}: {e}")
            if count > 0:
                logger.info(f"Cleanup: Removed {count} orphan containers on startup")
            return count
        except Exception as e:
            logger.error(f"Failed to cleanup all containers: {e}")
            return 0

    async def sync_with_manager(self) -> None:
        """Orchestrator から既存コンテナを取得しプールに取り込み (Phase 1 互換)"""
        try:
            containers = await self.provision_client.list_containers()
            adopted_count = 0
            for worker in containers:
                function_name = self._extract_function_name(worker.name)
                if function_name:
                    pool = await self.get_pool(function_name)
                    await pool.adopt(worker)
                    adopted_count += 1
            if adopted_count > 0:
                logger.info(f"Adopted {adopted_count} containers from Orchestrator")
        except Exception as e:
            logger.error(f"Failed to sync with manager: {e}")

    async def shutdown_all(self) -> None:
        """全プールをドレインし、コンテナを削除"""
        logger.info("Shutting down all pools...")
        for fname, pool in self._pools.items():
            workers = await pool.drain()
            for w in workers:
                try:
                    await self.provision_client.delete_container(w.id)
                except Exception as e:
                    logger.error(f"Failed to delete {w.name}: {e}")

    async def prune_all_pools(self, idle_timeout: float) -> Dict[str, List[WorkerInfo]]:
        """全プールで Pruning を実行し、Orchestrator から削除"""
        result = {}
        for fname, pool in self._pools.items():
            pruned = await pool.prune_idle_workers(idle_timeout)
            if pruned:
                result[fname] = pruned
                # Delete from orchestrator
                for w in pruned:
                    try:
                        await self.provision_client.delete_container(w.id)
                        logger.info(f"Pruned and deleted idle container: {w.name}")
                    except Exception as e:
                        logger.error(f"Failed to delete pruned container {w.name}: {e}")
        return result
