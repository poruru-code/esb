import logging
import os
import subprocess
import time
from http.client import HTTPConnection
from typing import Callable, Tuple

from e2e.runner import constants
from e2e.runner.utils import BRAND_SLUG

logger = logging.getLogger(__name__)

INFRA_COMPOSE_FILE = "docker-compose.infra.yml"


def _registry_container_name() -> str:
    return os.environ.get("REGISTRY_CONTAINER_NAME", f"{BRAND_SLUG}-infra-registry")


def ensure_infra_up(project_root: str, *, printer: Callable[[str], None] | None = None) -> None:
    """Ensure shared infrastructure (Registry) is up and running."""
    compose_file = os.path.join(project_root, INFRA_COMPOSE_FILE)
    if not os.path.exists(compose_file):
        logger.warning(
            f"Infra compose file not found at {compose_file}, skipping shared infra setup."
        )
        return

    logger.info("Ensuring shared infrastructure (Registry) is ready...")
    try:
        # Check if registry is already running to save time
        subprocess.check_call(
            [
                "docker",
                "compose",
                "-f",
                compose_file,
                "ps",
                "--services",
                "--filter",
                "status=running",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        # If successfully checked and output not empty (implied logic needs refinement, but 'up -d' is idempotent)
    except subprocess.CalledProcessError:
        pass  # Not running or error

    # Idempotent start
    if printer:
        printer("Ensuring shared infrastructure (Registry) is ready...")
    else:
        logger.info("Ensuring shared infrastructure (Registry) is ready...")
    env = os.environ.copy()
    env.setdefault("REGISTRY_CONTAINER_NAME", _registry_container_name())
    subprocess.check_call(
        ["docker", "compose", "-f", compose_file, "up", "-d", "registry"],
        env=env,
    )
    host_addr, _ = get_registry_config()
    wait_for_registry_ready(host_addr)


def get_registry_config() -> Tuple[str, str]:
    """
    Returns (host_registry_addr, service_registry_addr).
    host_registry_addr: For host-side registry checks (e.g. localhost:5010)
    service_registry_addr: For in-network pulls/pushes (e.g. registry:5010)
    """
    port = os.environ.get("PORT_REGISTRY", constants.DEFAULT_REGISTRY_PORT)

    # Host side: localhost is fine
    host_addr = f"127.0.0.1:{port}"
    service_addr = constants.DEFAULT_AGENT_REGISTRY

    return host_addr, service_addr


def connect_registry_to_network(network_name: str, alias: str = "registry") -> None:
    if not network_name:
        return
    container_name = _registry_container_name()
    try:
        subprocess.check_call(
            [
                "docker",
                "network",
                "connect",
                "--alias",
                alias,
                network_name,
                container_name,
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    except subprocess.CalledProcessError:
        # Already connected or network not found; ignore.
        return


def wait_for_registry_ready(host_addr: str, timeout: int = 60) -> None:
    url = f"http://{host_addr}/v2/"
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if _registry_v2_ready(host_addr, timeout=2):
            return
        time.sleep(1)
    raise RuntimeError(f"Registry not responding at {url}")


def _registry_v2_ready(host_addr: str, timeout: int = 2) -> bool:
    # This check targets the local dev registry; bypass proxy resolution entirely.
    conn = HTTPConnection(host_addr, timeout=timeout)
    try:
        conn.request("GET", "/v2/")
        response = conn.getresponse()
        response.read()
        return response.status == 200
    except OSError:
        return False
    finally:
        conn.close()
