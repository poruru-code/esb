# Where: e2e/runner/tests/test_utils_cli_bin.py
# What: Tests for E2E CLI binary selection priority.
# Why: Keep CLI_BIN canonical key behavior stable.
from __future__ import annotations

from e2e.runner import constants
from e2e.runner.utils import build_esb_cmd


def test_build_esb_cmd_prefers_cli_bin() -> None:
    env = {
        constants.ENV_CLI_BIN: "/tmp/app",
    }
    cmd = build_esb_cmd(["status"], None, env=env)
    assert cmd[0] == "/tmp/app"


def test_build_esb_cmd_falls_back_to_cli_name_from_defaults() -> None:
    cmd = build_esb_cmd(["status"], None, env={})
    assert cmd[0] == "esb"
