import sys
import pytest
from unittest.mock import patch
from tools.cli.main import main


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
