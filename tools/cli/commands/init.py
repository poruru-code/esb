import os
import shutil
import sys
from pathlib import Path
from typing import Any, cast

import questionary
import yaml

from tools.cli import config as cli_config
from tools.cli.core import logging
from tools.cli.core.cert import ensure_certs
from tools.cli.core.trust_store import install_root_ca


def run(args):
    """
    Run the interactive wizard and generate/update generator.yml.
    If --env is specified, run in silent mode (non-interactive).
    """
    # Check for silent mode (--env specified)
    if getattr(args, "env", None):
        return run_silent(args)

    print("🚀 Initializing Edge Serverless Box configuration...")

    # 1. Find the template file.
    # Priority: 1) main parser --template (cli_config.TEMPLATE_YAML)
    #           2) subparser --template (args.template)
    #           3) current directory search
    template_path = None

    # Use cli_config.TEMPLATE_YAML if set (via main parser).
    if cli_config.TEMPLATE_YAML and cli_config.TEMPLATE_YAML.exists():
        template_path = cli_config.TEMPLATE_YAML.resolve()
    elif args.template:
        template_path = Path(args.template).expanduser().resolve()
    else:
        # Default search order.
        candidates = [
            Path("template.yaml"),
            Path("template.yml"),
        ]
        for c in candidates:
            if c and c.exists():
                template_path = c.resolve()
                break

    if not template_path or not template_path.exists():
        # Prompt for input if not found.
        path_input = questionary.path("Path to SAM template.yaml:").ask()
        if not path_input:
            print("❌ No template provided. Aborting.")
            sys.exit(1)
        template_path = Path(path_input).resolve()

    print(f"ℹ Using template: {template_path}")
    sys.stdout.flush()

    # 2. Check if generator.yml already exists (early decision point)
    save_path = template_path.parent / "generator.yml"
    existing_config = {}
    is_overwrite = True

    if save_path.exists():
        choice = questionary.select(
            f"File {save_path} already exists. What would you like to do?",
            choices=[
                {"name": "Add/Update current environment only", "value": "add"},
                {"name": "Overwrite everything (Start fresh)", "value": "overwrite"},
                {"name": "Cancel", "value": "cancel"},
            ],
        ).ask()

        if choice == "cancel" or choice is None:
            print("Aborted.")
            sys.exit(0)

        if choice == "add":
            is_overwrite = False
            try:
                with open(save_path, "r", encoding="utf-8") as f:
                    existing_config = yaml.safe_load(f) or {}
            except Exception as e:
                logging.warning(f"Failed to load existing config: {e}. Falling back to overwrite.")
                is_overwrite = True

    # 3. Load the template and extract parameters.
    from tools.generator.parser import CfnLoader

    try:
        with open(template_path, "r", encoding="utf-8") as f:
            template_data = yaml.load(f, Loader=CfnLoader)
    except Exception as e:
        print(f"❌ Failed to load template: {e}")
        sys.exit(1)

    parameters = template_data.get("Parameters", {})
    param_values = {}

    if parameters:
        print("\n📝 Configure Parameters:")
        sys.stdout.flush()
        for key, value in parameters.items():
            default_val = value.get("Default", "")
            description = value.get("Description", "")
            prompt_text = f"Value for '{key}'"
            if description:
                prompt_text += f" ({description})"

            user_val = questionary.text(prompt_text, default=str(default_val)).ask()
            if user_val is None:
                print("❌ Input cancelled. Aborting.")
                sys.exit(1)
            param_values[key] = user_val

    # 3. Additional settings.
    print("\n⚙ Additional Configuration:")
    sys.stdout.flush()

    # Environment Name (also used as Docker Image Tag)
    env_name = questionary.text("Environment Name:", default="default").ask()
    if env_name is None:
        print("❌ Input cancelled. Aborting.")
        sys.exit(1)

    # Output Directory
    # Default is .esb under the template directory.
    default_output_dir = template_path.parent / ".esb"
    output_dir_input = questionary.path(
        "Output Directory for artifacts:", default=str(default_output_dir)
    ).ask()
    if output_dir_input is None:
        print("❌ Input cancelled. Aborting.")
        sys.exit(1)
    output_dir = Path(output_dir_input).resolve()

    # 5. Generate/Update generator.yml.
    # Convert paths to be relative to the template for portability.
    base_dir = template_path.parent

    def to_rel(p: Path) -> str:
        try:
            return os.path.relpath(p, base_dir)
        except ValueError:
            return str(p)

    # Build the list of initialized environments (with mode)
    current_mode = _default_mode()
    if is_overwrite:
        environments = {env_name: current_mode}
    else:
        environments = _normalize_env_modes(existing_config.get("environments", []))
        if env_name not in environments:
            environments[env_name] = current_mode

    generator_config = {
        "app": {
            "name": existing_config.get("app", {}).get("name", ""),
            "tag": env_name,
        },
        "environments": environments,
        "paths": {"sam_template": to_rel(template_path), "output_dir": to_rel(output_dir) + "/"},
    }

    if param_values:
        generator_config["parameters"] = param_values

    try:
        with open(save_path, "w", encoding="utf-8") as f:
            yaml.dump(
                generator_config, f, default_flow_style=False, sort_keys=False, allow_unicode=True
            )
        logging.success(f"Configuration saved to: {save_path}")

        # 5. Consistency Cleanup: Delete environment directories not in the list
        if output_dir.exists():
            rel_path = logging.highlight(to_rel(output_dir))
            logging.info(f"Cleaning up unused environment directories in {rel_path} ...")
            for item in output_dir.iterdir():
                if item.is_dir() and item.name not in environments.keys():
                    logging.info(f"  🗑  Removing orphaned environment: {item.name}")
                    shutil.rmtree(item)

        print("\nYou can now run 'esb build' to generate Dockerfiles.")
    except Exception as e:
        logging.error(f"Failed to save config or cleanup: {e}")
        sys.exit(1)

    # 5. Prepare SSL certificates and install OS trust.
    print("\n🔐 Preparing SSL certificates and trust store...")
    try:
        from tools.cli.config import get_cert_dir

        ensure_certs(get_cert_dir())
        install_root_ca(get_cert_dir() / "rootCA.crt")
        print("✅ Root CA installed successfully.")
    except Exception as e:
        print(f"⚠️ Failed to prepare certificates/trust store: {e}")
        print(
            "You may need to run this command with administrative privileges "
            "or manually install the CA cert."
        )


