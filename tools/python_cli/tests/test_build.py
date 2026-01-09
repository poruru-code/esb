from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from tools.python_cli import config as cli_config
from tools.python_cli.commands.build import build_base_image, build_function_images, run
from tools.python_cli.core import proxy


@pytest.fixture(autouse=True)
def _force_containerd_mode(monkeypatch):
    monkeypatch.setattr(
        "tools.python_cli.commands.build.runtime_mode.get_mode",
        lambda: cli_config.ESB_MODE_CONTAINERD,
    )


@pytest.fixture(autouse=True)
def _mock_context_validation(monkeypatch, request):
    """Mock environment validation and staging side effects to avoid sys.exit and FileNotFoundError."""
    # We want to keep enforce_env_arg mostly working but avoid its exit behavior in most tests
    if "no_mock_enforce" not in request.keywords:
        monkeypatch.setattr(
            "tools.python_cli.commands.build.context.enforce_env_arg", lambda *a, **kw: None
        )

    # Mock staging to avoid side effects on disk during unit tests
    monkeypatch.setattr("tools.python_cli.commands.build.shutil.rmtree", lambda *a, **kw: None)
    monkeypatch.setattr("tools.python_cli.commands.build.shutil.copy2", lambda *a, **kw: None)
    # DO NOT mock Path.mkdir globally as it breaks pytest/tmp_path
    yield


# ============================================================
# build_base_image tests
# ============================================================


@patch("tools.python_cli.commands.build.docker.from_env")
def test_build_base_image_success(mock_docker):
    """Return True when build_base_image succeeds."""
    mock_client = MagicMock()
    mock_docker.return_value = mock_client

    with patch("tools.python_cli.commands.build.RUNTIME_DIR", Path("/tmp/fake_runtime")):
        with patch.object(Path, "exists", return_value=True):
            result = build_base_image(no_cache=False)

    assert result is True
    mock_client.images.build.assert_called_once()
    expected_args = proxy.docker_build_args()
    assert mock_client.images.build.call_args.kwargs.get("buildargs") == expected_args


@patch("tools.python_cli.commands.build.docker.from_env")
def test_build_base_image_dockerfile_not_found(mock_docker):
    """Return False when the Dockerfile does not exist."""
    with patch("tools.python_cli.commands.build.RUNTIME_DIR", Path("/tmp/nonexistent")):
        result = build_base_image(no_cache=False)

    assert result is False


@patch("tools.python_cli.commands.build.docker.from_env")
def test_build_base_image_build_failure(mock_docker):
    """Return False when the build fails."""
    mock_client = MagicMock()
    mock_client.images.build.side_effect = Exception("Build failed")
    mock_docker.return_value = mock_client

    with patch("tools.python_cli.commands.build.RUNTIME_DIR", Path("/tmp/fake_runtime")):
        with patch.object(Path, "exists", return_value=True):
            result = build_base_image(no_cache=True)

    assert result is False


@patch("tools.python_cli.commands.build.docker.from_env")
def test_build_base_image_respects_proxy_env(mock_docker, monkeypatch):
    """Ensure proxy environment variables are forwarded to docker builds."""
    monkeypatch.setenv("HTTP_PROXY", "http://proxy.internal:3128")
    monkeypatch.setenv("NO_PROXY", "127.0.0.1")
    mock_client = MagicMock()
    mock_docker.return_value = mock_client

    with patch("tools.python_cli.commands.build.RUNTIME_DIR", Path("/tmp/fake_runtime")):
        with patch.object(Path, "exists", return_value=True):
            build_base_image(no_cache=False)

    buildargs = mock_client.images.build.call_args.kwargs.get("buildargs") or {}
    assert buildargs["HTTP_PROXY"] == "http://proxy.internal:3128"
    assert "localhost" in buildargs["NO_PROXY"]


# ============================================================
# build_function_images tests
# ============================================================


