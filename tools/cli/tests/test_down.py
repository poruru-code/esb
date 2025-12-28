"""Unit tests for esb down command"""
import pytest
from unittest.mock import patch, MagicMock
from argparse import Namespace

from tools.cli.commands import down


@patch("docker.from_env")
@patch("subprocess.check_call")
def test_down_basic(mock_subprocess, mock_docker):
    """Ensure down runs docker compose down."""
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
    """Ensure down --volumes runs docker compose down --volumes."""
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
    """Ensure down cleans Lambda containers (created_by=sample-dind)."""
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
    
    # Running container: kill -> remove.
    mock_container_running.kill.assert_called_once()
    mock_container_running.remove.assert_called_once_with(force=True)
    
    # Stopped container: remove only.
    mock_container_stopped.kill.assert_not_called()
    mock_container_stopped.remove.assert_called_once_with(force=True)


@patch("docker.from_env")
@patch("subprocess.check_call")
def test_down_continues_on_container_error(mock_subprocess, mock_docker):
    """Ensure processing continues if an individual container removal fails."""
    mock_container = MagicMock()
    mock_container.status = "running"
    mock_container.name = "lambda-failing"
    mock_container.kill.side_effect = Exception("Kill failed")
    
    mock_client = MagicMock()
    mock_client.containers.list.return_value = [mock_container]
    mock_docker.return_value = mock_client
    
    args = Namespace(volumes=False, rmi=False)
    # No exception should be raised (warning only).
    down.run(args)
    
    mock_subprocess.assert_called_once()


@patch("docker.from_env")
@patch("subprocess.check_call")
def test_down_with_rmi(mock_subprocess, mock_docker):
    """Ensure down --rmi passes the --rmi all option."""
    mock_client = MagicMock()
    mock_client.containers.list.return_value = []
    mock_docker.return_value = mock_client
    
    args = Namespace(volumes=False, rmi=True)
    down.run(args)
    
    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "--rmi" in cmd
    assert "all" in cmd
