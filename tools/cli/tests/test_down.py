"""Unit tests for esb down command"""
import pytest
from unittest.mock import patch, MagicMock
from argparse import Namespace

from tools.cli.commands import down


@patch("docker.from_env")
@patch("subprocess.check_call")
def test_down_basic(mock_subprocess, mock_docker):
    """down コマンドが docker compose down を実行すること"""
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.return_value = mock_client
    
    args = Namespace(volumes=False)
    down.run(args)
    
    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "docker" in cmd
    assert "compose" in cmd
    assert "down" in cmd
    assert "--remove-orphans" in cmd
    assert "--volumes" not in cmd


@patch("docker.from_env")
@patch("subprocess.check_call")
def test_down_with_volumes(mock_subprocess, mock_docker):
    """down --volumes が docker compose down --volumes を実行すること"""
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.return_value = mock_client
    
    args = Namespace(volumes=True)
    down.run(args)
    
    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "--volumes" in cmd


@patch("docker.from_env")
@patch("subprocess.check_call")
def test_down_cleans_lambda_containers(mock_subprocess, mock_docker):
    """down が Lambda コンテナ (created_by=sample-dind) をクリーンアップすること"""
    mock_container_running = MagicMock()
    mock_container_running.status = "running"
    mock_container_running.name = "lambda-test-running"
    
    mock_container_stopped = MagicMock()
    mock_container_stopped.status = "exited"
    mock_container_stopped.name = "lambda-test-stopped"
    
    mock_client = MagicMock()
    mock_client.containers.list.return_value = [mock_container_running, mock_container_stopped]
    mock_docker.return_value = mock_client
    
    args = Namespace(volumes=False)
    down.run(args)
    
    # running コンテナは kill -> remove
    mock_container_running.kill.assert_called_once()
    mock_container_running.remove.assert_called_once_with(force=True)
    
    # stopped コンテナは remove のみ
    mock_container_stopped.kill.assert_not_called()
    mock_container_stopped.remove.assert_called_once_with(force=True)


@patch("docker.from_env")
@patch("subprocess.check_call")
def test_down_continues_on_container_error(mock_subprocess, mock_docker):
    """個別コンテナ削除失敗時も処理を継続すること"""
    mock_container = MagicMock()
    mock_container.status = "running"
    mock_container.name = "lambda-failing"
    mock_container.kill.side_effect = Exception("Kill failed")
    
    mock_client = MagicMock()
    mock_client.containers.list.return_value = [mock_container]
    mock_docker.return_value = mock_client
    
    args = Namespace(volumes=False, rmi=False)
    # 例外が発生しないこと (ワーニングのみ)
    down.run(args)
    
    mock_subprocess.assert_called_once()


@patch("docker.from_env")
@patch("subprocess.check_call")
def test_down_with_rmi(mock_subprocess, mock_docker):
    """down --rmi で --rmi all オプションが渡されること"""
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.return_value = mock_client
    
    args = Namespace(volumes=False, rmi=True)
    down.run(args)
    
    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "--rmi" in cmd
    assert "all" in cmd
