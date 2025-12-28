"""
ContainerPool - Worker Pool Management for Auto-Scaling

Manages a pool of Lambda containers for a single function using Condition-based
capacity control. Supports concurrent acquire/release with proper notification of waiters.
"""

import asyncio
import logging
import time
from collections import deque
from typing import Callable, Awaitable, List, Set, Deque

from services.common.models.internal import WorkerInfo

logger = logging.getLogger("gateway.container_pool")


class ContainerPool:
    """
    Per-function container pool management (Condition-based).

    Uses asyncio.Condition to resolve semaphore inconsistencies noted in review
    and a deadlock observed in E2E (waiters not waking on release).
    """

    def __init__(
        self,
        function_name: str,
        max_capacity: int = 1,
        min_capacity: int = 0,
        acquire_timeout: float = 30.0,
    ):
        self.function_name = function_name
        self.max_capacity = max_capacity
        self.min_capacity = min_capacity
        self.acquire_timeout = acquire_timeout

        # Condition to guard state changes and send notifications.
        self._cv = asyncio.Condition()

        # Idle workers (managed efficiently with deque).
        self._idle_workers: Deque[WorkerInfo] = deque()

        # Ledger of all existing containers (busy + idle).
        self._all_workers: Set[WorkerInfo] = set()

        # Number of in-flight provisions (for capacity checks).
        self._provisioning_count = 0

    async def acquire(
        self, provision_callback: Callable[[str], Awaitable[List[WorkerInfo]]]
    ) -> WorkerInfo:
        """
        Acquire an available worker, provisioning if needed.
        """
        async with self._cv:
            start_time = time.time()

            while True:
                # 1. Prefer idle workers.
                if self._idle_workers:
                    worker = self._idle_workers.popleft()
                    return worker

                # 2. Provision if capacity is available.
                if len(self._all_workers) + self._provisioning_count < self.max_capacity:
                    # Reserve a provisioning slot.
                    self._provisioning_count += 1
                    break

                # 3. Wait if full.
                elapsed = time.time() - start_time
                remaining = self.acquire_timeout - elapsed
                if remaining <= 0:
                    raise asyncio.TimeoutError(f"Pool acquire timeout for {self.function_name}")

                try:
                    # wait() releases the lock and re-acquires on notify.
                    await asyncio.wait_for(self._cv.wait(), timeout=remaining)
                except asyncio.TimeoutError:
                    raise asyncio.TimeoutError(f"Pool acquire timeout for {self.function_name}")

        # --- Provisioning (I/O, so do it outside the CV lock) ---
        try:
            workers: List[WorkerInfo] = await provision_callback(self.function_name)
            worker = workers[0]
            async with self._cv:
                # Even if another worker exceeds max_capacity, register and
                # decrement provision_count (for safety).
                self._all_workers.add(worker)
                self._provisioning_count -= 1
                return worker
        except BaseException:
            # On failure/cancel, release reserved slot and wake waiters.
            async with self._cv:
                if self._provisioning_count > 0:
                    self._provisioning_count -= 1
                self._cv.notify_all()
            raise

    async def release(self, worker: WorkerInfo) -> None:
        """
        Return a worker to the pool.
        """
        async with self._cv:
            worker.last_used_at = time.time()
            self._idle_workers.append(worker)
            # Important: notify waiters that a resource is available.
            self._cv.notify_all()

    async def evict(self, worker: WorkerInfo) -> None:
        """
        Evict a dead worker from the pool (self-healing).
        """
        async with self._cv:
            if worker in self._all_workers:
                self._all_workers.discard(worker)
                # Notify because capacity is freed.
                self._cv.notify_all()

    def get_all_names(self) -> List[str]:
        """For heartbeat: list of all names (busy + idle)."""
        return [w.name for w in self._all_workers]

    def get_all_workers(self) -> List[WorkerInfo]:
        """Get all currently managed workers."""
        return list(self._all_workers)

    @property
    def size(self) -> int:
        """Current total workers (busy + idle)."""
        return len(self._all_workers)

    async def prune_idle_workers(self, idle_timeout: float) -> List[WorkerInfo]:
        """
        Remove workers that exceed IDLE_TIMEOUT.
        """
        async with self._cv:
            now = time.time()
            pruned = []
            surviving = deque()

            while self._idle_workers:
                worker = self._idle_workers.popleft()
                if now - worker.last_used_at > idle_timeout:
                    self._all_workers.discard(worker)
                    pruned.append(worker)
                else:
                    surviving.append(worker)

            self._idle_workers = surviving

            if pruned:
                # Notify because capacity is freed.
                self._cv.notify_all()

            return pruned

    async def adopt(self, worker: WorkerInfo) -> None:
        """Adopt a container into the pool on startup."""
        async with self._cv:
            if len(self._all_workers) + self._provisioning_count < self.max_capacity:
                # Only set timeout baseline if unset.
                if worker.last_used_at == 0:
                    worker.last_used_at = time.time()
                self._all_workers.add(worker)
                self._idle_workers.append(worker)
                self._cv.notify_all()
            else:
                logger.warning(
                    f"Adopt: Capacity limit reached for {self.function_name} while adopting {worker.name}."
                )

    async def drain(self) -> List[WorkerInfo]:
        """Drain all workers on shutdown."""
        async with self._cv:
            workers = list(self._all_workers)
            self._all_workers.clear()
            self._idle_workers.clear()
            self._provisioning_count = 0
            self._cv.notify_all()
            return workers

    @property
    def stats(self) -> dict:
        """Pool statistics."""
        return {
            "function_name": self.function_name,
            "total_workers": len(self._all_workers),
            "idle": len(self._idle_workers),
            "provisioning": self._provisioning_count,
            "max_capacity": self.max_capacity,
        }
