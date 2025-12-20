import docker
import docker.errors
import time
import logging
import os
import socket
from typing import Dict, Optional

logger = logging.getLogger("manager.service")


class ContainerManager:
    """
    Manages lifecycle of Lambda containers.
    """

    def __init__(self, network: Optional[str] = None):
        self.client = docker.from_env()
        self.last_accessed: Dict[str, float] = {}

        # Use env var or default to 'bridge' if not specified.
        # In this architecture, manager is in the same docker network as gateway usually.
        # But Lambdas might be in a separate network or the same one.
        # We will allow injection via env var `CONTAINERS_NETWORK`
        self.network = network or os.environ.get("CONTAINERS_NETWORK") or "bridge"
        logger.info(f"ContainerManager initialized with network: {self.network}")

    def ensure_container_running(
        self, name: str, image: Optional[str] = None, env: Optional[Dict[str, str]] = None
    ) -> str:
        """
        Ensures the container is running. Returns the hostname (container name).
        """
        self.last_accessed[name] = time.time()

        if image is None:
            image = f"{name}:latest"

        try:
            container = self.client.containers.get(name)

            if container.status == "running":
                pass  # Already running

            elif container.status == "exited":
                logger.info(f"Warm-up: Restarting container {name}...")
                container.start()
                self._wait_for_readiness(name)

            else:
                logger.info(f"Container {name} in state {container.status}, removing...")
                container.remove(force=True)
                raise docker.errors.NotFound(f"Removed {name}")

        except docker.errors.NotFound:
            logger.info(f"Cold Start: Creating and starting container {name}...")

            # The manager needs to run sibling containers
            # We assume the user configures CONTAINERS_NETWORK correctly (e.g., sample-dind-lambda_default)
            container = self.client.containers.run(
                image,
                name=name,
                detach=True,
                environment=env or {},
                network=self.network,
                restart_policy={"Name": "no"},
                labels={"created_by": "sample-dind"},  # Mark for cleanup
                # Important: Lambdas should not be privileged usually, but depends on use case.
                # The test expects privileged=False (default).
            )
            self._wait_for_readiness(name)

        # Reload container to get latest attributes (IP)
        try:
            container.reload()
            ip = container.attrs["NetworkSettings"]["Networks"][self.network]["IPAddress"]
            return ip
        except KeyError:
            # Fallback if specific network key is missing or IP is empty, return name (hostname)
            logger.warning(
                f"Could not get IP for {name} on network {self.network}. Returning hostname."
            )
            return name

    def _wait_for_readiness(self, host: str, port: int = 8080, timeout: int = 30) -> None:
        start = time.time()
        while time.time() - start < timeout:
            try:
                # We need to resolve IP if we are inside a container and 'host' is a container name
                # This works if we share the docker network or have DNS resolution
                with socket.create_connection((host, port), timeout=1):
                    return
            except (socket.timeout, ConnectionRefusedError, OSError):
                time.sleep(0.5)

        logger.warning(f"Container {host} did not become ready in {timeout}s")

    def stop_idle_containers(self, timeout_seconds: int = 900) -> None:
        now = time.time()
        to_remove = []

        for name, last_access in self.last_accessed.items():
            if now - last_access > timeout_seconds:
                try:
                    logger.info(f"Scale-down: Stopping idle container {name}")
                    container = self.client.containers.get(name)
                    if container.status == "running":
                        container.stop()
                    to_remove.append(name)
                except docker.errors.NotFound:
                    to_remove.append(name)
                except Exception as e:
                    logger.error(f"Failed to stop {name}: {e}")

        for name in to_remove:
            del self.last_accessed[name]

    def prune_managed_containers(self):
        """
        Kills and removes containers managed by this service (zombies).
        """
        logger.info("Pruning zombie containers...")
        try:
            containers = self.client.containers.list(
                all=True,  # Include stopped ones
                filters={"label": "created_by=sample-dind"},
            )
            for container in containers:
                logger.info(f"Removing zombie container: {container.name}")
                try:
                    if container.status == "running":
                        container.kill()
                    container.remove(force=True)
                except Exception as e:
                    logger.warning(f"Failed to remove {container.name}: {e}")
        except Exception as e:
            logger.error(f"Failed to prune containers: {e}")
