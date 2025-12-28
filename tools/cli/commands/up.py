import os
import sys
import yaml
import subprocess
from . import build
from tools.provisioner import main as provisioner
from tools.cli import config as cli_config
from tools.cli.config import PROJECT_ROOT
from dotenv import load_dotenv


from tools.cli.core import logging
from tools.cli.core.cert import ensure_certs
import time
import requests


def wait_for_gateway(timeout=60):
    """Wait until Gateway responds."""
    start_time = time.time()
    # Ideally this should be retrieved dynamically from the CLI, but in tests we
    # assume localhost:443 (Gateway). Use config.py or a default value.
    url = "https://localhost/health"

    logging.step("Waiting for Gateway...")
    while time.time() - start_time < timeout:
        try:
            # verify=False allows a self-signed certificate.
            if requests.get(url, verify=False, timeout=1).status_code == 200:
                logging.success("Gateway is ready!")
                return True
        except Exception:
            time.sleep(1)
            # We could print dots for progress, but keep it simple here.

    logging.error("Gateway failed to start.")
    return False


def run(args):
    # 0. Prepare SSL certificates.
    ensure_certs(PROJECT_ROOT / "certs")

    # Load .env.test (same as run_tests.py).
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        logging.info(f"Loading environment variables from {logging.highlight(env_file)}")
        load_dotenv(env_file, override=False)

    # 1. Apply custom settings (set env vars if generator.yml exists).
    config_path = cli_config.E2E_DIR / "generator.yml"
    if config_path.exists():
        try:
            with open(config_path, "r") as f:
                config = yaml.safe_load(f)

            paths = config.get("paths", {})
            if "functions_yml" in paths:
                os.environ["GATEWAY_FUNCTIONS_YML"] = str(paths["functions_yml"])
            if "routing_yml" in paths:
                os.environ["GATEWAY_ROUTING_YML"] = str(paths["routing_yml"])
        except Exception as e:
            logging.warning(f"Failed to load generator.yml for environment injection: {e}")

    # 2. Run build if requested.
    if getattr(args, "build", False):
        build.run(args)

    # 2. Start services.
    logging.step("Starting services...")
    cmd = ["docker", "compose", "up"]
    if getattr(args, "detach", True):
        cmd.append("-d")

    # Rebuild services themselves.
    if getattr(args, "build", False):
        cmd.append("--build")

    try:
        subprocess.check_call(cmd)
    except subprocess.CalledProcessError as e:
        logging.error(f"Failed to start services: {e}")
        sys.exit(1)

    # 3. Infrastructure provisioning.
    logging.step("Preparing infrastructure...")
    from tools.cli.config import TEMPLATE_YAML

    provisioner.main(template_path=TEMPLATE_YAML)

    logging.success("Environment is ready! (https://localhost:443)")

    # 4. Wait logic (optional).
    if getattr(args, "wait", False):
        if not wait_for_gateway():
            sys.exit(1)