@patch("tools.python_cli.commands.build.docker.from_env")
def test_build_function_images_success(mock_docker, tmp_path):
    """Ensure function image build succeeds."""
    mock_client = MagicMock()
    mock_docker.return_value = mock_client

    context_dir = tmp_path / "out"
    dockerfile_dir = context_dir / "functions" / "test-func"
    dockerfile_dir.mkdir(parents=True, exist_ok=True)

    # Create a dummy Dockerfile.
    dockerfile = dockerfile_dir / "Dockerfile"
    dockerfile.write_text("FROM python:3.12")

    functions = [
        {
            "name": "test-func",
            "dockerfile_path": str(dockerfile),
            "context_path": str(context_dir),
        }
    ]

    build_function_images(functions, template_path=str(tmp_path / "template.yaml"))

    mock_client.images.build.assert_called_once()
    expected_args = proxy.docker_build_args()
    build_kwargs = mock_client.images.build.call_args.kwargs
    assert build_kwargs.get("buildargs") == expected_args
    assert build_kwargs.get("dockerfile") == "functions/test-func/Dockerfile"

    dockerignore_path = context_dir / ".dockerignore"
    assert dockerignore_path.exists()
    dockerignore = dockerignore_path.read_text(encoding="utf-8")
    assert "!functions/test-func/" in dockerignore
    assert "!layers/" in dockerignore


@patch("tools.python_cli.commands.build.docker.from_env")
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


@patch("tools.python_cli.commands.build.docker.from_env")
def test_build_function_images_build_failure_exits(mock_docker, tmp_path):
    """Exit with sys.exit(1) when the build fails."""
    mock_client = MagicMock()
    mock_client.images.build.side_effect = RuntimeError("Build failed")
    mock_docker.return_value = mock_client

    context_dir = tmp_path / "out"
    dockerfile_dir = context_dir / "functions" / "failing-func"
    dockerfile_dir.mkdir(parents=True, exist_ok=True)

    dockerfile = dockerfile_dir / "Dockerfile"
    dockerfile.write_text("FROM python:3.12")

    functions = [
        {
            "name": "failing-func",
            "dockerfile_path": str(dockerfile),
            "context_path": str(context_dir),
        }
    ]

    with pytest.raises(SystemExit) as exc:
        build_function_images(
            functions, template_path=str(tmp_path / "template.yaml"), verbose=False
        )

    assert exc.value.code == 1


# ============================================================
# run() command end-to-end tests
# ============================================================


@patch("tools.python_cli.commands.build.ensure_registry_running")
@patch("tools.python_cli.commands.build.build_function_images")
@patch("tools.python_cli.commands.build.build_base_image")
@patch("tools.python_cli.commands.build.generator.generate_files")
@patch("tools.python_cli.commands.build.generator.load_config")
def test_build_command_flow(
    mock_load_config, mock_generate_files, mock_build_base, mock_build_funcs, mock_ensure_registry
):
    """Ensure build calls the generator and Docker builds correctly."""
    mock_load_config.return_value = {"app": {"name": "", "tag": "latest"}, "paths": {}}
    mock_generate_files.return_value = [
        {"name": "test-func", "dockerfile_path": "/path/to/Dockerfile"}
    ]
    mock_build_base.return_value = True

    args = MagicMock()
    args.dry_run = False
    args.verbose = False
    args.no_cache = True

    run(args)

    mock_generate_files.assert_called_once()
    mock_build_base.assert_called_once()
    mock_build_funcs.assert_called_once()


@patch("tools.python_cli.commands.build.ensure_registry_running")
@patch("tools.python_cli.commands.build.build_service_images.build_and_push", return_value=True)
@patch("tools.python_cli.commands.build.build_function_images")
@patch("tools.python_cli.commands.build.build_base_image")
@patch("tools.python_cli.commands.build.generator.generate_files")
@patch("tools.python_cli.commands.build.generator.load_config")
def test_build_command_firecracker_builds_service_images(
    mock_load_config,
    mock_generate_files,
    mock_build_base,
    mock_build_funcs,
    mock_build_services,
    mock_ensure_registry,
    monkeypatch,
):
    """Build service images when running in firecracker mode."""
    monkeypatch.setattr(
        "tools.python_cli.commands.build.runtime_mode.get_mode",
        lambda: cli_config.ESB_MODE_FIRECRACKER,
    )
    mock_load_config.return_value = {"app": {"name": "", "tag": "latest"}, "paths": {}}
    mock_generate_files.return_value = [
        {"name": "test-func", "dockerfile_path": "/path/to/Dockerfile"}
    ]
    mock_build_base.return_value = True

    args = MagicMock()
    args.dry_run = False
    args.verbose = False
    args.no_cache = False

    run(args)

    mock_build_services.assert_called_once()


