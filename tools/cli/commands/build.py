import docker
import os
import sys
import subprocess
from pathlib import Path
from tools.generator import main as generator
from tools.cli import config as cli_config
from tools.cli.core import logging

# Directory for the ESB Lambda base image.
RUNTIME_DIR = cli_config.PROJECT_ROOT / "tools" / "generator" / "runtime"
BASE_IMAGE_TAG = "esb-lambda-base:latest"


def ensure_registry_running():
    """Ensure the registry is running when required."""
    registry = os.getenv("CONTAINER_REGISTRY")
    if not registry:
        return  # Registry not required.

    logging.info(f"Checking if registry ({registry}) is running...")

    try:
        import requests

        # Registry health check.
        response = requests.get(f"http://{registry}/v2/", timeout=2)
        if response.status_code == 200:
            logging.success(f"Registry ({registry}) is already running.")
            return
    except Exception:
        pass  # Registry not running.

    # Start the registry.
    logging.warning(f"Registry ({registry}) is not running. Starting it now...")
    try:
        subprocess.check_call(
            ["docker", "compose", "up", "-d", "registry"],
            cwd=cli_config.PROJECT_ROOT,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        logging.success(f"Registry ({registry}) started successfully.")

        # Wait for startup completion.
        import time

        for _ in range(10):
            try:
                response = requests.get(f"http://{registry}/v2/", timeout=1)
                if response.status_code == 200:
                    return
            except Exception:
                pass
            time.sleep(0.5)

        logging.warning("Registry may not be fully ready yet, but continuing...")
    except subprocess.CalledProcessError as e:
        logging.error(f"Failed to start registry: {e}")
        sys.exit(1)


def build_base_image(no_cache=False):
    """Build the ESB Lambda base image."""
    client = docker.from_env()
    dockerfile_path = RUNTIME_DIR / "Dockerfile.base"

    if not dockerfile_path.exists():
        logging.warning(f"Base Dockerfile not found: {dockerfile_path}")
        return False

    # Get registry prefix from environment (default: localhost:5000).
    registry = os.getenv("CONTAINER_REGISTRY", "localhost:5010")
    image_tag = f"{registry}/{BASE_IMAGE_TAG}"

    logging.step("Building base image...")
    print(f"  • Building {logging.highlight(image_tag)} ...", end="", flush=True)

    try:
        client.images.build(
            path=str(RUNTIME_DIR),
            dockerfile="Dockerfile.base",
            tag=image_tag,
            nocache=no_cache,
            rm=True,
        )
        print(f" {logging.Color.GREEN}✅{logging.Color.END}")
    except Exception as e:
        print(f" {logging.Color.RED}❌{logging.Color.END}")
        logging.error(f"Base image build failed: {e}")
        return False

    # Push to registry
    print(f"  • Pushing {logging.highlight(image_tag)} ...", end="", flush=True)
    try:
        for line in client.images.push(image_tag, stream=True, decode=True):
            if "error" in line:
                raise Exception(line["error"])
        print(f" {logging.Color.GREEN}✅{logging.Color.END}")
        return True
    except Exception as e:
        print(f" {logging.Color.RED}❌{logging.Color.END}")
        error_msg = str(e).lower()
        if "connection refused" in error_msg or "connection" in error_msg:
            logging.error(
                "Registry is not reachable. Is 'esb-registry' running?\n"
                "Run 'docker-compose up -d registry' first."
            )
        else:
            logging.error(f"Push failed: {e}")
        return False


def _extract_function_name_from_dockerfile(dockerfile_path) -> str | None:
    """Extract FunctionName from a Dockerfile."""
    try:
        with open(dockerfile_path, "r", encoding="utf-8") as f:
            for line in f:
                if line.startswith("# FunctionName:"):
                    return line.split(":", 1)[1].strip()
    except Exception:
        pass
    return None


def build_function_images(functions, template_path, no_cache=False, verbose=False):
    """
    Build images for each function.
    """
    client = docker.from_env()

    # Get registry prefix from environment (default: localhost:5000).
    registry = os.getenv("CONTAINER_REGISTRY", "localhost:5010")

    logging.step("Building function images...")

    for func in functions:
        function_name = func["name"]
        dockerfile_path = func.get("dockerfile_path")
        context_path = func.get("context_path")

        if not dockerfile_path or not Path(dockerfile_path).exists():
            logging.warning(f"Dockerfile not found for {function_name} at {dockerfile_path}")
            continue

        image_tag = f"{registry}/{function_name}:latest"

        print(f"  • Building {logging.highlight(image_tag)} ...", end="", flush=True)
        try:
            # Build context is the generated staging directory (context_path).
            # Dockerfile name is fixed as "Dockerfile".
            client.images.build(
                path=str(context_path),
                dockerfile="Dockerfile",
                tag=image_tag,
                nocache=no_cache,
                rm=True,
            )
            print(f" {logging.Color.GREEN}✅{logging.Color.END}")
        except Exception as e:
            print(f" {logging.Color.RED}❌{logging.Color.END}")
            if verbose:
                logging.error(f"Build failed for {image_tag}: {e}")
                raise
            else:
                logging.error(f"Build failed for {image_tag}. Use --verbose for details.")
                import sys

                sys.exit(1)

        # Push to registry
        print(f"  • Pushing {logging.highlight(image_tag)} ...", end="", flush=True)
        try:
            for line in client.images.push(image_tag, stream=True, decode=True):
                if "error" in line:
                    raise Exception(line["error"])
            print(f" {logging.Color.GREEN}✅{logging.Color.END}")
        except Exception as e:
            print(f" {logging.Color.RED}❌{logging.Color.END}")
            error_msg = str(e).lower()
            if "connection refused" in error_msg or "connection" in error_msg:
                logging.error(
                    "Registry is not reachable. Is 'esb-registry' running?\n"
                    "Run 'docker-compose up -d registry' first."
                )
            else:
                logging.error(f"Push failed: {e}")
            import sys

            sys.exit(1)


def run(args):
    dry_run = getattr(args, "dry_run", False)
    verbose = getattr(args, "verbose", False)

    if dry_run:
        logging.info("Running in DRY-RUN mode. No files will be written, no images built.")

    # 0. Ensure registry is running (when required).
    ensure_registry_running()

    # 1. Generate configuration files (Phase 1 Generator).
    logging.step("Generating configurations...")
    logging.info(f"Using template: {logging.highlight(cli_config.TEMPLATE_YAML)}")

    # Load generator config.
    # Prefer generator.yml in the same directory as the template.
    config_path = cli_config.E2E_DIR / "generator.yml"

    if not config_path.exists():
        import questionary
        from tools.cli.commands import init

        print(f"ℹ Configuration file not found at: {config_path}")
        if questionary.confirm("Do you want to initialize configuration now?").ask():
            # Call init (reuse current args, but only pass template).
            init_args = type("Args", (), {"template": str(cli_config.TEMPLATE_YAML)})
            init.run(init_args)
            # After init, we could ask to continue build, but exit for now.
            logging.info("Configuration initialized. Please run build command again.")
            return
        else:
            logging.error("Configuration file missing. Cannot proceed.")
            return

    config = generator.load_config(config_path)

    # Resolve template path.
    if "paths" not in config:
        config["paths"] = {}
    config["paths"]["sam_template"] = str(cli_config.TEMPLATE_YAML)

    functions = generator.generate_files(
        config=config,
        project_root=cli_config.PROJECT_ROOT,
        dry_run=dry_run,
        verbose=verbose,
    )

    if dry_run:
        logging.success("Dry-run complete. Exiting.")
        return

    logging.success("Configurations generated.")

    # 2. Build the base image.
    no_cache = getattr(args, "no_cache", False)

    if not build_base_image(no_cache=no_cache):
        import sys

        sys.exit(1)

    # 3. Build Lambda function images.
    build_function_images(
        functions=functions,
        template_path=cli_config.TEMPLATE_YAML,
        no_cache=no_cache,
        verbose=verbose,
    )

    logging.success("Build complete.")
