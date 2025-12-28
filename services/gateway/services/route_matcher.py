"""
Route matching service.

Loads routing.yml and resolves target containers from request paths/methods.

Note:
    Provides functionality different from FastAPI's APIRouter.
    This module implements config-based route matching logic.
"""

import re
from typing import Optional, Tuple, Dict, Any, List
import yaml
import logging

from ..config import config

logger = logging.getLogger(__name__)


class RouteMatcher:
    def __init__(self, function_registry: Any):
        """
        Args:
            function_registry: FunctionRegistry instance
        """
        self.function_registry = function_registry
        self.config_path = config.ROUTING_CONFIG_PATH
        self._routing_config: List[Dict[str, Any]] = []

    def load_routing_config(self) -> List[Dict[str, Any]]:
        """
        Load routing.yml and cache it.
        """
        try:
            with open(self.config_path, "r", encoding="utf-8") as f:
                cfg = yaml.safe_load(f) or {}
                self._routing_config = cfg.get("routes") or []
                logger.info(f"Loaded {len(self._routing_config)} routes from {self.config_path}")
        except FileNotFoundError:
            logger.warning(f"Warning: Routing config not found at {self.config_path}")
            self._routing_config = []
        except yaml.YAMLError as e:
            logger.error(f"Error parsing routing config: {e}")
            self._routing_config = []

        return self._routing_config

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
    ) -> Tuple[Optional[str], Dict[str, str], Optional[str], Dict[str, Any]]:
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
                - function_config: function settings (image, environment, etc.)
        """
        if not self._routing_config:
            self.load_routing_config()

        for route in self._routing_config:
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
