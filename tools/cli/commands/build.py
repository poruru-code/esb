import docker
import os
import sys
import subprocess
import shutil
from pathlib import Path
from tools.generator import main as generator
from tools.cli import config as cli_config
from tools.cli import compose as cli_compose
from tools.cli import runtime_mode
from tools.cli import build_service_images
from tools.cli.core import logging
from tools.cli.core import proxy

# Directory for the ESB Lambda base image.
RUNTIME_DIR = cli_config.PROJECT_ROOT / "tools" / "generator" / "runtime"


def get_base_image_tag(tag: str | None = None) -> str:
    """Evaluate base image tag dynamically."""
    if tag is None:
        tag = cli_config.get_image_tag()
    return f"esb-lambda-base:{tag}"


def ensure_registry_running(registry=None, extra_files=None, project_name=None):
    """Ensure the registry is running when required."""
    if not registry:
        registry = os.getenv("CONTAINER_REGISTRY")
    if not registry:
        return  # Registry not required.

    logging.info(f"Checking if registry ({registry}) is running...")

    try:
        import requests
        from urllib3.exceptions import InsecureRequestWarning

        # Suppress insecure request warnings for local registry checks
        requests.packages.urllib3.disable_warnings(category=InsecureRequestWarning)

        # Try HTTPS first, then fallback to HTTP for health check.
        urls = [f"https://{registry}/v2/", f"http://{registry}/v2/"]
        for url in urls:
            try:
                response = requests.get(url, timeout=2, verify=False)
                if response.status_code == 200:
                    logging.success(
                        f"Registry ({registry}) is already running (via {url.split(':')[0]})."
                    )
                    return
            except Exception:
                continue
    except Exception:
        pass  # Registry not running.

    # Start the registry.
    logging.warning(f"Registry ({registry}) is not running. Starting it now...")
    try:
        subprocess.check_call(
            cli_compose.build_compose_command(
                ["up", "-d", "registry"],
                target="control",
                extra_files=extra_files,
                project_name=project_name,
            ),
            cwd=cli_config.PROJECT_ROOT,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        logging.success(f"Registry ({registry}) started successfully.")

        # Wait for startup completion.
        import time

        for _ in range(10):
            try:
                # Try HTTPS first, then fallback to HTTP for health check.
                urls = [f"https://{registry}/v2/", f"http://{registry}/v2/"]
                for url in urls:
                    try:
                        response = requests.get(url, timeout=1, verify=False)
                        if response.status_code == 200:
                            return
                    except Exception:
                        continue
            except Exception:
                pass
            time.sleep(0.5)

        logging.warning("Registry may not be fully ready yet, but continuing...")
    except subprocess.CalledProcessError as e:
        logging.error(f"Failed to start registry: {e}")
        sys.exit(1)


def build_base_image(no_cache=False, push_registry=None):
    """Build the ESB Lambda base image."""
    client = docker.from_env()
    dockerfile_path = RUNTIME_DIR / "Dockerfile.base"

    if not dockerfile_path.exists():
        logging.warning(f"Base Dockerfile not found: {dockerfile_path}")
        return False

    # Use push_registry if provided, otherwise fallback to CONTAINER_REGISTRY
    registry = push_registry or os.getenv("CONTAINER_REGISTRY")
    base_tag = get_base_image_tag(cli_config.get_image_tag())
    if registry:
        image_tag = f"{registry}/{base_tag}"
    else:
        image_tag = base_tag  # Local tag only

    logging.step("Building base image...")
    print(f"  • Building {logging.highlight(image_tag)} ...", end="", flush=True)

    try:
        client.images.build(
            path=str(RUNTIME_DIR),
            dockerfile="Dockerfile.base",
            tag=image_tag,
            nocache=no_cache,
            rm=True,
            buildargs=proxy.docker_build_args(),
        )
        print(f" {logging.Color.GREEN}✅{logging.Color.END}")
    except Exception as e:
        print(f" {logging.Color.RED}❌{logging.Color.END}")
        logging.error(f"Base image build failed: {e}")
        return False

    # Push to registry
    if not push_registry and not os.getenv("CONTAINER_REGISTRY"):
        return True  # Skip push

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


def build_function_images(
    functions, template_path, no_cache=False, verbose=False, push_registry=None
):
    """
    Build images for each function.
    """
    client = docker.from_env()

    # Use push_registry if provided, otherwise fallback to CONTAINER_REGISTRY
    registry = push_registry or os.getenv("CONTAINER_REGISTRY")

    logging.step("Building function images...")

    for func in functions:
        function_name = func["name"]
        dockerfile_path = func.get("dockerfile_path")
        context_path = func.get("context_path")

        if not dockerfile_path:
            logging.warning(f"No Dockerfile path defined for {function_name}")
            continue

        dockerfile_full_path = Path(dockerfile_path)
        if not dockerfile_full_path.exists():
            logging.warning(
                f"Dockerfile not found for {function_name} at {dockerfile_full_path.absolute()}"
            )
            continue

        if verbose:
            logging.info(f"Using Dockerfile: {dockerfile_full_path.absolute()}")

        # Use push_registry if provided, otherwise fallback to local tagging
        image_tag_only = cli_config.get_image_tag()
        if registry:
            image_tag = f"{registry}/{function_name}:{image_tag_only}"
        else:
            image_tag = f"{function_name}:{image_tag_only}"

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
                buildargs=proxy.docker_build_args(),
            )
            print(f" {logging.Color.GREEN}✅{logging.Color.END}")
        except Exception as e:
            print(f" {logging.Color.RED}❌{logging.Color.END}")
            if verbose:
                logging.error(f"Build failed for {image_tag}: {e}")
                raise
            else:
                logging.error(f"Build failed for {image_tag}. Use --verbose for details.")
                sys.exit(1)

        # Push to registry
        if not push_registry and not os.getenv("CONTAINER_REGISTRY"):
            continue  # Skip push

        registry = push_registry or os.getenv("CONTAINER_REGISTRY")
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

            sys.exit(1)


