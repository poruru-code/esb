"""
ContainerManager Unit Tests

Docker SDKをモック化してコンテナ管理ロジックをテスト
"""
from unittest.mock import MagicMock, patch
import time


class TestContainerManagerNetworkResolution:
    """_resolve_network() および __init__ の動的解決ロジックのテスト"""

    @patch("gateway.app.services.container.docker")
    @patch("gateway.app.services.container.socket")
    def test_resolve_network_success(self, mock_socket, mock_docker):
        """Gatewayが接続しているネットワークから内部ネットワークを特定できる"""
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        # ホスト名（コンテナID）のモック
        mock_socket.gethostname.return_value = "gateway-container-id"
        
        # コンテナ取得のモック
        mock_container = MagicMock()
        mock_client.containers.get.return_value = mock_container
        
        # ネットワーク設定のモック
        # 期待するネットワーク名を含む辞書を返す
        mock_container.attrs = {
            "NetworkSettings": {
                "Networks": {
                    "sample-project_onpre-internal-network": {},
                    "sample-project_default": {}
                }
            }
        }
        
        # Act - network引数なしで初期化
        manager = ContainerManager(network=None)
        
        # Assert
        assert manager.network == "sample-project_onpre-internal-network"
        mock_client.containers.get.assert_called_with("gateway-container-id")

    @patch("gateway.app.services.container.docker")
    @patch("gateway.app.services.container.socket")
    def test_resolve_network_fallback(self, mock_socket, mock_docker):
        """内部ネットワークが見つからない場合は bridge にフォールバック"""
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        mock_socket.gethostname.return_value = "gateway-container-id"
        
        mock_container = MagicMock()
        mock_client.containers.get.return_value = mock_container
        
        # マッチするネットワークがない
        mock_container.attrs = {
            "NetworkSettings": {
                "Networks": {
                    "other-network": {}
                }
            }
        }
        
        # Act
        manager = ContainerManager(network=None)
        
        # Assert
        assert manager.network == "bridge"

    @patch("gateway.app.services.container.docker")
    @patch("gateway.app.services.container.socket")
    def test_init_with_arg_skips_resolution(self, mock_socket, mock_docker):
        """引数でネットワークを指定した場合は動的解決をスキップ"""
        from gateway.app.services.container import ContainerManager
        
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        # Act
        manager = ContainerManager(network="explicit-network")
        
        # Assert
        assert manager.network == "explicit-network"
        mock_socket.gethostname.assert_not_called()
        mock_client.containers.get.assert_not_called()


class TestContainerManagerEnsureRunning:
    """ensure_container_running() のテスト"""

    @patch("gateway.app.services.container.docker")
    def test_cold_start_creates_new_container(self, mock_docker):
        """コンテナが存在しない場合、新規作成される"""
        import docker.errors
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        # docker.errorsをモックに設定（実装側でdocker.errors.NotFoundを使用するため）
        mock_docker.errors = docker.errors
        
        # コンテナが存在しない
        mock_client.containers.get.side_effect = docker.errors.NotFound("not found")
        
        manager = ContainerManager(network="test-network")
        
        # Act
        result = manager.ensure_container_running(
            name="lambda-test",
            image="lambda-test:latest",
            env={"KEY": "value"}
        )
        
        # Assert
        mock_client.containers.run.assert_called_once()
        call_kwargs = mock_client.containers.run.call_args.kwargs
        assert call_kwargs["name"] == "lambda-test"
        assert call_kwargs["network"] == "test-network"
        assert result == "lambda-test"

    @patch("gateway.app.services.container.docker")
    def test_warm_start_restarts_stopped_container(self, mock_docker):
        """停止中のコンテナはrestartされる"""
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        mock_container = MagicMock()
        mock_container.status = "exited"
        mock_client.containers.get.return_value = mock_container
        
        manager = ContainerManager(network="test-network")
        
        # Act
        result = manager.ensure_container_running(
            name="lambda-test",
            image="lambda-test:latest",
            env={}
        )
        
        # Assert
        mock_container.start.assert_called_once()
        mock_client.containers.run.assert_not_called()
        assert result == "lambda-test"

    @patch("gateway.app.services.container.docker")
    def test_already_running_does_nothing(self, mock_docker):
        """既に起動中のコンテナは何もしない"""
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        mock_container = MagicMock()
        mock_container.status = "running"
        mock_client.containers.get.return_value = mock_container
        
        manager = ContainerManager(network="test-network")
        
        # Act
        result = manager.ensure_container_running(
            name="lambda-test",
            image="lambda-test:latest",
            env={}
        )
        
        # Assert
        mock_container.start.assert_not_called()
        mock_client.containers.run.assert_not_called()
        assert result == "lambda-test"

    @patch("gateway.app.services.container.docker")
    def test_updates_last_accessed_time(self, mock_docker):
        """アクセス時刻が更新される"""
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        mock_container = MagicMock()
        mock_container.status = "running"
        mock_client.containers.get.return_value = mock_container
        
        manager = ContainerManager(network="test-network")
        
        # Act
        before = time.time()
        manager.ensure_container_running(
            name="lambda-test",
            image="lambda-test:latest",
            env={}
        )
        after = time.time()
        
        # Assert
        assert "lambda-test" in manager.last_accessed
        assert before <= manager.last_accessed["lambda-test"] <= after


