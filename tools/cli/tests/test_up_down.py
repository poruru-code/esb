from unittest.mock import patch, MagicMock
from tools.cli.commands.up import run as run_up
from tools.cli.commands.down import run as run_down


@patch("subprocess.check_call")
@patch("tools.provisioner.main.main")
@patch("tools.cli.commands.build.run")
def test_up_command_flow(mock_build_run, mock_provisioner_main, mock_subprocess):
    """up コマンドが build, docker compose, provisioner を正しく呼び出すか確認"""
    args = MagicMock()
    args.build = True
    args.detach = True

    run_up(args)

    # 1. build が呼ばれたか
    mock_build_run.assert_called_once_with(args)

    # 2. docker compose up が呼ばれたか
    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "docker" in cmd
    assert "compose" in cmd
    assert "up" in cmd
    assert "-d" in cmd

    # 3. provisioner が呼ばれたか
    mock_provisioner_main.assert_called_once()


@patch("subprocess.check_call")
def test_down_command_flow(mock_subprocess):
    """down コマンドが docker compose down を呼び出すか確認"""
    args = MagicMock()
    run_down(args)

    mock_subprocess.assert_called_once()
    cmd = mock_subprocess.call_args[0][0]
    assert "down" in cmd
