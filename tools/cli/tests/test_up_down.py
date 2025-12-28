from unittest.mock import patch, MagicMock
from tools.cli.commands.up import run as run_up
from tools.cli.commands.down import run as run_down


@patch("subprocess.check_call")
@patch("tools.provisioner.main.main")
@patch("tools.cli.commands.build.run")
def test_up_command_flow(mock_build_run, mock_provisioner_main, mock_subprocess):
    """Ensure the up command calls build, docker compose, and the provisioner."""
    args = MagicMock()
    args.build = True
    args.detach = True

    run_up(args)

    # 1. Ensure build was called.
    mock_build_run.assert_called_once_with(args)

    # 2. Ensure docker compose up was called.
    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "docker" in cmd
    assert "compose" in cmd
    assert "up" in cmd
    assert "-d" in cmd

    # 3. Ensure the provisioner was called.
    mock_provisioner_main.assert_called_once()


@patch("subprocess.check_call")
def test_down_command_flow(mock_subprocess):
    """Ensure the down command calls docker compose down."""
    args = MagicMock()
    args.volumes = False
    run_down(args)

    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "down" in cmd
    assert "--volumes" not in cmd


@patch("subprocess.check_call")
def test_down_command_volumes_option(mock_subprocess):
    """Ensure down --volumes passes the --volumes option."""
    args = MagicMock()
    args.volumes = True
    run_down(args)

    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "--volumes" in cmd