class TestContainerManagerStopIdle:
    """stop_idle_containers() のテスト"""

    @patch("gateway.app.services.container.docker")
    def test_stops_idle_containers(self, mock_docker):
        """タイムアウト超過コンテナが停止される"""
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        mock_container = MagicMock()
        mock_container.status = "running"
        mock_client.containers.get.return_value = mock_container
        
        manager = ContainerManager(network="test-network")
        
        # 古いアクセス時刻を設定
        manager.last_accessed["lambda-old"] = time.time() - 1000  # 1000秒前
        
        # Act
        manager.stop_idle_containers(timeout_seconds=900)  # 15分
        
        # Assert
        mock_container.stop.assert_called_once()
        assert "lambda-old" not in manager.last_accessed

    @patch("gateway.app.services.container.docker")
    def test_keeps_recent_containers(self, mock_docker):
        """最近アクセスされたコンテナは停止されない"""
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        manager = ContainerManager(network="test-network")
        
        # 最近のアクセス時刻
        manager.last_accessed["lambda-recent"] = time.time() - 100  # 100秒前
        
        # Act
        manager.stop_idle_containers(timeout_seconds=900)
        
        # Assert
        mock_client.containers.get.assert_not_called()
        assert "lambda-recent" in manager.last_accessed

    @patch("gateway.app.services.container.docker")
    @patch("gateway.app.services.container.time")
    def test_timeout_boundary_exact(self, mock_time, mock_docker):
        """タイムアウトちょうどの時間ではコンテナは停止されない（>で比較するため）"""
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        # 現在時刻を固定
        mock_time.time.return_value = 1000.0
        
        manager = ContainerManager(network="test-network")
        
        # ちょうど15分前（900秒前）= 時刻100.0
        manager.last_accessed["lambda-boundary"] = 100.0  # 1000 - 100 = 900秒
        
        # Act
        manager.stop_idle_containers(timeout_seconds=900)
        
        # Assert: 900秒ちょうどなので停止されない（now - last_access = 900、900 > 900 は False）
        mock_client.containers.get.assert_not_called()
        assert "lambda-boundary" in manager.last_accessed

    @patch("gateway.app.services.container.docker")
    @patch("gateway.app.services.container.time")
    def test_timeout_boundary_exceeded(self, mock_time, mock_docker):
        """タイムアウトを1秒超えたらコンテナは停止される"""
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        mock_container = MagicMock()
        mock_container.status = "running"
        mock_client.containers.get.return_value = mock_container
        
        # 現在時刻を固定
        mock_time.time.return_value = 1000.0
        
        manager = ContainerManager(network="test-network")
        
        # 15分+1秒前（901秒前）= 時刻99.0
        manager.last_accessed["lambda-exceeded"] = 99.0  # 1000 - 99 = 901秒
        
        # Act
        manager.stop_idle_containers(timeout_seconds=900)
        
        # Assert: 901秒なので停止される（now - last_access = 901、901 > 900 は True）
        mock_container.stop.assert_called_once()
        assert "lambda-exceeded" not in manager.last_accessed


class TestIdleTimeoutMinutesEnv:
    """IDLE_TIMEOUT_MINUTES環境変数のテスト"""

    def test_default_timeout_is_5_minutes(self):
        """デフォルトタイムアウトは5分（300秒）"""
        import os
        # 環境変数をクリア
        os.environ.pop("IDLE_TIMEOUT_MINUTES", None)
        
        # scheduler.pyを再読み込み
        from gateway.app.services import scheduler
        import importlib
        importlib.reload(scheduler)
        
        assert scheduler.IDLE_TIMEOUT_MINUTES == 5
        assert scheduler.IDLE_TIMEOUT == 300

    def test_custom_timeout_from_env(self):
        """環境変数でタイムアウトを変更できる"""
        import os
        os.environ["IDLE_TIMEOUT_MINUTES"] = "5"
        
        try:
            from gateway.app.services import scheduler
            import importlib
            importlib.reload(scheduler)
            
            assert scheduler.IDLE_TIMEOUT_MINUTES == 5
            assert scheduler.IDLE_TIMEOUT == 300  # 5分 = 300秒
        finally:
            # クリーンアップ
            os.environ.pop("IDLE_TIMEOUT_MINUTES", None)


class TestContainerManagerImageDefault:
    """imageのデフォルト値のテスト"""

    @patch("gateway.app.services.container.docker")
    def test_default_image_from_container_name(self, mock_docker):
        """imageが指定されない場合、container:latestが使用される"""
        import docker.errors
        from gateway.app.services.container import ContainerManager
        
        # Arrange
        mock_client = MagicMock()
        mock_docker.from_env.return_value = mock_client
        
        # docker.errorsをモックに設定
        mock_docker.errors = docker.errors
        
        mock_client.containers.get.side_effect = docker.errors.NotFound("not found")
        
        manager = ContainerManager(network="test-network")
        
        # Act - imageをNoneで渡す
        manager.ensure_container_running(
            name="lambda-test",
            image=None,  # 省略
            env={}
        )
        
        # Assert - デフォルトでcontainer:latestが使用される
        call_args = mock_client.containers.run.call_args
        assert call_args[0][0] == "lambda-test:latest"
