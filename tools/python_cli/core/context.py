# Where: tools/python_cli/core/context.py
# What: Centralized context management and validation for CLI commands.
# Why: Ensure consistent environment argument handling and pre-execution validation.

import argparse
import os
import sys
from typing import Any, cast

import questionary  # Added for interactive selection

from tools.python_cli import config as cli_config
from tools.python_cli.core import logging


def enforce_env_arg(
    args: argparse.Namespace,
    require_initialized: bool = False,
    require_built: bool = False,
    skip_interactive: bool = False,
) -> None:
    """
    Ensure environment setup based on CLI args and optional validation.

    Args:
        args: Parsed arguments object (expected to optionally have 'env').
        require_initialized: If True, validates that the environment is initialized
                             (generator.yml exists).
        require_built: If True, validates that the target environment has been built
                       (config exists). Raises SystemExit(1) if validation fails.
        skip_interactive: If True, skip interactive prompts (for internal command calls).
    """
    # 1. Prioritize argument-provided environment
    target_env = getattr(args, "env", None)
    if target_env is not None and not isinstance(target_env, str):
        # In tests, a MagicMock might be returned. We should ignore it unless it's a string.
        target_env = None

    # 2. If no --env arg, check if ESB_ENV was set by a PARENT CLI command (marked by ESB_ENV_SET)
    #    We don't use shell-level ESB_ENV to avoid confusion - user must explicitly select
    if not target_env and os.environ.get("ESB_ENV_SET") == "1":
        target_env = os.environ.get("ESB_ENV")

    # 3. If still no env and not skipping interactive, prompt the user to select
    if not target_env and not skip_interactive:
        target_env = _prompt_environment_selection()

    if target_env:
        mode = _resolve_env_mode(target_env)
        if mode and not os.environ.get("ESB_MODE"):
            os.environ["ESB_MODE"] = mode

        # Override the process environment variable.
        # This is critical for commands that rely on os.getenv("ESB_ENV") directly or
        # via config.get_env_name().
        os.environ["ESB_ENV"] = target_env
        # Mark that this was set by the CLI, so child commands can inherit without prompting
        os.environ["ESB_ENV_SET"] = "1"

        # Re-run environment setup to update dependent variables (ports, networks, etc.)
        # This ensures that even if setup_environment() was called early (e.g. in main.py),
        # it is refreshed with the authoritative environment.
        cli_config.setup_environment(target_env)

    # 4. Validation (if requested)
    if getattr(args, "require_initialized", False) or require_initialized:
        _validate_environment_initialized()

    if require_built:
        _validate_environment_exists()


def _prompt_environment_selection() -> str | None:
    """
    Read environments from generator.yml and prompt user to select one.
    Returns the selected environment name or None if cancelled/unavailable.
    """
    import yaml

    config_path = cli_config.E2E_DIR / "generator.yml"

    if not config_path.exists():
        logging.error("No generator.yml found. Please run 'esb init' first.")
        print(f"\nConfiguration file not found at: {config_path}")
        sys.exit(1)

    try:
        with open(config_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f) or {}
            env_modes = _normalize_env_modes(data.get("environments", []))
            environments = list(env_modes.keys())
    except Exception as e:
        logging.error(f"Failed to read generator.yml: {e}")
        sys.exit(1)

    if not environments:
        logging.error("No environments are initialized. Please run 'esb init' first.")
        sys.exit(1)

    if len(environments) == 1:
        # Auto-select if only one environment exists
        return environments[0]

    selected = questionary.select("Select an environment:", choices=environments).ask()

    if selected is None:
        print("Aborted.")
        sys.exit(0)

    return selected


def _validate_environment_initialized() -> None:
    """
    Check if the environment (ESB_ENV) has been initialized (generator.yml exists).
    Fails with usage instructions if 'generator.yml' is missing.
    """
    env_name = cli_config.get_env_name()
    # generator.yml resides in the template directory (E2E_DIR)
    config_path = cli_config.E2E_DIR / "generator.yml"

    if not config_path.exists():
        logging.error(f"Environment '{env_name}' is not initialized.")
        print(f"\nConfiguration file not found at: {config_path}")
        print("Please run the init command first:")
        print(f"  esb init --env={env_name}")
        sys.exit(1)

    try:
        import yaml

        with open(config_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f) or {}
            env_modes = _normalize_env_modes(data.get("environments", []))
            initialized_envs = list(env_modes.keys())

            if env_name not in env_modes:
                logging.error(f"Environment '{env_name}' is not initialized in {config_path.name}.")
                env_str = ", ".join(initialized_envs) if initialized_envs else "None"
                print(f"\nInitialized environments: {env_str}")
                print("Please run the init command for this environment:")
                print(f"  esb init --env={env_name}")
                sys.exit(1)
    except Exception as e:
        logging.error(f"Failed to validate environment initialization: {e}")
        sys.exit(1)


def _validate_environment_exists() -> None:
    """
    Check if the current environment (ESB_ENV) has been initialized/built.
    Fails with usage instructions if 'config' directory is missing in .esb/<env>.
    """
    env_name = cli_config.get_env_name()
    # We define "built" as having a configuration directory.
    # Config is generated in <output_dir>/<env_name>/config (default .esb relative to project)
    # dependent on generator.yml settings.
    env_config_root = cli_config.get_build_output_dir(env_name)
    config_dir = env_config_root / "config"

    # Note: We rely on the fact that 'esb build' creates this structure.
    if not config_dir.exists():
        logging.error(f"Environment '{env_name}' is not built.")
        print(f"\nSaved configuration not found at: {config_dir}")
        print("Please run the build command first:")
        print(f"  esb build --env={env_name}")
        sys.exit(1)


def _resolve_env_mode(env_name: str) -> str | None:
    """
    Resolve the mode for an environment from generator.yml.
    Returns None if missing or not configured.
    """
    try:
        config_path = cli_config.E2E_DIR / "generator.yml"
        if not config_path.exists():
            return None
        import yaml

        with open(config_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f) or {}
            env_modes = _normalize_env_modes(data.get("environments", []))
            return env_modes.get(env_name)
    except Exception:
        return None


def _normalize_env_modes(raw: object) -> dict[str, str | None]:
    modes: dict[str, str | None] = {}
    if isinstance(raw, dict):
        for name, value in raw.items():
            if not isinstance(name, str):
                continue
            mode = None
            if isinstance(value, dict):
                value_dict = cast(dict[str, Any], value)
                mode = value_dict.get("mode") if isinstance(value_dict.get("mode"), str) else None
            elif isinstance(value, str):
                mode = value
            modes[name] = mode
        return modes
    if isinstance(raw, list):
        for item in raw:
            if isinstance(item, str):
                modes[item] = None
            elif isinstance(item, dict):
                item_dict = cast(dict[str, Any], item)
                name = item_dict.get("name")
                mode = item_dict.get("mode")
                if isinstance(name, str):
                    modes[name] = mode if isinstance(mode, str) else None
        return modes
    return modes