@patch("tools.python_cli.commands.build.generator.generate_files")
@patch("tools.python_cli.commands.build.generator.load_config")
def test_build_dry_run_mode(mock_load_config, mock_generate_files):
    """Ensure builds are skipped in --dry-run mode."""
    mock_load_config.return_value = {"app": {}, "paths": {}}
    mock_generate_files.return_value = []

    args = MagicMock()
    args.dry_run = True
    args.verbose = False
    args.no_cache = False

    # Build functions should not be called in dry_run mode.
    with patch("tools.python_cli.commands.build.build_base_image") as mock_base:
        run(args)
        mock_base.assert_not_called()


@patch("tools.python_cli.commands.build.ensure_registry_running")
@patch("tools.python_cli.commands.build.build_function_images")
@patch("tools.python_cli.commands.build.build_base_image")
@patch("tools.python_cli.commands.build.generator.generate_files")
@patch("tools.python_cli.commands.build.generator.load_config")
def test_build_base_image_failure_exits(
    mock_load_config,
    mock_generate_files,
    mock_build_base,
    mock_build_funcs,
    mock_ensure_registry,
):
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
# Redirection to init tests (Merged from unit tests)
# ============================================================


@pytest.mark.no_mock_enforce
def test_build_redirects_to_init_when_config_missing_and_confirmed(tmp_path):
    """Call init when generator.yml is missing and the user selects Yes."""
    from argparse import Namespace

    from tools.python_cli.commands import build

    args = Namespace(dry_run=False, verbose=False, no_cache=False)

    with patch("tools.python_cli.commands.build.cli_config") as mock_cli_config:
        # Create a mock for config_path that exists() returns False
        mock_config_path = MagicMock()
        mock_config_path.exists.return_value = False

        # Mock E2E_DIR so that E2E_DIR / "generator.yml" returns our mock_config_path
        mock_cli_config.PROJECT_ROOT = tmp_path
        mock_cli_config.E2E_DIR = mock_cli_config.PROJECT_ROOT / ".esb"
        mock_cli_config.TEMPLATE_YAML = Path("/tmp/template.yaml")
        mock_cli_config.get_generator_config_path.return_value = mock_config_path
        mock_cli_config.get_esb_home.return_value = mock_cli_config.PROJECT_ROOT / ".esb"
        mock_cli_config.get_env_name.return_value = "default"
        mock_cli_config.get_port_mapping.return_value = {}
        mock_cli_config.get_registry_config.return_value = {
            "external": "localhost:5010",
            "internal": "registry:5010",
        }

        with (
            patch("questionary.confirm") as mock_confirm,
            patch("tools.python_cli.commands.init.run"),
            patch("tools.python_cli.commands.build.generator.load_config"),
            patch("tools.python_cli.commands.build.generator.generate_files"),
            patch("tools.python_cli.commands.build.build_base_image", return_value=True),
        ):
            mock_confirm.return_value.ask.return_value = True
            # Now it should exit via enforce_env_arg(require_initialized=True)
            with pytest.raises(SystemExit) as exc:
                build.run(args)
            assert exc.value.code == 1


@pytest.mark.no_mock_enforce
def test_build_aborts_when_config_missing_and_cancelled(tmp_path):
    """Exit when generator.yml is missing and the user selects No."""
    from argparse import Namespace

    from tools.python_cli.commands import build

    args = Namespace(dry_run=False, verbose=False, no_cache=False)

    with patch("tools.python_cli.commands.build.cli_config") as mock_cli_config:
        # Create a mock for config_path that exists() returns False
        mock_config_path = MagicMock()
        mock_config_path.exists.return_value = False

        # Mock E2E_DIR so that E2E_DIR / "generator.yml" returns our mock_config_path
        mock_cli_config.PROJECT_ROOT = tmp_path
        mock_cli_config.E2E_DIR = mock_cli_config.PROJECT_ROOT / ".esb"
        mock_cli_config.get_generator_config_path.return_value = mock_config_path
        mock_cli_config.get_esb_home.return_value = mock_cli_config.PROJECT_ROOT / ".esb"
        mock_cli_config.get_env_name.return_value = "default"
        mock_cli_config.get_port_mapping.return_value = {}
        mock_cli_config.get_registry_config.return_value = {
            "external": "localhost:5010",
            "internal": "registry:5010",
        }

        with (
            patch("questionary.confirm") as mock_confirm,
            patch("tools.python_cli.commands.init.run"),
            patch("tools.python_cli.commands.build.generator.load_config"),
            patch("tools.python_cli.commands.build.generator.generate_files"),
            patch("tools.python_cli.commands.build.build_base_image", return_value=True),
        ):
            mock_confirm.return_value.ask.return_value = False
            # Should exit via enforce_env_arg
            with pytest.raises(SystemExit) as exc:
                build.run(args)
            assert exc.value.code == 1


