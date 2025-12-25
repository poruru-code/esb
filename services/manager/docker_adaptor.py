"""
Docker Client Adapter for Async IO

Blocking docker-py calls are offloaded to a dedicated executor.
"""

import asyncio
import docker
import logging
from typing import Any, List
from concurrent.futures import ThreadPoolExecutor
from importlib.metadata import metadata

from .config import config

logger = logging.getLogger("manager.docker_adaptor")

# プロジェクト名を動的に取得
PROJECT_NAME = metadata("edge-serverless-box")["Name"]


class DockerAdaptor:
    def __init__(self):
        # タイムアウトを設定して無限待ちを防ぐ
        self._client = docker.from_env(timeout=config.DOCKER_CLIENT_TIMEOUT)

        # Docker操作専用のスレッドプール
        # これにより、Dockerが詰まっても他の非同期処理(HTTPなど)は生き残る
        self.executor = ThreadPoolExecutor(
            max_workers=config.DOCKER_MAX_WORKERS,
            thread_name_prefix="docker_worker",
        )

    async def get_container(self, name: str) -> Any:
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(self.executor, self._client.containers.get, name)

    async def run_container(self, image: str, **kwargs) -> Any:
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(
            self.executor, lambda: self._client.containers.run(image, **kwargs)
        )

    async def list_containers(self, **kwargs) -> List[Any]:
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(
            self.executor, lambda: self._client.containers.list(**kwargs)
        )

    # Note: Accessing attributes of a container object (like container.status) is usually fast/cached
    # IF it was just returned by get/run.
    # BUT container.reload() is network I/O.

    async def reload_container(self, container: Any) -> None:
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(self.executor, container.reload)

    async def stop_container(self, container: Any) -> None:
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(self.executor, container.stop)

    async def remove_container(self, container: Any, force: bool = False) -> None:
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(self.executor, lambda: container.remove(force=force))

    async def kill_container(self, container: Any) -> None:
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(self.executor, container.kill)

    async def prune_containers(self) -> None:
        """
        ゾンビコンテナ（label=created_by={PROJECT_NAME}）を削除します。

        非同期処理のため、run_in_executorを使用してブロッキングを回避します。
        """

        def _prune():
            """同期的なコンテナ削除処理"""
            try:
                containers = self._client.containers.list(
                    all=True,  # Include stopped ones
                    filters={"label": f"created_by={PROJECT_NAME}"},
                )
                for container in containers:
                    logger.info(f"Removing zombie container: {container.name}")
                    try:
                        if container.status == "running":
                            container.kill()
                        container.remove(force=True)
                    except Exception as e:
                        logger.error(f"Error removing {container.name}: {e}")
            except Exception as e:
                logger.error(f"Failed to prune containers: {e}")

        loop = asyncio.get_running_loop()
        await loop.run_in_executor(self.executor, _prune)

    def shutdown(self):
        """スレッドプールを安全にシャットダウン"""
        self.executor.shutdown(wait=True)