from tools.cli.core import context

def run(args):
    dry_run = getattr(args, "dry_run", False)
    verbose = getattr(args, "verbose", False)

    # Normalize proxy variables early for compose/docker calls.
    proxy.apply_proxy_env()

    if dry_run:
        logging.info("Running in DRY-RUN mode. No files will be written, no images built.")

    context.enforce_env_arg(args, require_initialized=True)
    env_name = cli_config.get_env_name()
    project_name = os.environ.get("ESB_PROJECT_NAME")

    registry_config = cli_config.get_registry_config(env_name)
    external_registry = registry_config["external"]
    internal_registry = registry_config["internal"]

    # 1. Generate configuration files (Phase 1 Generator).
    logging.step("Generating configurations...")
    logging.info(f"Using template: {logging.highlight(str(cli_config.TEMPLATE_YAML))}")

    # Load generator config.
    config = generator.load_config(cli_config.E2E_DIR / "generator.yml")

    # 0. Ensure registry is running (when required).
    if not dry_run:
        extra_files = getattr(args, "file", [])
        ensure_registry_running(
            registry=external_registry, extra_files=extra_files, project_name=project_name
        )

    # Resolve template path.
    if "paths" not in config:
        config["paths"] = {}
    config["paths"]["sam_template"] = str(cli_config.TEMPLATE_YAML)

    # Set output directory based on environment name (honoring generator.yml)
    # This allows parallel builds for different environments
    output_dir_path = cli_config.get_build_output_dir(env_name)
    output_dir = str(output_dir_path)
    config["paths"]["output_dir"] = output_dir

    # --- Configuration Staging ---
    # We ALWAYS stage configuration files into services/gateway/.esb-staging/{env}/config
    # to maintain a consistent build context and enable image baking regardless of template location.
    # Using env-specific directory to support parallel builds.
    config_dir_abs = output_dir_path / "config"
    gateway_staging_dir = cli_config.PROJECT_ROOT / "services" / "gateway" / ".esb-staging" / env_name / "config"
    staging_relative_path = f"services/gateway/.esb-staging/{env_name}/config"
    
    logging.info(f"Staging configuration to {logging.highlight(staging_relative_path)} ...")
    
    try:
        # Clean and recreate staging directory
        if gateway_staging_dir.exists():
            shutil.rmtree(gateway_staging_dir)
        gateway_staging_dir.mkdir(parents=True, exist_ok=True)
        
        # Copy configuration files
        for cfg_file in ["functions.yml", "routing.yml"]:
            src = config_dir_abs / cfg_file
            if src.exists():
                shutil.copy2(src, gateway_staging_dir / cfg_file)
        
        # Set ESB_CONFIG_DIR to the staged path relative to project root
        os.environ["ESB_CONFIG_DIR"] = staging_relative_path
        logging.success("Staging complete. Configuration will be baked into the image.")
        
    except Exception as e:
        logging.error(f"Failed to stage configuration: {e}")
        logging.warning("Falling back to runtime (empty) configuration.")
        os.environ["ESB_CONFIG_DIR"] = ""

    # Get dynamic parameters based on current mode (e.g., endpoint hosts)
    parameters = cli_config.get_generator_parameters(env_name)

    functions = generator.generate_files(
        config=config,
        project_root=cli_config.PROJECT_ROOT,
        dry_run=dry_run,
        verbose=verbose,
        registry_external=external_registry,
        registry_internal=internal_registry,
        parameters=parameters,
        tag=cli_config.get_image_tag(env_name),
    )

    if dry_run:
        logging.success("Dry-run complete. Exiting.")
        return

    logging.success("Configurations generated.")

    # 2. Build the base image.
    no_cache = getattr(args, "no_cache", False)

    if not build_base_image(no_cache=no_cache, push_registry=external_registry):
        sys.exit(1)

    # 3. Build Lambda function images.
    build_function_images(
        functions=functions,
        template_path=cli_config.TEMPLATE_YAML,
        no_cache=no_cache,
        verbose=verbose,
        push_registry=external_registry,
    )

    if runtime_mode.get_mode() == cli_config.ESB_MODE_FIRECRACKER:
        if not build_service_images.build_and_push(
            no_cache=no_cache, push_registry=external_registry
        ):
            sys.exit(1)

    logging.success("Build complete.")
