# Where: tools/cli/tests/test_logs.py
# What: Tests for the CLI logs command behavior.
# Why: Ensure compose invocation stays consistent across options.
"""Unit tests for esb logs command"""
import pytest
from unittest.mock import patch, MagicMock
from argparse import Namespace

from tools.cli.commands import logs
from tools.cli import config as cli_config


@patch("tools.cli.compose.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_CONTAINERD)
@patch("subprocess.run")
def test_logs_basic(mock_run, _mock_mode):
    """Ensure logs runs docker compose logs."""
    args = Namespace(service=None, follow=False, tail=None, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "docker" in cmd
    assert "compose" in cmd
    assert "logs" in cmd
    assert "-f" in cmd
    assert str(cli_config.COMPOSE_BASE_FILE) in cmd
    assert str(cli_config.COMPOSE_WORKER_FILE) in cmd
    assert str(cli_config.COMPOSE_CONTAINERD_FILE) in cmd


@patch("tools.cli.compose.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_CONTAINERD)
@patch("subprocess.run")
def test_logs_with_service(mock_run, _mock_mode):
    """Ensure logs [service] displays logs for a specific service."""
    args = Namespace(service="gateway", follow=False, tail=None, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "gateway" in cmd


@patch("tools.cli.compose.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_CONTAINERD)
@patch("subprocess.run")
def test_logs_follow(mock_run, _mock_mode):
    """Ensure logs --follow passes the --follow option."""
    args = Namespace(service=None, follow=True, tail=None, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--follow" in cmd


@patch("tools.cli.compose.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_CONTAINERD)
@patch("subprocess.run")
def test_logs_tail(mock_run, _mock_mode):
    """Ensure logs --tail N sets the latest N lines."""
    args = Namespace(service=None, follow=False, tail=50, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--tail" in cmd
    assert "50" in cmd


@patch("tools.cli.compose.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_CONTAINERD)
@patch("subprocess.run")
def test_logs_timestamps(mock_run, _mock_mode):
    """Ensure logs --timestamps shows timestamps."""
    args = Namespace(service=None, follow=False, tail=None, timestamps=True)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--timestamps" in cmd


@patch("tools.cli.compose.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_CONTAINERD)
@patch("subprocess.run")
def test_logs_combined_options(mock_run, _mock_mode):
    """Ensure combined options work correctly."""
    args = Namespace(service="manager", follow=True, tail=100, timestamps=True)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--follow" in cmd
    assert "--tail" in cmd
    assert "100" in cmd
    assert "--timestamps" in cmd
    assert "manager" in cmd
