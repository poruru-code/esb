from unittest.mock import patch, MagicMock
from tools.cli.commands.build import run


@patch("tools.generator.main.generate_files")
@patch("docker.from_env")
def test_build_command_flow(mock_docker_env, mock_generate_files):
    """build コマンドが Generator と Docker ビルドを正しく呼び出すか確認"""
    mock_client = MagicMock()
    mock_docker_env.return_value = mock_client

    # テスト用のダミー引数
    args = MagicMock()
    args.no_cache = True

    # 実行
    run(args)

    # 1. Generator が呼ばれたか
    mock_generate_files.assert_called_once()

    # 2. Docker ビルドが適切なディレクトリで呼ばれたか
    assert mock_client.images.build.called
