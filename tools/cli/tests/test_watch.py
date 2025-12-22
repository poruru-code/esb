import pytest
from unittest.mock import patch, MagicMock
from pathlib import Path
from tools.cli.commands.watch import SmartReloader


@pytest.fixture
def reloader():
    with patch("docker.from_env"):
        return SmartReloader()


@patch("subprocess.run")
@patch("tools.generator.main.generate_files")
@patch("tools.provisioner.main.main")
def test_handle_template_change(mock_provisioner, mock_generate_files, mock_subprocess, reloader):
    """template.yaml 変更時のリロードフローを確認"""
    reloader.handle_template_change()

    mock_generate_files.assert_called_once()
    mock_subprocess.assert_called_once()
    args = mock_subprocess.call_args[0][0]
    assert "restart" in args
    assert "gateway" in args
    mock_provisioner.assert_called_once()


@patch("docker.from_env")
def test_handle_function_change(mock_docker_env, reloader):
    """関数コード変更時のイメージビルドとコンテナ停止フローを確認"""
    mock_client = MagicMock()
    reloader.docker_client = mock_client

    test_path = Path("tests/e2e/functions/hello/lambda_function.py")
    reloader.handle_function_change(test_path)

    # 1. イメージビルドが呼ばれたか
    mock_client.images.build.assert_called_once()
    kwargs = mock_client.images.build.call_args[1]
    assert "lambda-hello" in kwargs["tag"]

    # 2. 実行中コンテナの停止が呼ばれたか
    mock_client.containers.list.assert_called_once()
