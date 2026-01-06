# Where: tools/cli/commands/up.py
# What: Start ESB services via docker compose.
# Why: Boot the local stack with consistent CLI behavior.
import os
import sys
import yaml
import subprocess
from pathlib import Path

# from . import build
from tools.provisioner import main as provisioner
from tools.cli import config as cli_config
from tools.cli import compose as cli_compose
from tools.cli import runtime_mode
from tools.cli.config import PROJECT_ROOT
from dotenv import load_dotenv


from tools.cli.core import logging
from tools.cli.core.cert import ensure_certs
from tools.cli.core import proxy
import time
import requests


def wait_for_gateway(timeout=60):
    """Wait until Gateway responds."""
    start_time = time.time()
    start_time = time.time()

    # Dynamically resolve Gateway port based on active environment (already in os.environ)
    port = os.environ.get("ESB_PORT_GATEWAY_HTTPS", "443")
    url = f"https://localhost:{port}/health"

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


def _should_override_agent_address(current: str | None) -> bool:
    if not current:
        return True
    normalized = current.strip().lower()
    return normalized in {
        "localhost:50051",
        "127.0.0.1:50051",
        "::1:50051",
        "agent:50051",
        "runtime-node:50051",
    }


def _resolve_firecracker_agent_address() -> str | None:
    nodes_path = cli_config.ESB_HOME / "nodes.yaml"
    if not nodes_path.exists():
        return None
    try:
        data = yaml.safe_load(nodes_path.read_text())
    except Exception as exc:
        logging.warning(f"Failed to read nodes config for Agent address: {exc}")
        return None
    if not isinstance(data, dict):
        return None
    nodes = data.get("nodes")
    if not isinstance(nodes, list):
        return None
    for node in nodes:
        if not isinstance(node, dict):
            continue
        wg_addr = node.get("wg_compute_addr") or ""
        if not wg_addr:
            continue
        host = wg_addr.split("/")[0]
        port = os.environ.get("AGENT_GRPC_PORT") or os.environ.get("PORT")
        if not port:
            port = str(cli_config.DEFAULT_AGENT_GRPC_PORT)
        return f"{host}:{port}"
    return None


def run(args):
    # 0. Prepare SSL certificates.
    cert_dir = Path(os.environ.get("ESB_CERT_DIR", str(cli_config.DEFAULT_CERT_DIR))).expanduser()
    ensure_certs(cert_dir)
    proxy.apply_proxy_env()

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

    if runtime_mode.get_mode() == cli_config.ESB_MODE_FIRECRACKER:
        if _should_override_agent_address(os.environ.get("AGENT_GRPC_ADDRESS")):
            resolved = _resolve_firecracker_agent_address()
            if resolved:
                os.environ["AGENT_GRPC_ADDRESS"] = resolved
                logging.info(
                    f"Firecracker mode: using AGENT_GRPC_ADDRESS={logging.highlight(resolved)}"
                )
            else:
                logging.warning(
                    "Firecracker mode requires AGENT_GRPC_ADDRESS. "
                    "Set it manually or run `esb node add` to populate nodes.yaml."
                )

    # 2. Start services.
    logging.step(
        f"Starting services for environment: {logging.highlight(cli_config.get_env_name())}..."
    )
    compose_args = ["up"]
    if getattr(args, "detach", True):
        compose_args.append("-d")

    # Rebuild services themselves.
    if getattr(args, "build", False):
        compose_args.append("--build")

    extra_files = getattr(args, "file", [])
    project_name = os.environ.get("ESB_PROJECT_NAME")

    cmd = cli_compose.build_compose_command(
        compose_args, target="control", extra_files=extra_files, project_name=project_name
    )

    try:
        # Pass updated env to subprocess implicitly (os.environ is updated)
        subprocess.check_call(cmd, env=proxy.prepare_env())
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
