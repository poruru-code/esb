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
    関数ごとのコンテナプール管理 (Condition方式)

    外部レビューで指摘されたセマフォの挙動不整合と、
    E2Eで発生したデッドロック（release時に待機者が起きない問題）を解消するため、
    asyncio.Condition を用いた明示的な通知モデルを採用。
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

        # 全状態変更を保護し、通知を行うための Condition
        self._cv = asyncio.Condition()

        # アイドルワーカー（deque で効率的に管理）
        self._idle_workers: Deque[WorkerInfo] = deque()

        # 台帳: 存在する全コンテナ (Busy + Idle)
        self._all_workers: Set[WorkerInfo] = set()

        # プロビジョニング中の件数 (容量制限チェック用)
        self._provisioning_count = 0

    async def acquire(
        self, provision_callback: Callable[[str], Awaitable[List[WorkerInfo]]]
    ) -> WorkerInfo:
        """
        利用可能なワーカーを取得。なければプロビジョニング。
        """
        async with self._cv:
            start_time = time.time()

            while True:
                # 1. アイドルがあれば優先的に使う
                if self._idle_workers:
                    worker = self._idle_workers.popleft()
                    return worker

                # 2. 空き枠があればプロビジョニングへ進む
                if len(self._all_workers) + self._provisioning_count < self.max_capacity:
                    # プロビジョニング枠を予約
                    self._provisioning_count += 1
                    break

                # 3. 満杯なら待機
                elapsed = time.time() - start_time
                remaining = self.acquire_timeout - elapsed
                if remaining <= 0:
                    raise asyncio.TimeoutError(f"Pool acquire timeout for {self.function_name}")

                try:
                    # wait() は一時的にロックを解除し、notify で起こされたら再度ロックを取得する
                    await asyncio.wait_for(self._cv.wait(), timeout=remaining)
                except asyncio.TimeoutError:
                    raise asyncio.TimeoutError(f"Pool acquire timeout for {self.function_name}")

        # --- プロビジョニング実行 (I/Oを伴うため CV ロックの外で行う) ---
        try:
            workers: List[WorkerInfo] = await provision_callback(self.function_name)
            worker = workers[0]
            async with self._cv:
                # すでに別のワーカーが滑り込み等で max_capacity を超えた場合でも、
                # provision_count を下げて登録する。
                # (基本的には break 后の atomic 操作で防いでいるが安全性のため)
                self._all_workers.add(worker)
                self._provisioning_count -= 1
                return worker
        except BaseException:
            # 失敗（またはキャンセル）した場合は予約した枠を戻し、待機者を起こす
            async with self._cv:
                if self._provisioning_count > 0:
                    self._provisioning_count -= 1
                self._cv.notify_all()
            raise

    async def release(self, worker: WorkerInfo) -> None:
        """
        ワーカーをプールに返却
        """
        async with self._cv:
            worker.last_used_at = time.time()
            self._idle_workers.append(worker)
            # 重要: 待機者にリソースが利用可能になったことを通知
            self._cv.notify_all()

    async def evict(self, worker: WorkerInfo) -> None:
        """
        死んだワーカーをプールから除外 (Self-Healing)
        """
        async with self._cv:
            if worker in self._all_workers:
                self._all_workers.discard(worker)
                # 枠が空いたので通知
                self._cv.notify_all()

    def get_all_names(self) -> List[str]:
        """Heartbeat用: Busy も Idle もすべて含む Name リスト"""
        return [w.name for w in self._all_workers]

    def get_all_workers(self) -> List[WorkerInfo]:
        """現在管理している全ワーカーを取得"""
        return list(self._all_workers)

    @property
    def size(self) -> int:
        """現在の総ワーカー数 (Busy + Idle)"""
        return len(self._all_workers)

    async def prune_idle_workers(self, idle_timeout: float) -> List[WorkerInfo]:
        """
        IDLE_TIMEOUT を超えたワーカーをプールから除外
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
                # 枠が空いたので通知
                self._cv.notify_all()

            return pruned

    async def adopt(self, worker: WorkerInfo) -> None:
        """起動時にコンテナをプールに取り込み"""
        async with self._cv:
            if len(self._all_workers) + self._provisioning_count < self.max_capacity:
                # 未設定の場合のみタイムアウト起点を現在にする
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
        """終了時に全ワーカーを排出"""
        async with self._cv:
            workers = list(self._all_workers)
            self._all_workers.clear()
            self._idle_workers.clear()
            self._provisioning_count = 0
            self._cv.notify_all()
            return workers

    @property
    def stats(self) -> dict:
        """プール統計情報"""
        return {
            "function_name": self.function_name,
            "total_workers": len(self._all_workers),
            "idle": len(self._idle_workers),
            "provisioning": self._provisioning_count,
            "max_capacity": self.max_capacity,
        }
