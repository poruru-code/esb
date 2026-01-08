# Where: tools/cli/commands/stop.py
# What: Stop ESB services without removing them.
# Why: Preserve container state/logs/volumes between runs.
import os
import subprocess

from tools.cli import compose as cli_compose
from tools.cli.core import logging, proxy


def run(args):



    logging.step("Stopping services (preserving state)...")
    compose_args = ["stop"]
    
    extra_files = getattr(args, "file", [])
    project_name = os.environ.get("ESB_PROJECT_NAME")

    cmd = cli_compose.build_compose_command(
        compose_args, 
        target="control", 
        extra_files=extra_files,
        project_name=project_name
    )

    try:
        subprocess.check_call(cmd, env=proxy.prepare_env())
        logging.success("Services stopped.")
    except subprocess.CalledProcessError as e:
        logging.error(f"Failed to stop services: {e}")
