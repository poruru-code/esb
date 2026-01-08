# Where: tools/cli/tests/test_main.py
# What: CLI entrypoint tests for command dispatch.
# Why: Validate argument parsing and routing behavior.
import sys
from unittest.mock import patch

import pytest

from tools.cli.main import main


@pytest.fixture(autouse=True)
def mock_template(monkeypatch):
    """Mock TEMPLATE_YAML to avoid early exit in main()."""
    monkeypatch.setattr("tools.cli.config.TEMPLATE_YAML", "fake-template.yaml")
    # Also skip context validation in main tests as they are dispatch tests
    monkeypatch.setattr("tools.cli.core.context._validate_environment_initialized", lambda *args, **kwargs: None)
    monkeypatch.setattr("tools.cli.core.context._validate_environment_exists", lambda *args, **kwargs: None)
    yield


def test_cli_help(capsys):
    """Ensure --help works correctly."""
    with patch.object(sys, "argv", ["esb", "--help"]):
        with pytest.raises(SystemExit) as e:
            main()
        assert e.value.code == 0

    captured = capsys.readouterr()
    assert "Edge Serverless Box CLI" in captured.out
    assert "build" in captured.out
    assert "up" in captured.out
    assert "watch" in captured.out
    assert "down" in captured.out
    assert "init" in captured.out
    assert "node" in captured.out


@patch("tools.cli.commands.build.run")
def test_cli_build_dispatch(mock_build_run):
    """Ensure the build subcommand is dispatched correctly."""
    with patch.object(sys, "argv", ["esb", "build"]):
        main()
    mock_build_run.assert_called_once()


@patch("tools.cli.commands.up.run")
def test_cli_up_dispatch(mock_up_run):
    """Ensure the up subcommand is dispatched correctly."""
    with patch.object(sys, "argv", ["esb", "up", "--build"]):
        main()
    mock_up_run.assert_called_once()
    args = mock_up_run.call_args[0][0]
    assert args.build is True
    assert args.detach is True  # Default value.


@patch("tools.cli.commands.init.run")
def test_cli_init_dispatch(mock_init_run):
    """Ensure the init subcommand is dispatched correctly."""
    with patch.object(sys, "argv", ["esb", "init"]):
        main()
    mock_init_run.assert_called_once()


@patch("tools.cli.commands.init.run")
@patch("tools.cli.config.set_template_yaml")
def test_cli_template_argument(mock_set_template, mock_init_run):
    """Ensure --template calls set_template_yaml."""
    with patch.object(sys, "argv", ["esb", "--template", "/path/to/template.yaml", "init"]):
        main()
    mock_set_template.assert_called_once_with("/path/to/template.yaml")
    mock_init_run.assert_called_once()


@patch("tools.cli.commands.down.run")
def test_cli_down_dispatch(mock_down_run):
    """Ensure the down subcommand is dispatched correctly."""
    with patch.object(sys, "argv", ["esb", "down"]):
        main()
    mock_down_run.assert_called_once()


@patch("tools.cli.commands.down.run")
def test_cli_down_volumes_flag(mock_down_run):
    """Ensure the down --volumes flag is passed correctly."""
    with patch.object(sys, "argv", ["esb", "down", "--volumes"]):
        main()
    mock_down_run.assert_called_once()
    args = mock_down_run.call_args[0][0]
    assert args.volumes is True


@patch("tools.cli.commands.logs.run")
def test_cli_logs_dispatch(mock_logs_run):
    """Ensure the logs subcommand is dispatched correctly."""
    with patch.object(sys, "argv", ["esb", "logs"]):
        main()
    mock_logs_run.assert_called_once()


@patch("tools.cli.commands.node.run")
def test_cli_node_add_dispatch(mock_node_run):
    """Ensure the node add subcommand is dispatched correctly."""
    with patch.object(sys, "argv", ["esb", "node", "add"]):
        main()
    mock_node_run.assert_called_once()


@patch("tools.cli.commands.node.run")
def test_cli_node_doctor_dispatch(mock_node_run):
    """Ensure the node doctor subcommand is dispatched correctly."""
    with patch.object(sys, "argv", ["esb", "node", "doctor"]):
        main()
    mock_node_run.assert_called_once()


@patch("tools.cli.commands.node.run")
def test_cli_node_provision_dispatch(mock_node_run):
    """Ensure the node provision subcommand is dispatched correctly."""
    with patch.object(sys, "argv", ["esb", "node", "provision"]):
        main()
    mock_node_run.assert_called_once()


@patch("tools.cli.commands.node.run")
def test_cli_node_up_dispatch(mock_node_run):
    """Ensure the node up subcommand is dispatched correctly."""
    with patch.object(sys, "argv", ["esb", "node", "up"]):
        main()
    mock_node_run.assert_called_once()






@patch("tools.cli.commands.logs.run")
def test_cli_logs_with_options(mock_logs_run):
    """Ensure logs options are passed correctly."""
    with patch.object(sys, "argv", ["esb", "logs", "gateway", "-f", "--tail", "100"]):
        main()
    mock_logs_run.assert_called_once()
    args = mock_logs_run.call_args[0][0]
    assert args.service == "gateway"
    assert args.follow is True
    assert args.tail == 100
