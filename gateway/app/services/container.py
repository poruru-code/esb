"""
ContainerManager - Lambda コンテナのライフサイクル管理

オンデマンドでコンテナを起動し、アイドル状態のコンテナを停止する。
"""

import docker
import docker.errors
import time
import logging
import os
from typing import Dict, Optional


import socket

logger = logging.getLogger("gateway.container_manager")


class ContainerManager:
    """
    Lambdaコンテナのライフサイクルを管理するクラス

    - ensure_container_running(): コンテナが起動していなければ起動
    - stop_idle_containers(): アイドル状態のコンテナを停止
    """

    def __init__(self, network: Optional[str] = None):
        """
        Args:
            network: コンテナを接続するDockerネットワーク名
                     省略時は自動検出（Gateway接続ネットワークから特定）
        """
        self.client = docker.from_env()
        self.last_accessed: Dict[str, float] = {}

        # ネットワークの動的解決
        self.network = network or os.environ.get("DOCKER_NETWORK") or self._resolve_network()
        logger.info(f"ContainerManager initialized with network: {self.network}")

    def _resolve_network(self) -> str:
        """
        自分自身(Gateway)が接続しているネットワークから、
        Lambda用の内部ネットワーク名を動的に特定する

        Dockerコンテナ内ではホスト名=短縮コンテナID
        """
        try:
            hostname = socket.gethostname()
            self_container = self.client.containers.get(hostname)

            networks = self_container.attrs.get("NetworkSettings", {}).get("Networks", {})

            # 末尾が 'onpre-internal-network' で終わるネットワークを探す
            for net_name in networks.keys():
                if net_name.endswith("onpre-internal-network"):
                    logger.info(f"Auto-detected internal network: {net_name}")
                    return net_name

            # 見つからない場合はデフォルト
            logger.warning("Could not auto-detect internal network. Using 'bridge'.")
            return "bridge"

        except Exception as e:
            logger.warning(f"Failed to resolve network dynamically: {e}. Using 'bridge'.")
            return "bridge"

    def resolve_gateway_internal_url(self) -> str:
        """
        Gateway自身の内部アクセス用URLを解決する。
        onpre-internal-network 上での自身のアドレスを特定する。
        解決できない場合はエラーとする（起動不可）。

        Returns:
            str: 内部用URL (例: https://onpre-gateway:443)
        """
        try:
            hostname = socket.gethostname()
            # 自分のコンテナ情報を取得
            self_container = self.client.containers.get(hostname)
            networks = self_container.attrs.get("NetworkSettings", {}).get("Networks", {})

            # 内部ネットワーク上のIPまたはエイリアスを探す
            for net_name, net_info in networks.items():
                if net_name.endswith("onpre-internal-network"):
                    # 内向きにはコンテナ名(hostname)でアクセス可能だが、
                    # 念のためIPアドレスまたはNetworkエイリアスを使用する手もある。
                    # ここでは最も確実なコンテナ名(hostname) + デフォルトのHTTPSポートを使用する。
                    # エイリアスが設定されている場合もあるが、hostnameはユニーク。
                    # ただし、self.client.containers.get(hostname).name がコンテナ名。
                    container_name = self_container.name
                    logger.info(f"Resolved Gateway Internal Host: {container_name} on {net_name}")
                    return f"https://{container_name}"

            # 見つからない場合
            raise RuntimeError(
                f"Gateway container is not attached to 'onpre-internal-network'. "
                f"Attached networks: {list(networks.keys())}"
            )

        except Exception as e:
            logger.critical(f"Failed to resolve Gateway Internal URL: {e}")
            raise RuntimeError(f"Critical Error: Could not resolve Gateway Internal URL: {e}")

    def ensure_container_running(
        self, name: str, image: Optional[str] = None, env: Optional[Dict[str, str]] = None
    ) -> str:
        """
        コンテナが起動していなければ起動し、ホスト名を返す

        Args:
            name: コンテナ名（Dockerネットワーク内のホスト名としても使用）
            image: Dockerイメージ名（省略時は name:latest）
            env: 環境変数の辞書

        Returns:
            コンテナのホスト名（= name）
        """
        # 最終アクセス時刻を更新
        self.last_accessed[name] = time.time()

        # imageのデフォルト値
        if image is None:
            image = f"{name}:latest"

        try:
            container = self.client.containers.get(name)

            if container.status == "running":
                logger.debug(f"Container {name} is already running")
                return name

            elif container.status == "exited":
                logger.info(f"Warm-up: Restarting container {name}...")
                container.start()
                self._wait_for_readiness(name)
                return name

            else:
                # created, paused, etc. - 停止して再作成
                logger.info(f"Container {name} in state {container.status}, removing...")
                container.remove(force=True)
                raise docker.errors.NotFound(f"Removed {name}")

        except docker.errors.NotFound:
            logger.info(f"Cold Start: Creating and starting container {name}...")
            self.client.containers.run(
                image,
                name=name,
                detach=True,
                environment=env or {},
                network=self.network,
                restart_policy={"Name": "no"},
            )
            self._wait_for_readiness(name)
            return name

    def _wait_for_readiness(self, host: str, port: int = 8080, timeout: int = 30) -> None:
        """
        Lambda RIEがリクエスト可能になるまで待機

        TCP ポート接続で確認（POST invocation を消費しない）
        これにより同時リクエスト時の RIE パニックを回避

        Args:
            host: コンテナのホスト名
            port: RIE のリッスンポート（デフォルト 8080）
            timeout: 待機タイムアウト（秒）
        """

        start = time.time()

        while time.time() - start < timeout:
            try:
                with socket.create_connection((host, port), timeout=1):
                    logger.debug(f"Container {host} is listening on port {port}")
                    return
            except (socket.timeout, ConnectionRefusedError, OSError):
                time.sleep(0.5)

        logger.warning(f"Container {host} did not become ready in {timeout}s")

    def stop_idle_containers(self, timeout_seconds: int = 900) -> None:
        """
        タイムアウトしたコンテナを停止

        Args:
            timeout_seconds: アイドルタイムアウト（秒）。デフォルト15分
        """
        now = time.time()
        to_remove = []

        for name, last_access in self.last_accessed.items():
            if now - last_access > timeout_seconds:
                try:
                    logger.info(f"Scale-down: Stopping idle container {name}")
                    container = self.client.containers.get(name)
                    if container.status == "running":
                        container.stop()
                    to_remove.append(name)
                except docker.errors.NotFound:
                    # コンテナが既に削除されている
                    to_remove.append(name)
                except Exception as e:
                    logger.error(f"Failed to stop {name}: {e}")

        for name in to_remove:
            del self.last_accessed[name]


# シングルトンインスタンス（遅延初期化）
_manager_instance: Optional[ContainerManager] = None


def get_manager() -> ContainerManager:
    """
    ContainerManagerのシングルトンインスタンスを取得

    遅延初期化により、インポート時にDockerに接続しない
    """
    global _manager_instance
    if _manager_instance is None:
        _manager_instance = ContainerManager()
    return _manager_instance
