import os
import sys
from unittest.mock import MagicMock, patch

import pytest

# Mock data
TEST_ENV = "test_env_arg"


@pytest.fixture(autouse=True)
def mock_dependencies():
    """Mock out complex dependencies to prevent import-time side effects."""
    with patch.dict(sys.modules, {"services.gateway.config": MagicMock()}):
        yield


@pytest.fixture
def mock_env_vars():
    """Reset environment variables after each test."""
    old_environ = dict(os.environ)
    # Clear critical variables to ensure test isolation
    os.environ.pop("ESB_ENV", None)
    os.environ.pop("ESB_PROJECT_NAME", None)
    os.environ.pop("ESB_MODE", None)
    os.environ.pop("ESB_ENV_SET", None)

    yield
    os.environ.clear()
    os.environ.update(old_environ)


class Args:
    """Helper class to mock argparse arguments."""

    def __init__(self, **kwargs):
        self.env = kwargs.get("env")
        # Default flags for up/reset, ensuring all attrs are mockable
        self.detach = True
        self.build = False
        self.wait = False
        self.file = []
        self.rmi = False
        self.yes = True
        self.verbose = False
        for k, v in kwargs.items():
            setattr(self, k, v)


def test_up_uses_env_argument_over_default(mock_env_vars):
    """Test that 'up' command prioritizes args.env over default."""
    # Import inside test to ensure mock_env_vars is active
    from tools.cli.commands import up

    args = Args(env=TEST_ENV)

    with (
        patch("subprocess.check_call"),
        patch("subprocess.run"),
        patch("tools.cli.config.setup_environment"),
        patch("tools.cli.compose.build_compose_command"),
        patch("tools.cli.compose.resolve_compose_files", return_value=[]),
        patch("tools.cli.core.port_discovery.discover_ports", return_value={}),
        patch("tools.cli.core.port_discovery.save_ports"),
        patch("tools.cli.core.port_discovery.log_ports"),
        patch("tools.cli.core.context._validate_environment_initialized"),
        patch("tools.cli.core.context._validate_environment_exists"),
        patch("tools.cli.config.TEMPLATE_YAML", "fake.yaml"),
        patch("tools.provisioner.main.main"),
    ):
        # Simulate environment exists to bypass require_built=True check
        with patch("pathlib.Path.exists", return_value=True):
            try:
                up.run(args)
            except Exception:
                pass

        assert os.environ.get("ESB_ENV") == TEST_ENV


def test_up_fails_if_environment_not_built(mock_env_vars):
    """Test that 'up' command fails if the environment config does not exist."""
    # Import inside test
    from tools.cli.commands import up

    args = Args(env="non_existent_env")

    with patch("pathlib.Path.exists", return_value=False):
        with pytest.raises(SystemExit) as excinfo:
            with (
                patch("subprocess.check_call"),
                patch("tools.cli.compose.resolve_compose_files", return_value=[]),
                patch("tools.cli.config.setup_environment"),
            ):
                up.run(args)

        assert excinfo.value.code == 1


def test_reset_propagates_env_to_subcommands(mock_env_vars):
    """Test that 'reset' command passes env to down/up/build and they respect it."""
    # Import inside test
    from tools.cli.commands import reset

    args = Args(env=TEST_ENV)

    with (
        patch("tools.cli.commands.down.run"),
        patch("tools.cli.commands.build.run"),
        patch("tools.cli.commands.up.run"),
        patch("tools.cli.config.setup_environment"),
    ):
        # Simulate environment exists to bypass require_built=True check for 'up' which is called by 'reset'
        with patch("pathlib.Path.exists", return_value=True):
            reset.run(args)

        assert os.environ.get("ESB_ENV") == TEST_ENV


def test_main_arg_parsing_sets_env_and_calls_setup(mock_env_vars):
    """Test that main.py strictly prioritizes --env arg and calls setup_environment."""
    # Import inside test
    # We also need to reload main or ensure it's fresh if it was imported before
    import importlib

    from tools.cli import main

    importlib.reload(main)

    test_args = ["esb", "up", "--env", TEST_ENV]

    with (
        patch.object(sys, "argv", test_args),
        patch("tools.cli.commands.up.run"),
        patch("tools.cli.config.setup_environment") as mock_setup,
        patch("tools.cli.config.TEMPLATE_YAML", "fake.yaml"),
    ):
        try:
            main.main()
        except SystemExit:
            pass

        assert os.environ.get("ESB_ENV") == TEST_ENV
        assert mock_setup.called


def test_enforce_env_arg_sets_mode_from_generator(mock_env_vars, tmp_path):
    """Ensure generator.yml mode is applied when ESB_MODE is unset."""
    from tools.cli import config as cli_config
    from tools.cli.core import context

    (tmp_path / "generator.yml").write_text("environments:\n  test_env: containerd\n")
    args = Args(env="test_env")

    with patch.object(cli_config, "E2E_DIR", tmp_path), patch(
        "tools.cli.config.setup_environment"
    ):
        context.enforce_env_arg(args, skip_interactive=True)

    assert os.environ.get("ESB_MODE") == "containerd"
