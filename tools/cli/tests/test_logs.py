"""Unit tests for esb logs command"""
import pytest
from unittest.mock import patch, MagicMock
from argparse import Namespace

from tools.cli.commands import logs


@patch("subprocess.run")
def test_logs_basic(mock_run):
    """Ensure logs runs docker compose logs."""
    args = Namespace(service=None, follow=False, tail=None, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert cmd == ["docker", "compose", "logs"]


@patch("subprocess.run")
def test_logs_with_service(mock_run):
    """Ensure logs [service] displays logs for a specific service."""
    args = Namespace(service="gateway", follow=False, tail=None, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "gateway" in cmd


@patch("subprocess.run")
def test_logs_follow(mock_run):
    """Ensure logs --follow passes the --follow option."""
    args = Namespace(service=None, follow=True, tail=None, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--follow" in cmd


@patch("subprocess.run")
def test_logs_tail(mock_run):
    """Ensure logs --tail N sets the latest N lines."""
    args = Namespace(service=None, follow=False, tail=50, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--tail" in cmd
    assert "50" in cmd


@patch("subprocess.run")
def test_logs_timestamps(mock_run):
    """Ensure logs --timestamps shows timestamps."""
    args = Namespace(service=None, follow=False, tail=None, timestamps=True)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--timestamps" in cmd


@patch("subprocess.run")
def test_logs_combined_options(mock_run):
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
