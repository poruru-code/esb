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

    # Calculate isolation variables
    env_name = cli_config.get_env_name()
    project_name = f"esb-{env_name}".lower()
    os.environ["ESB_PROJECT_NAME"] = project_name
    
    # Update os.environ with isolation vars so docker-compose picks them up
    os.environ.update(cli_config.get_port_mapping(env_name))
    os.environ.update(cli_config.get_subnet_config(env_name))

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
