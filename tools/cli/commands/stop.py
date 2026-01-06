# Where: tools/cli/commands/stop.py
# What: Stop ESB services without removing them.
# Why: Preserve container state/logs/volumes between runs.
import subprocess
import os
from tools.cli.config import PROJECT_ROOT
from tools.cli import compose as cli_compose
from dotenv import load_dotenv
from tools.cli.core import logging

def run(args):
    # Load .env.test.
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        load_dotenv(env_file, override=False)

    from tools.cli import config as cli_config

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
        subprocess.check_call(cmd)
        logging.success("Services stopped.")
    except subprocess.CalledProcessError as e:
        logging.error(f"Failed to stop services: {e}")
