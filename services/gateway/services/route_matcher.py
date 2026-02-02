"""
Route matching service.

Loads routing.yml and resolves target containers from request paths/methods.
Provides functionality different from FastAPI's APIRouter.
Supports hot reload via ConfigReloader.
"""

import logging
import re
import threading
from typing import Any, Dict, List, Optional, Tuple, Union

import yaml

from services.gateway.config import config
from services.gateway.models.function import FunctionEntity

logger = logging.getLogger(__name__)


class RouteMatcher:
    """
    Route matching service that resolves target containers from request paths/methods.

    Thread-safe matcher that loads routing.yml and provides route resolution.
    Supports hot reload via ConfigReloader.
    """

    def __init__(self, function_registry: Any):
        """
        Args:
            function_registry: FunctionRegistry instance
        """
        self.function_registry = function_registry
        self.config_path = config.ROUTING_CONFIG_PATH
        self._routing_config: List[Dict[str, Any]] = []
        self._lock = threading.RLock()

    def load_routing_config(self, force: bool = False) -> List[Dict[str, Any]]:
        """
        Load routing.yml and cache it.

        Args:
            force: If True, force reload even if file hasn't changed

        Returns:
            List of route configurations
        """
        try:
            with open(self.config_path, "r", encoding="utf-8") as f:
                cfg = yaml.safe_load(f) or {}
        except FileNotFoundError:
            logger.warning(f"Warning: Routing config not found at {self.config_path}")
            return self._get_routing_copy()
        except yaml.YAMLError as e:
            logger.error(f"Error parsing routing config: {e}")
            return self._get_routing_copy()
        except Exception as e:
            logger.error(f"Error loading routing config: {e}")
            return self._get_routing_copy()

        routes = cfg.get("routes")
        if routes is None:
            routes = []
        if not isinstance(routes, list):
            logger.error("routing.yml has invalid format: routes must be a list")
            return self._get_routing_copy()

        with self._lock:
            self._routing_config = routes

        logger.info(f"Loaded {len(self._routing_config)} routes from {self.config_path}")

        return self._get_routing_copy()

    def reload(self) -> None:
        """
        Force reload of the routing configuration.
        Called by ConfigReloader when routing.yml changes.
        """
        logger.info("Reloading routing configuration...")
        self.load_routing_config(force=True)

    def _get_routing_copy(self) -> List[Dict[str, Any]]:
        """
        Get a thread-safe copy of the routing config.

        Returns:
            Copy of the routing configuration list
        """
        with self._lock:
            return list(self._routing_config)

    def _path_to_regex(self, path_pattern: str) -> str:
        """
        Convert a path pattern to a regular expression.

        Example: "/users/{user_id}/posts/{post_id}"
            → "^/users/(?P<user_id>[^/]+)/posts/(?P<post_id>[^/]+)$"
        """
        # Replace {param} with named capture groups.
        regex_pattern = re.sub(r"\{(\w+)\}", r"(?P<\1>[^/]+)", path_pattern)
        return f"^{regex_pattern}$"

    def match_route(
        self, request_path: str, request_method: str
    ) -> Tuple[Optional[str], Dict[str, str], Optional[str], Union[FunctionEntity, Dict[str, Any]]]:
        """
        Resolve the target container from request path and method.

        Args:
            request_path: request path (e.g., "/api/users/123")
            request_method: HTTP method (e.g., "POST")

        Returns:
            Tuple of:
                - target_container: container name (None if not found)
                - path_params: dict of path parameters
                - route_path: matched route pattern (for resource)
                - function_config: function settings (environment, scaling, etc.)
        """
        routing_config = self._get_routing_copy()

        if not routing_config:
            # Try loading if not loaded
            routing_config = self.load_routing_config()
            if not routing_config:
                return None, {}, None, {}

        for route in routing_config:
            route_path = route.get("path", "")
            route_method = route.get("method", "").upper()

            # Check if method matches.
            if request_method.upper() != route_method:
                continue

            # Convert path pattern to regex and match.
            regex_pattern = self._path_to_regex(route_path)
            match = re.match(regex_pattern, request_path)

            if match:
                # Extract path parameters.
                path_params = match.groupdict()

                # Get function config (new format: string, old format: dict).
                function_ref = route.get("function", {})

                if isinstance(function_ref, str):
                    # New format: fetch config from function_registry.
                    target_container = function_ref
                    function_config = self.function_registry.get_function_config(function_ref) or {}
                else:
                    # Old format (backward compatible): use dict directly.
                    target_container = function_ref.get("container", "")
                    function_config = function_ref

                return target_container, path_params, route_path, function_config

        # No matching route found.
        return None, {}, None, {}

    def list_routes(self) -> List[Dict[str, Any]]:
        """
        List all registered routes.

        Returns:
            List of route configurations
        """
        return self._get_routing_copy()

    def get_route_count(self) -> int:
        """
        Get the number of registered routes.

        Returns:
            Number of routes
        """
        return len(self._get_routing_copy())