@patch("tools.python_cli.commands.build.docker.from_env")
def test_build_base_image_without_registry(mock_docker, monkeypatch):
    """Build base image locally when CONTAINER_REGISTRY is not set."""
    monkeypatch.delenv("CONTAINER_REGISTRY", raising=False)
    mock_client = MagicMock()
    mock_docker.return_value = mock_client

    with patch("tools.python_cli.commands.build.RUNTIME_DIR", Path("/tmp/fake_runtime")):
        with patch.object(Path, "exists", return_value=True):
            result = build_base_image(no_cache=False)

    assert result is True
    # Ensure push was NOT called
    mock_client.images.push.assert_not_called()


@patch("tools.python_cli.commands.build.docker.from_env")
def test_build_function_images_without_registry(mock_docker, monkeypatch, tmp_path):
    """Build function images locally when CONTAINER_REGISTRY is not set."""
    monkeypatch.delenv("CONTAINER_REGISTRY", raising=False)
    mock_client = MagicMock()
    mock_docker.return_value = mock_client

    # Create a dummy Dockerfile.
    context_dir = tmp_path / "out"
    dockerfile_dir = context_dir / "functions" / "test-func"
    dockerfile_dir.mkdir(parents=True, exist_ok=True)

    dockerfile = dockerfile_dir / "Dockerfile"
    dockerfile.write_text("FROM python:3.12")

    functions = [
        {
            "name": "test-func",
            "dockerfile_path": str(dockerfile),
            "context_path": str(context_dir),
        }
    ]

    build_function_images(functions, template_path=str(tmp_path / "template.yaml"))

    assert mock_client.images.build.called
    # Ensure push was NOT called
    mock_client.images.push.assert_not_called()


@patch("tools.python_cli.commands.build.ensure_registry_running")
@patch("tools.python_cli.commands.build.build_function_images")
@patch("tools.python_cli.commands.build.build_base_image", return_value=True)
@patch("tools.python_cli.commands.build.generator.generate_files")
@patch("tools.python_cli.commands.build.generator.load_config")
@patch("tools.python_cli.commands.build.shutil.copy2")
@patch("tools.python_cli.commands.build.shutil.rmtree")
@patch("tools.python_cli.commands.build.cli_config.get_build_output_dir")
@patch("tools.python_cli.commands.build.cli_config.get_env_name", return_value="testenv")
def test_build_staging_happens_after_generation(
    mock_get_env_name,
    mock_get_output_dir,
    mock_rmtree,
    mock_copy2,
    mock_load_config,
    mock_generate_files,
    mock_build_base,
    mock_build_funcs,
    mock_ensure_registry,
    tmp_path,
    monkeypatch,
):
    """Ensure that configuration staging (copying files) happens AFTER file generation."""
    # Setup paths
    build_dir = tmp_path / "build"
    config_dir = build_dir / "config"
    config_dir.mkdir(parents=True)

    # Create dummy config files so copy logic triggers
    (config_dir / "functions.yml").touch()
    (config_dir / "routing.yml").touch()

    mock_get_output_dir.return_value = build_dir

    # Track order
    manager = MagicMock()
    manager.attach_mock(mock_generate_files, "generate_files")
    manager.attach_mock(mock_copy2, "copy2")

    mock_load_config.return_value = {"app": {}, "paths": {}}
    mock_generate_files.return_value = []

    # Run
    args = MagicMock()
    args.dry_run = False
    args.verbose = False
    args.no_cache = False

    run(args)

    # Verify order
    # generate_files must be called
    assert mock_generate_files.called
    # copy2 must be called (for functions.yml and routing.yml)
    assert mock_copy2.called

    # Check that the FIRST copy2 call happened AFTER generate_files
    generate_idx = -1
    first_copy_idx = -1

    for i, call in enumerate(manager.mock_calls):
        name = call[0]
        if name == "generate_files":
            generate_idx = i
        elif name == "copy2" and first_copy_idx == -1:
            first_copy_idx = i

    assert generate_idx != -1
    assert first_copy_idx != -1
    assert generate_idx < first_copy_idx, (
        f"generate_files ({generate_idx}) should be called before copy2 ({first_copy_idx})"
    )
