"""Unit tests for esb logs command"""
import pytest
from unittest.mock import patch, MagicMock
from argparse import Namespace

from tools.cli.commands import logs


@patch("subprocess.run")
def test_logs_basic(mock_run):
    """logs コマンドが docker compose logs を実行すること"""
    args = Namespace(service=None, follow=False, tail=None, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert cmd == ["docker", "compose", "logs"]


@patch("subprocess.run")
def test_logs_with_service(mock_run):
    """logs [service] で特定サービスのログを表示すること"""
    args = Namespace(service="gateway", follow=False, tail=None, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "gateway" in cmd


@patch("subprocess.run")
def test_logs_follow(mock_run):
    """logs --follow で --follow オプションが渡されること"""
    args = Namespace(service=None, follow=True, tail=None, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--follow" in cmd


@patch("subprocess.run")
def test_logs_tail(mock_run):
    """logs --tail N で最新N行が指定されること"""
    args = Namespace(service=None, follow=False, tail=50, timestamps=False)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--tail" in cmd
    assert "50" in cmd


@patch("subprocess.run")
def test_logs_timestamps(mock_run):
    """logs --timestamps でタイムスタンプが表示されること"""
    args = Namespace(service=None, follow=False, tail=None, timestamps=True)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--timestamps" in cmd


@patch("subprocess.run")
def test_logs_combined_options(mock_run):
    """複数オプションの組み合わせが正しく動作すること"""
    args = Namespace(service="manager", follow=True, tail=100, timestamps=True)
    logs.run(args)
    
    mock_run.assert_called_once()
    cmd = mock_run.call_args[0][0]
    assert "--follow" in cmd
    assert "--tail" in cmd
    assert "100" in cmd
    assert "--timestamps" in cmd
    assert "manager" in cmd
