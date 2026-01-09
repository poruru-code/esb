# Where: tools/python_cli/tests/test_compose.py
# What: Tests for compose file resolution based on runtime mode.
# Why: Ensure control/compute stacks map to the intended compose files.
from unittest.mock import patch

from tools.python_cli import compose as cli_compose
from tools.python_cli import config as cli_config


def test_resolve_compose_files_containerd_control():
    with patch(
        "tools.python_cli.compose.runtime_mode.get_mode",
        return_value=cli_config.ESB_MODE_CONTAINERD,
    ):
        files = cli_compose.resolve_compose_files(target="control")
    assert files == [
        cli_config.COMPOSE_BASE_FILE,
        cli_config.COMPOSE_WORKER_FILE,
        cli_config.COMPOSE_REGISTRY_FILE,
        cli_config.COMPOSE_CONTAINERD_FILE,
    ]


def test_resolve_compose_files_firecracker_control():
    with patch(
        "tools.python_cli.compose.runtime_mode.get_mode",
        return_value=cli_config.ESB_MODE_FIRECRACKER,
    ):
        files = cli_compose.resolve_compose_files(target="control")
    assert files == [
        cli_config.COMPOSE_BASE_FILE,
        cli_config.COMPOSE_WORKER_FILE,
        cli_config.COMPOSE_REGISTRY_FILE,
        cli_config.COMPOSE_FC_FILE,
    ]


def test_build_compose_command_containerd_order():
    command = cli_compose.build_compose_command(
        ["up", "-d"], mode=cli_config.ESB_MODE_CONTAINERD, target="control"
    )
    assert command == [
        "docker",
        "compose",
        "-f",
        str(cli_config.COMPOSE_BASE_FILE),
        "-f",
        str(cli_config.COMPOSE_WORKER_FILE),
        "-f",
        str(cli_config.COMPOSE_REGISTRY_FILE),
        "-f",
        str(cli_config.COMPOSE_CONTAINERD_FILE),
        "up",
        "-d",
    ]
