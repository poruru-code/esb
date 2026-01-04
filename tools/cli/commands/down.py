# Where: tools/cli/commands/down.py
# What: Stop ESB services and clean up resources.
# Why: Provide a consistent shutdown path for the CLI.
import subprocess
from tools.cli.config import PROJECT_ROOT
from tools.cli import compose as cli_compose
from dotenv import load_dotenv
from tools.cli.core import logging
from importlib.metadata import metadata


def run(args):
    # Load .env.test.
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        load_dotenv(env_file, override=False)

    from tools.cli import config as cli_config
    import os

    logging.step("Stopping services...")
    compose_args = ["down", "--remove-orphans"]
    if getattr(args, "volumes", False):
        compose_args.append("--volumes")
    if getattr(args, "rmi", False):
        compose_args.extend(["--rmi", "all"])
    
    extra_files = getattr(args, "file", [])

    # Calculate isolation variables
    env_name = cli_config.get_env_name()
    project_name = f"esb-{env_name}".lower()
    os.environ["ESB_PROJECT_NAME"] = project_name
    
    # Update os.environ with isolation vars so docker-compose picks them up
    # This is crucial so that variable substitution in docker-compose.yml works during 'down'
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
        # Attempt to clean up Lambda containers even if compose down fails.

    # Clean up Lambda containers (created_by=edge-serverless-box).
    import docker

    try:
        client = docker.from_env()
        project_name = metadata("edge-serverless-box")["Name"]
        lambda_containers = client.containers.list(
            all=True, filters={"label": f"created_by={project_name}"}
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
