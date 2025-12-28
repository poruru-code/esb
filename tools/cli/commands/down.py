import subprocess
from tools.cli.config import PROJECT_ROOT
from dotenv import load_dotenv
from tools.cli.core import logging
from importlib.metadata import metadata


def run(args):
    # Load .env.test.
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        load_dotenv(env_file, override=False)

    logging.step("Stopping services...")
    cmd = ["docker", "compose", "down", "--remove-orphans"]
    if getattr(args, "volumes", False):
        cmd.append("--volumes")
    if getattr(args, "rmi", False):
        cmd.extend(["--rmi", "all"])

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
