import os

from tools.cli import config as cli_config


def normalize_mode(value: str | None) -> str | None:
    if not value:
        return None
    candidate = value.strip().lower()
    if candidate in cli_config.VALID_ESB_MODES:
        return candidate
    return None


def get_mode() -> str:
    # 1. Check ESB_MODE environment variable
    env_mode = normalize_mode(os.getenv("ESB_MODE"))
    if env_mode:
        return env_mode

    # 2. Fallback to default
    return cli_config.DEFAULT_ESB_MODE
