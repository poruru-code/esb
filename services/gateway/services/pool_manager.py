"""
PoolManager - Manages ContainerPools for all functions

Provides a unified interface for acquiring/releasing workers across multiple
Lambda functions. Each function gets its own ContainerPool with independent
capacity management.
"""

import asyncio
import logging
from typing import Callable, Dict, List, Any, Optional, Set

from .container_pool import ContainerPool
from services.common.models.internal import WorkerInfo

logger = logging.getLogger("gateway.pool_manager")


class PoolManager:
    """
    Manage pools for all functions.

    - Pools are lazily initialized (created on first get_pool)
    - Each function can have its own max_capacity
    """

    def __init__(
        self,
        provision_client: Any,
        config_loader: Callable[[str], Dict[str, Any]],
        pause_enabled: bool = False,
        pause_idle_seconds: float = 0.0,
    ):
        """
        Args:
            provision_client: client that sends provision requests to the Manager
            config_loader: callback to fetch config by function name (function_name -> config dict)
        """
        self._pools: Dict[str, ContainerPool] = {}
        self._lock = asyncio.Lock()
        self.provision_client = provision_client
        self.config_loader = config_loader
        try:
            pause_idle_value = float(pause_idle_seconds)
        except (TypeError, ValueError):
            pause_idle_value = 0.0

        self.pause_enabled = bool(pause_enabled) and pause_idle_value > 0
        self.pause_idle_seconds = pause_idle_value
        self._pause_tasks: Dict[str, asyncio.Task] = {}
        self._paused_ids: Set[str] = set()

        if self.pause_enabled and (
            not hasattr(provision_client, "pause_container")
            or not hasattr(provision_client, "resume_container")
        ):
            logger.warning(
                "Pause enabled but provision client lacks pause/resume; disabling pause."
            )
            self.pause_enabled = False

    async def get_pool(self, function_name: str) -> ContainerPool:
        """Get a pool by function name (create if missing)."""
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

    async def _cancel_pause_task(self, worker_id: str) -> None:
        task = self._pause_tasks.pop(worker_id, None)
        if task:
            task.cancel()
            try:
                await task
            except asyncio.CancelledError:
                pass

    async def _schedule_pause(
        self, function_name: str, pool: ContainerPool, worker: WorkerInfo
    ) -> None:
        if not self.pause_enabled:
            return

        await self._cancel_pause_task(worker.id)

        async def _pause_after_delay() -> None:
            task_ref = asyncio.current_task()
            try:
                await asyncio.sleep(self.pause_idle_seconds)
                if not self.pause_enabled:
                    return
                if worker.id in self._paused_ids:
                    return
                if not await pool.is_idle(worker.id):
                    return
                await self.provision_client.pause_container(function_name, worker)
                self._paused_ids.add(worker.id)
            except asyncio.CancelledError:
                return
            except Exception as e:
                logger.error(
                    f"Failed to pause container {worker.id} for {function_name}: {e}"
                )
            finally:
                if task_ref and self._pause_tasks.get(worker.id) is task_ref:
                    self._pause_tasks.pop(worker.id, None)

        task = asyncio.create_task(_pause_after_delay())
        self._pause_tasks[worker.id] = task

    async def _provision_wrapper(self, function_name: str) -> List[WorkerInfo]:
        """Provision API wrapper (returns List[WorkerInfo])."""
        return await self.provision_client.provision(function_name)

    async def acquire_worker(self, function_name: str) -> WorkerInfo:
        """Acquire a worker."""
        pool = await self.get_pool(function_name)
        while True:
            worker = await pool.acquire(self._provision_wrapper)
            if self.pause_enabled:
                await self._cancel_pause_task(worker.id)
                if worker.id in self._paused_ids:
                    try:
                        await self.provision_client.resume_container(function_name, worker)
                        self._paused_ids.discard(worker.id)
                        return worker
                    except Exception as e:
                        logger.error(
                            f"Failed to resume container {worker.id} for {function_name}: {e}"
                        )
                        self._paused_ids.discard(worker.id)
                        await pool.evict(worker)
                        continue
            return worker

    async def release_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """Release a worker."""
        if function_name in self._pools:
            pool = self._pools[function_name]
            await pool.release(worker)
            if self.pause_enabled:
                await self._schedule_pause(function_name, pool, worker)

    async def evict_worker(self, function_name: str, worker: WorkerInfo) -> None:
        """Evict a dead worker."""
        if function_name in self._pools:
            await self._cancel_pause_task(worker.id)
            self._paused_ids.discard(worker.id)
            await self._pools[function_name].evict(worker)

    def get_all_worker_names(self) -> Dict[str, List[str]]:
        """For heartbeat: collect all worker names across pools (busy + idle)."""
        result = {}
        for fname, pool in self._pools.items():
            result[fname] = pool.get_all_names()
        return result

    def _extract_function_name(self, name: str) -> Optional[str]:
        """
        Extract function name from container name.
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
        """Fetch all containers from Agent and delete them (startup cleanup)."""
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
        """Adopt existing containers from the orchestrator (Phase 1 compatibility)."""
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
        """Drain all pools and delete containers."""
        logger.info("Shutting down all pools...")
        await self._cancel_all_pause_tasks()
        self._paused_ids.clear()
        for fname, pool in self._pools.items():
            workers = await pool.drain()
            for w in workers:
                try:
                    await self.provision_client.delete_container(w.id)
                except Exception as e:
                    logger.error(f"Failed to delete {w.name}: {e}")

    async def prune_all_pools(self, idle_timeout: float) -> Dict[str, List[WorkerInfo]]:
        """Prune all pools and delete from orchestrator."""
        result = {}
        for fname, pool in self._pools.items():
            pruned = await pool.prune_idle_workers(idle_timeout)
            if pruned:
                for w in pruned:
                    await self._cancel_pause_task(w.id)
                    self._paused_ids.discard(w.id)
                result[fname] = pruned
                # Delete from orchestrator
                for w in pruned:
                    try:
                        await self.provision_client.delete_container(w.id)
                        logger.info(f"Pruned and deleted idle container: {w.name}")
                    except Exception as e:
                        logger.error(f"Failed to delete pruned container {w.name}: {e}")
        return result

    async def reconcile_orphans(self) -> int:
        """
        Detect containers not managed by the Gateway (orphans) and delete via Agent (full reconciliation).

        Grace period: containers created within ORPHAN_GRACE_PERIOD_SECONDS are excluded.
        This prevents deleting containers during creation/readiness checks.
        """
        import time
        from services.gateway.config import config as gateway_config

        grace_period = gateway_config.ORPHAN_GRACE_PERIOD_SECONDS
        current_time = time.time()

        try:
            # 1. Get all current containers from Agent.
            actual_containers = await self.provision_client.list_containers()
            if not actual_containers:
                return 0

            # 2. Collect all worker IDs known to the Gateway.
            known_ids = set()
            for pool in self._pools.values():
                workers = pool.get_all_workers()
                for w in workers:
                    known_ids.add(w.id)

            # 3. Detect orphans (present in actual but not known).
            orphans = [c for c in actual_containers if c.id not in known_ids]

            # 4. Delete orphans (respect grace period).
            removed_count = 0
            for orphan in orphans:
                # Grace period check: skip containers created within grace_period seconds.
                container_age = current_time - orphan.created_at
                if container_age < grace_period:
                    logger.debug(
                        f"Ignoring young container {orphan.id} ({orphan.name}). "
                        f"Age: {container_age:.1f}s < {grace_period}s grace period"
                    )
                    continue

                try:
                    logger.warning(
                        f"Found orphan container {orphan.id} ({orphan.name}). "
                        f"Age: {container_age:.1f}s. Deleting... (Reconciliation)"
                    )
                    await self.provision_client.delete_container(orphan.id)
                    removed_count += 1
                except Exception as e:
                    logger.error(f"Failed to delete orphan {orphan.id}: {e}")

            if removed_count > 0:
                logger.info(f"Reconciliation: Removed {removed_count} orphan containers")

            return removed_count

        except Exception as e:
            logger.error(f"Reconciliation failed: {e}")
            return 0

    async def _cancel_all_pause_tasks(self) -> None:
        tasks = list(self._pause_tasks.values())
        self._pause_tasks.clear()
        for task in tasks:
            task.cancel()
        for task in tasks:
            try:
                await task
            except asyncio.CancelledError:
                pass
