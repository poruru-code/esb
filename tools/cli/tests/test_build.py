from unittest.mock import patch, MagicMock
from pathlib import Path
import pytest

from tools.cli.commands.build import run, build_base_image, build_function_images


# ============================================================
# build_base_image テスト
# ============================================================

@patch("tools.cli.commands.build.docker.from_env")
def test_build_base_image_success(mock_docker):
    """build_base_image が成功した場合に True を返すこと"""
    mock_client = MagicMock()
    mock_docker.return_value = mock_client
    
    with patch("tools.cli.commands.build.RUNTIME_DIR", Path("/tmp/fake_runtime")):
        with patch.object(Path, "exists", return_value=True):
            result = build_base_image(no_cache=False)
    
    assert result is True
    mock_client.images.build.assert_called_once()


@patch("tools.cli.commands.build.docker.from_env")
def test_build_base_image_dockerfile_not_found(mock_docker):
    """Dockerfile が存在しない場合に False を返すこと"""
    with patch("tools.cli.commands.build.RUNTIME_DIR", Path("/tmp/nonexistent")):
        result = build_base_image(no_cache=False)
    
    assert result is False


@patch("tools.cli.commands.build.docker.from_env")
def test_build_base_image_build_failure(mock_docker):
    """ビルドが失敗した場合に False を返すこと"""
    mock_client = MagicMock()
    mock_client.images.build.side_effect = Exception("Build failed")
    mock_docker.return_value = mock_client
    
    with patch("tools.cli.commands.build.RUNTIME_DIR", Path("/tmp/fake_runtime")):
        with patch.object(Path, "exists", return_value=True):
            result = build_base_image(no_cache=True)
    
    assert result is False


# ============================================================
# build_function_images テスト
# ============================================================

@patch("tools.cli.commands.build.docker.from_env")
def test_build_function_images_success(mock_docker, tmp_path):
    """関数イメージのビルドが成功すること"""
    mock_client = MagicMock()
    mock_docker.return_value = mock_client
    
    # ダミーの Dockerfile 作成
    dockerfile = tmp_path / "Dockerfile"
    dockerfile.write_text("FROM python:3.12")
    
    functions = [
        {
            "name": "test-func",
            "dockerfile_path": str(dockerfile),
            "context_path": str(tmp_path),
        }
    ]
    
    build_function_images(functions, template_path=str(tmp_path / "template.yaml"))
    
    mock_client.images.build.assert_called_once()


@patch("tools.cli.commands.build.docker.from_env")
def test_build_function_images_dockerfile_missing(mock_docker):
    """Dockerfile が存在しない場合にスキップすること"""
    mock_client = MagicMock()
    mock_docker.return_value = mock_client
    
    functions = [
        {
            "name": "missing-func",
            "dockerfile_path": "/nonexistent/Dockerfile",
            "context_path": "/nonexistent",
        }
    ]
    
    # 例外は発生しない（警告のみでスキップ）
    build_function_images(functions, template_path="/tmp/template.yaml")
    
    # ビルドは呼ばれない
    mock_client.images.build.assert_not_called()


@patch("tools.cli.commands.build.docker.from_env")
def test_build_function_images_build_failure_exits(mock_docker, tmp_path):
    """ビルド失敗時に sys.exit(1) すること"""
    mock_client = MagicMock()
    mock_client.images.build.side_effect = Exception("Build failed")
    mock_docker.return_value = mock_client
    
    dockerfile = tmp_path / "Dockerfile"
    dockerfile.write_text("FROM python:3.12")
    
    functions = [
        {
            "name": "failing-func",
            "dockerfile_path": str(dockerfile),
            "context_path": str(tmp_path),
        }
    ]
    
    with pytest.raises(SystemExit) as exc:
        build_function_images(functions, template_path=str(tmp_path / "template.yaml"), verbose=False)
    
    assert exc.value.code == 1


# ============================================================
# run() コマンド全体テスト
# ============================================================

@patch("tools.cli.commands.build.build_function_images")
@patch("tools.cli.commands.build.build_base_image")
@patch("tools.cli.commands.build.generator.generate_files")
@patch("tools.cli.commands.build.generator.load_config")
def test_build_command_flow(mock_load_config, mock_generate_files, mock_build_base, mock_build_funcs):
    """build コマンドが Generator と Docker ビルドを正しく呼び出すか確認"""
    mock_load_config.return_value = {"app": {"name": "", "tag": "latest"}, "paths": {}}
    mock_generate_files.return_value = [{"name": "test-func", "dockerfile_path": "/path/to/Dockerfile"}]
    mock_build_base.return_value = True

    args = MagicMock()
    args.dry_run = False
    args.verbose = False
    args.no_cache = True

    run(args)

    mock_generate_files.assert_called_once()
    mock_build_base.assert_called_once()
    mock_build_funcs.assert_called_once()


@patch("tools.cli.commands.build.generator.generate_files")
@patch("tools.cli.commands.build.generator.load_config")
def test_build_dry_run_mode(mock_load_config, mock_generate_files):
    """--dry-run モードでビルドがスキップされること"""
    mock_load_config.return_value = {"app": {}, "paths": {}}
    mock_generate_files.return_value = []

    args = MagicMock()
    args.dry_run = True
    args.verbose = False
    args.no_cache = False

    # dry_run モードではビルド関数は呼ばれない
    with patch("tools.cli.commands.build.build_base_image") as mock_base:
        run(args)
        mock_base.assert_not_called()


@patch("tools.cli.commands.build.build_function_images")
@patch("tools.cli.commands.build.build_base_image")
@patch("tools.cli.commands.build.generator.generate_files")
@patch("tools.cli.commands.build.generator.load_config")
def test_build_base_image_failure_exits(mock_load_config, mock_generate_files, mock_build_base, mock_build_funcs):
    """ベースイメージビルド失敗時に sys.exit(1) すること"""
    mock_load_config.return_value = {"app": {}, "paths": {}}
    mock_generate_files.return_value = []
    mock_build_base.return_value = False  # ビルド失敗

    args = MagicMock()
    args.dry_run = False
    args.verbose = False
    args.no_cache = False

    with pytest.raises(SystemExit) as exc:
        run(args)
    
    assert exc.value.code == 1
    mock_build_funcs.assert_not_called()  # 関数ビルドは呼ばれない
