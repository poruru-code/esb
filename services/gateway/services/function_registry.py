"""
Lambda function registry.

Loads functions.yml and provides name-to-config mapping.
Merges default environment variables into function-specific settings.
"""

import logging
import os
import string
from typing import Any, Dict, Optional

import yaml

from services.gateway.config import config
from services.gateway.models.function import FunctionEntity

logger = logging.getLogger("gateway.function_registry")


class FunctionRegistry:
    def __init__(self):
        self._registry: Dict[str, Dict[str, Any]] = {}
        self._defaults: Dict[str, Any] = {}
        self.config_path = config.FUNCTIONS_CONFIG_PATH

    def load_functions_config(self) -> Dict[str, Dict[str, Any]]:
        """
        Load and cache functions.yml.

        Returns:
            Dict of function name -> config
        """
        try:
            with open(self.config_path, "r", encoding="utf-8") as f:
                # Substitute environment variables using string.Template.
                template = string.Template(f.read())

                # Build default mapping.
                mapping = os.environ.copy()
                if "LOG_LEVEL" not in mapping:
                    mapping["LOG_LEVEL"] = "INFO"

                content = template.safe_substitute(mapping)
                cfg = yaml.safe_load(content) or {}

            self._defaults = cfg.get("defaults", {})
            self._registry = cfg.get("functions", {})

            logger.info(f"Loaded {len(self._registry)} functions from {self.config_path}")

        except FileNotFoundError:
            logger.warning(f"Functions config not found at {self.config_path}")
            self._registry = {}
            self._defaults = {}

        except yaml.YAMLError as e:
            logger.error(f"Error parsing functions config: {e}")
            self._registry = {}
            self._defaults = {}

        return self._registry

    def get_function_config(self, function_name: str) -> Optional[FunctionEntity]:
        """
        Get configuration by function name.

        Merge default environment variables and scaling settings into function-specific settings.

        Args:
            function_name: function name (container name)

        Returns:
            FunctionEntity (with defaults merged), or None if missing
        """
        if function_name not in self._registry:
            return None

        func_config = self._registry[function_name] or {}

        # Merge defaults (environment & scaling).
        merged_env = dict(self._defaults.get("environment", {}))
        merged_env.update(func_config.get("environment", {}))

        merged_scaling = dict(self._defaults.get("scaling", {}))
        merged_scaling.update(func_config.get("scaling", {}))

        # Build data for entity.
        data = dict(func_config)
        data["environment"] = merged_env
        data["scaling"] = merged_scaling

        return FunctionEntity.from_dict(function_name, data)
