from unittest.mock import MagicMock, patch

from tools.cli.commands.down import run as run_down
from tools.cli.commands.up import run as run_up


@patch("tools.cli.commands.up.context.enforce_env_arg")
@patch("subprocess.check_call")
@patch("tools.provisioner.main.main")
@patch("tools.cli.commands.build.run")
def test_up_command_flow(mock_build_run, mock_provisioner_main, mock_subprocess, mock_enforce):
    """Ensure the up command calls build, docker compose, and the provisioner."""
    args = MagicMock()
    args.build = True
    args.detach = True
    args.wait = False

    run_up(args)

    # 1. Ensure Phase 1 Build (generation) was called.
    mock_build_run.assert_called_once()

    # 2. Ensure docker compose up was called with --build.
    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "docker" in cmd
    assert "compose" in cmd
    assert "up" in cmd
    assert "-d" in cmd
    assert "--build" in cmd

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
