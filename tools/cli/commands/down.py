# Where: tools/cli/commands/down.py
# What: Stop ESB services and clean up resources.
# Why: Provide a consistent shutdown path for the CLI.
import subprocess
from tools.cli.config import PROJECT_ROOT
from tools.cli import compose as cli_compose

from tools.cli.core import logging
from importlib.metadata import metadata
from tools.cli.core import proxy


def run(args):


    from tools.cli import config as cli_config
    import os

    logging.step("Stopping services...")
    compose_args = ["down", "--remove-orphans"]
    if getattr(args, "volumes", False):
        compose_args.append("--volumes")
    if getattr(args, "rmi", False):
        compose_args.extend(["--rmi", "all"])
    
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
        # Attempt to clean up Lambda containers even if compose down fails.

    # Clean up Lambda containers (created_by=edge-serverless-box).
    import docker

    try:
        client = docker.from_env()
        lambda_containers = client.containers.list(
            all=True, filters={"label": "created_by=esb"}
        )
        if lambda_containers:
            logging.step(f"Cleaning up {len(lambda_containers)} Lambda containers...")
            for container in lambda_containers:
                try:
                    if container.status == "running":
                        container.kill()
                    container.remove(force=True)
                except Exception as e:
                    logging.warning(f"Failed to remove container {container.name}: {e}")
            logging.success("Lambda containers cleaned up.")
    except Exception as e:
        logging.warning(f"Failed to cleanup Lambda containers: {e}")