def run_silent(args):
    """
    Run silent initialization for E2E testing.
    Creates generator.yml with the specified environments using defaults.
    Always overwrites (creates new file).

    Args:
        args: Must have 'env' attribute with comma-separated environment names.
    """
    # Parse environments from comma-separated string
    env_str = args.env
    environments = _parse_env_entries(env_str)

    if not environments:
        logging.error("No environments specified in --env argument.")
        sys.exit(1)

    print(f"🔧 Silent initialization for environments: {', '.join(environments.keys())}")

    # 1. Find the template file (same logic as interactive mode)
    template_path = None

    if cli_config.TEMPLATE_YAML and cli_config.TEMPLATE_YAML.exists():
        template_path = cli_config.TEMPLATE_YAML.resolve()
    elif getattr(args, "template", None):
        template_path = Path(args.template).expanduser().resolve()
    else:
        # Default search order
        candidates = [Path("template.yaml"), Path("template.yml")]
        for c in candidates:
            if c.exists():
                template_path = c.resolve()
                break

    if not template_path or not template_path.exists():
        logging.error(
            "No template found. Specify --template or place template.yaml in current directory."
        )
        sys.exit(1)

    print(f"ℹ Using template: {template_path}")

    # 2. Set up paths with defaults
    assert template_path is not None  # Type narrowing for ty
    base_dir = template_path.parent
    save_path = base_dir / "generator.yml"
    default_output_dir = base_dir / ".esb"

    def to_rel(p: Path) -> str:
        try:
            return os.path.relpath(p, base_dir)
        except ValueError:
            return str(p)

    # 3. Create generator.yml with defaults (always overwrite)
    # Use first environment as the default tag
    generator_config = {
        "app": {
            "name": "",
            "tag": next(iter(environments.keys())),
        },
        "environments": environments,
        "paths": {
            "sam_template": to_rel(template_path),
            "output_dir": to_rel(default_output_dir) + "/",
        },
    }

    try:
        with open(save_path, "w", encoding="utf-8") as f:
            yaml.dump(
                generator_config, f, default_flow_style=False, sort_keys=False, allow_unicode=True
            )
        logging.success(f"Configuration saved to: {save_path}")
    except Exception as e:
        logging.error(f"Failed to save config: {e}")
        sys.exit(1)

    # 4. Skip SSL certificates for silent mode (to avoid sudo prompt)
    print("\n🔐 Skipping SSL certificates setup in silent mode.")


def _default_mode() -> str:
    mode = os.environ.get("ESB_MODE", "").strip().lower()
    if not mode:
        return "docker"
    return mode


def _parse_env_entries(env_str: str) -> dict[str, str]:
    mode_default = _default_mode()
    entries: dict[str, str] = {}
    for raw in env_str.split(","):
        raw = raw.strip()
        if not raw:
            continue
        if ":" in raw:
            name, mode = raw.split(":", 1)
            name = name.strip()
            mode = mode.strip().lower() or mode_default
        else:
            name = raw
            mode = mode_default
        if not name:
            continue
        entries[name] = mode
    return entries


def _normalize_env_modes(raw: object) -> dict[str, str]:
    modes: dict[str, str] = {}
    if isinstance(raw, dict):
        for name, value in raw.items():
            if not isinstance(name, str):
                continue
            if isinstance(value, dict):
                value_dict = cast(dict[str, Any], value)
                mode = value_dict.get("mode")
                if isinstance(mode, str):
                    modes[name] = mode
                else:
                    modes[name] = _default_mode()
            elif isinstance(value, str):
                modes[name] = value
            else:
                modes[name] = _default_mode()
        return modes
    if isinstance(raw, list):
        for item in raw:
            if isinstance(item, str):
                modes[item] = _default_mode()
            elif isinstance(item, dict):
                item_dict = cast(dict[str, Any], item)
                name = item_dict.get("name")
                mode = item_dict.get("mode")
                if isinstance(name, str):
                    if isinstance(mode, str):
                        modes[name] = mode
                    else:
                        modes[name] = _default_mode()
        return modes
    return modes
