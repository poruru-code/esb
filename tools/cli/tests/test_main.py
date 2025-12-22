import sys
import pytest
from unittest.mock import patch
from tools.cli.main import main


def test_cli_help(capsys):
    """--help が正常に動作するか確認"""
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


@patch("tools.cli.commands.build.run")
def test_cli_build_dispatch(mock_build_run):
    """build サブコマンドが正しくディスパッチされるか確認"""
    with patch.object(sys, "argv", ["esb", "build"]):
        main()
    mock_build_run.assert_called_once()


@patch("tools.cli.commands.up.run")
def test_cli_up_dispatch(mock_up_run):
    """up サブコマンドが正しくディスパッチされるか確認"""
    with patch.object(sys, "argv", ["esb", "up", "--build"]):
        main()
    mock_up_run.assert_called_once()
    args = mock_up_run.call_args[0][0]
    assert args.build is True
    assert args.detach is True  # デフォルト値
