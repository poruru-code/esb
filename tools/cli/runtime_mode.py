# Where: tools/cli/runtime_mode.py
# What: Load and store the ESB runtime mode in ~/.esb.
# Why: Keep mode selection consistent across CLI commands.
from pathlib import Path
from typing import Any

import yaml

from tools.cli import config as cli_config


def normalize_mode(value: str | None) -> str | None:
    if not value:
        return None
    candidate = value.strip().lower()
    if candidate in cli_config.VALID_ESB_MODES:
        return candidate
    return None


def _default_mode_config() -> dict[str, Any]:
    return {
        "version": cli_config.MODE_CONFIG_VERSION,
        "mode": cli_config.DEFAULT_ESB_MODE,
    }


def load_mode(path: Path | None = None) -> dict[str, Any]:
    config_path = path or cli_config.get_mode_config_path()
    if not config_path.exists():
        return _default_mode_config()
    try:
        data = yaml.safe_load(config_path.read_text())
    except Exception:
        return _default_mode_config()
    if not isinstance(data, dict):
        return _default_mode_config()
    mode = normalize_mode(str(data.get("mode", "")))
    if not mode:
        return _default_mode_config()
    data.setdefault("version", cli_config.MODE_CONFIG_VERSION)
    data["mode"] = mode
    return data


def get_mode(path: Path | None = None) -> str:
    return load_mode(path).get("mode", cli_config.DEFAULT_ESB_MODE)


def save_mode(mode: str, path: Path | None = None) -> dict[str, Any]:
    normalized = normalize_mode(mode)
    if not normalized:
        raise ValueError(
            f"Invalid mode: {mode}. Choose from {', '.join(cli_config.VALID_ESB_MODES)}."
        )
    data = {
        "version": cli_config.MODE_CONFIG_VERSION,
        "mode": normalized,
    }
    config_path = path or cli_config.get_mode_config_path()
    config_path.parent.mkdir(parents=True, exist_ok=True)
    config_path.write_text(yaml.safe_dump(data, sort_keys=False))
    return data
