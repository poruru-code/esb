import asyncio
import time
from unittest.mock import MagicMock, patch
import pytest
from ..service import ContainerManager


class MockContainer:
    def __init__(self, name, status="running"):
        self.name = name
        self.status = status
        self.attrs = {"NetworkSettings": {"Networks": {"bridge": {"IPAddress": "172.17.0.2"}}}}

    def reload(self):
        pass

    def start(self):
        self.status = "running"

    def stop(self):
        self.status = "exited"


@pytest.mark.asyncio
@patch("services.manager.service.docker.from_env")
async def test_ensure_container_concurrency(mock_from_env):
    """
    同一コンテナに対して100並列でリクエストを投げた際、
    docker run が1回しか呼ばれないことを検証する。
    """
    # モックコンテナの設定
    mock_container = MockContainer("test-func")

    # 例外クラスをインポート
    from docker.errors import NotFound

    # モッククライアントの設定（ContainerManager のインスタンス化前に）
    mock_client = MagicMock()
    mock_from_env.return_value = mock_client

    # containers.get の動作を定義
    # 最初の呼び出しは NotFound を投げる
    # run が呼ばれた後は、作成済みコンテナを返す
    get_call_count = {"count": 0}

    def get_side_effect(name):
        get_call_count["count"] += 1
        if get_call_count["count"] == 1:
            raise NotFound("Not Found")
        # 2回目以降はコンテナを返す（run が完了した後）
        return mock_container

    mock_client.containers.get.side_effect = get_side_effect
    # Run は成功してコンテナを返す
    mock_client.containers.run.return_value = mock_container

    # ここで ContainerManager をインスタンス化
    manager = ContainerManager(network="bridge")

    # 読み込み待ちをスキップ
    manager._wait_for_readiness = MagicMock()

    container_name = "test-func"

    # 100並列で実行
    tasks = [
        asyncio.to_thread(manager.ensure_container_running, container_name) for _ in range(100)
    ]

    results = await asyncio.gather(*tasks)

    # 検証: すべて同じコンテナ名を返しているか
    assert all(r == container_name for r in results)

    # 検証: 100回リクエストが来ても、実際に docker run が呼ばれたのは「1回」だけであること
    assert mock_client.containers.run.call_count == 1


@pytest.mark.asyncio
@patch("services.manager.service.docker.from_env")
async def test_lock_cleanup_concurrency(mock_from_env):
    """
    stop_idle_containers でロックが正しくクリーンアップされること、
    およびクリーンアップ後に再度アクセスしても正常に動作することを検証する。
    """
    mock_client = MagicMock()
    mock_from_env.return_value = mock_client

    manager = ContainerManager(network="bridge")
    mock_container = MockContainer("test-func")
    from docker.errors import NotFound

    mock_client.containers.run.return_value = mock_container
    manager._wait_for_readiness = MagicMock()

    container_name = "idle-func"

    # 1. コンテナを起動（ロックが作成される）
    manager.ensure_container_running(container_name)
    assert container_name in manager.locks

    # 2. アイドル停止を実行（ロックが削除される）
    manager.last_accessed[container_name] = time.time() - 1000  # 過去にする
    mock_client.containers.get.return_value = mock_container

    manager.stop_idle_containers(timeout_seconds=500)

    assert container_name not in manager.locks
    assert container_name not in manager.last_accessed

    # 3. 再度アクセス（新しいロックが作成され、正常に動作する）
    mock_client.containers.get.side_effect = NotFound("Not Found")
    manager.ensure_container_running(container_name)
    assert container_name in manager.locks
