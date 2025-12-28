from unittest.mock import patch, MagicMock
from pathlib import Path
import pytest

from tools.cli.commands.build import run, build_base_image, build_function_images


# ============================================================
# build_base_image tests
# ============================================================

@patch("tools.cli.commands.build.docker.from_env")
def test_build_base_image_success(mock_docker):
    """Return True when build_base_image succeeds."""
    mock_client = MagicMock()
    mock_docker.return_value = mock_client
    
    with patch("tools.cli.commands.build.RUNTIME_DIR", Path("/tmp/fake_runtime")):
        with patch.object(Path, "exists", return_value=True):
            result = build_base_image(no_cache=False)
    
    assert result is True
    mock_client.images.build.assert_called_once()


@patch("tools.cli.commands.build.docker.from_env")
def test_build_base_image_dockerfile_not_found(mock_docker):
    """Return False when the Dockerfile does not exist."""
    with patch("tools.cli.commands.build.RUNTIME_DIR", Path("/tmp/nonexistent")):
        result = build_base_image(no_cache=False)
    
    assert result is False


@patch("tools.cli.commands.build.docker.from_env")
def test_build_base_image_build_failure(mock_docker):
    """Return False when the build fails."""
    mock_client = MagicMock()
    mock_client.images.build.side_effect = Exception("Build failed")
    mock_docker.return_value = mock_client
    
    with patch("tools.cli.commands.build.RUNTIME_DIR", Path("/tmp/fake_runtime")):
        with patch.object(Path, "exists", return_value=True):
            result = build_base_image(no_cache=True)
    
    assert result is False


# ============================================================
# build_function_images tests
# ============================================================

@patch("tools.cli.commands.build.docker.from_env")
def test_build_function_images_success(mock_docker, tmp_path):
    """Ensure function image build succeeds."""
    mock_client = MagicMock()
    mock_docker.return_value = mock_client
    
    # Create a dummy Dockerfile.
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
    """Skip when the Dockerfile does not exist."""
    mock_client = MagicMock()
    mock_docker.return_value = mock_client
    
    functions = [
        {
            "name": "missing-func",
            "dockerfile_path": "/nonexistent/Dockerfile",
            "context_path": "/nonexistent",
        }
    ]
    
    # No exception should be raised (skip with warning).
    build_function_images(functions, template_path="/tmp/template.yaml")
    
    # Build should not be called.
    mock_client.images.build.assert_not_called()


@patch("tools.cli.commands.build.docker.from_env")
def test_build_function_images_build_failure_exits(mock_docker, tmp_path):
    """Exit with sys.exit(1) when the build fails."""
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
# run() command end-to-end tests
# ============================================================

@patch("tools.cli.commands.build.build_function_images")
@patch("tools.cli.commands.build.build_base_image")
@patch("tools.cli.commands.build.generator.generate_files")
@patch("tools.cli.commands.build.generator.load_config")
def test_build_command_flow(mock_load_config, mock_generate_files, mock_build_base, mock_build_funcs):
    """Ensure build calls the generator and Docker builds correctly."""
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
    """Ensure builds are skipped in --dry-run mode."""
    mock_load_config.return_value = {"app": {}, "paths": {}}
    mock_generate_files.return_value = []

    args = MagicMock()
    args.dry_run = True
    args.verbose = False
    args.no_cache = False

    # Build functions should not be called in dry_run mode.
    with patch("tools.cli.commands.build.build_base_image") as mock_base:
        run(args)
        mock_base.assert_not_called()


@patch("tools.cli.commands.build.build_function_images")
@patch("tools.cli.commands.build.build_base_image")
@patch("tools.cli.commands.build.generator.generate_files")
@patch("tools.cli.commands.build.generator.load_config")
def test_build_base_image_failure_exits(mock_load_config, mock_generate_files, mock_build_base, mock_build_funcs):
    """Exit with sys.exit(1) when base image build fails."""
    mock_load_config.return_value = {"app": {}, "paths": {}}
    mock_generate_files.return_value = []
    mock_build_base.return_value = False  # Build failure.

    args = MagicMock()
    args.dry_run = False
    args.verbose = False
    args.no_cache = False

    with pytest.raises(SystemExit) as exc:
        run(args)
    
    assert exc.value.code == 1
    mock_build_funcs.assert_not_called()  # Function builds should not be called.
# ============================================================
# Redirection to init tests (Merged from tests/unit)
# ============================================================

def test_build_redirects_to_init_when_config_missing_and_confirmed():
    """Call init when generator.yml is missing and the user selects Yes."""
    from tools.cli.commands import build, init
    from argparse import Namespace

    args = Namespace(dry_run=False, verbose=False, no_cache=False)
    
    with patch("tools.cli.commands.build.cli_config") as mock_cli_config:
        # Create a mock for config_path that exists() returns False
        mock_config_path = MagicMock()
        mock_config_path.exists.return_value = False
        
        # Mock E2E_DIR so that E2E_DIR / "generator.yml" returns our mock_config_path
        mock_cli_config.E2E_DIR = MagicMock()
        mock_cli_config.E2E_DIR.__truediv__.return_value = mock_config_path
        mock_cli_config.TEMPLATE_YAML = "/tmp/template.yaml"
        
        with patch("questionary.confirm") as mock_confirm, \
             patch("tools.cli.commands.init.run") as mock_init_run, \
             patch("tools.cli.commands.build.generator.load_config"), \
             patch("tools.cli.commands.build.generator.generate_files"), \
             patch("tools.cli.commands.build.build_base_image", return_value=True):

            mock_confirm.return_value.ask.return_value = True
            build.run(args)
            
            mock_init_run.assert_called_once()
            call_args = mock_init_run.call_args[0][0]
            assert call_args.template == "/tmp/template.yaml"


def test_build_aborts_when_config_missing_and_cancelled():
    """Exit when generator.yml is missing and the user selects No."""
    from tools.cli.commands import build
    from argparse import Namespace

    args = Namespace(dry_run=False, verbose=False, no_cache=False)
    
    with patch("tools.cli.commands.build.cli_config") as mock_cli_config:
        # Create a mock for config_path that exists() returns False
        mock_config_path = MagicMock()
        mock_config_path.exists.return_value = False
        
        # Mock E2E_DIR so that E2E_DIR / "generator.yml" returns our mock_config_path
        mock_cli_config.E2E_DIR = MagicMock()
        mock_cli_config.E2E_DIR.__truediv__.return_value = mock_config_path
        
        with patch("questionary.confirm") as mock_confirm, \
             patch("tools.cli.commands.init.run") as mock_init_run, \
             patch("tools.cli.commands.build.generator.load_config"), \
             patch("tools.cli.commands.build.generator.generate_files"), \
             patch("tools.cli.commands.build.build_base_image", return_value=True):
            
            mock_confirm.return_value.ask.return_value = False
            build.run(args)
            
            mock_init_run.assert_not_called()
