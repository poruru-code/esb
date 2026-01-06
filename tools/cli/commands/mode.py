# Where: tools/cli/commands/mode.py
# What: CLI command to get/set the ESB runtime mode.
# Why: Persist mode selection and drive compose behavior.
import sys

from tools.cli import config as cli_config
from tools.cli import runtime_mode
from tools.cli.core import logging


def _run_get() -> None:
    current = runtime_mode.get_mode()
    logging.info(f"Current mode: {logging.highlight(current)}")
    logging.info(f"Config: {logging.highlight(str(cli_config.get_mode_config_path()))}")


def _run_set(args) -> None:
    try:
        data = runtime_mode.save_mode(args.mode)
    except ValueError as exc:
        logging.error(str(exc))
        sys.exit(1)
    logging.success(f"Mode set to {logging.highlight(data['mode'])}")
    logging.info(f"Config: {logging.highlight(str(cli_config.get_mode_config_path()))}")


def run(args) -> None:
    if args.mode_command == "get":
        _run_get()
        return
    if args.mode_command == "set":
        _run_set(args)
        return

    logging.error(f"Unsupported mode command: {args.mode_command}")
    sys.exit(1)
