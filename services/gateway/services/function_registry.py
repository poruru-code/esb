"""
Lambda function registry.

Loads functions.yml and provides name-to-config mapping.
Merges default environment variables into function-specific settings.
Supports hot reload via ConfigReloader.
"""

import logging
import os
import string
import threading
from typing import Any, Dict, Optional

import yaml

from services.gateway.config import config
from services.gateway.models.function import FunctionEntity

logger = logging.getLogger("gateway.function_registry")


class FunctionRegistry:
    """
    Registry for Lambda function configurations.

    Thread-safe registry that loads functions.yml and provides
    name-to-config mapping with default environment variable merging.
    Supports hot reload via ConfigReloader.
    """

    def __init__(self):
        self._registry: Dict[str, Dict[str, Any]] = {}
        self._defaults: Dict[str, Any] = {}
        self.config_path = config.FUNCTIONS_CONFIG_PATH
        self._lock = threading.RLock()

    def load_functions_config(self, force: bool = False) -> Dict[str, Dict[str, Any]]:
        """
        Load and cache functions.yml.

        Args:
            force: If True, force reload even if file hasn't changed

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
        except FileNotFoundError:
            logger.warning(f"Functions config not found at {self.config_path}")
            return self._get_registry_copy()
        except yaml.YAMLError as e:
            logger.error(f"Error parsing functions config: {e}")
            return self._get_registry_copy()
        except Exception as e:
            logger.error(f"Error loading functions config: {e}")
            return self._get_registry_copy()

        functions_cfg = cfg.get("functions", {})
        if isinstance(functions_cfg, dict):
            invalid = []
            for name, spec in functions_cfg.items():
                if isinstance(spec, dict) and "image" in spec:
                    invalid.append(name)
            if invalid:
                message = (
                    "functions.yml does not allow 'image' entries. "
                    f"Remove image from: {', '.join(sorted(invalid))}"
                )
                logger.error(message)
                return self._get_registry_copy()
        else:
            logger.error("functions.yml has invalid format: functions must be a map")
            return self._get_registry_copy()

        defaults_cfg = cfg.get("defaults", {})
        if not isinstance(defaults_cfg, dict):
            defaults_cfg = {}

        with self._lock:
            self._defaults = defaults_cfg
            self._registry = functions_cfg

        logger.info(f"Loaded {len(self._registry)} functions from {self.config_path}")

        return self._get_registry_copy()

    def reload(self) -> None:
        """
        Force reload of the functions configuration.
        Called by ConfigReloader when functions.yml changes.
        """
        logger.info("Reloading functions configuration...")
        self.load_functions_config(force=True)

    def _get_registry_copy(self) -> Dict[str, Dict[str, Any]]:
        """
        Get a thread-safe copy of the registry.

        Returns:
            Copy of the function registry
        """
        with self._lock:
            return dict(self._registry)

    def _get_defaults_copy(self) -> Dict[str, Any]:
        """
        Get a thread-safe copy of the defaults.

        Returns:
            Copy of the defaults
        """
        with self._lock:
            return dict(self._defaults)

    def get_function_config(self, function_name: str) -> Optional[FunctionEntity]:
        """
        Get configuration by function name.

        Merge default environment variables and scaling settings into function-specific settings.

        Args:
            function_name: function name (container name)

        Returns:
            FunctionEntity (with defaults merged), or None if missing
        """
        registry = self._get_registry_copy()
        if function_name not in registry:
            return None

        func_config = registry[function_name] or {}

        # Get defaults for merging
        defaults = self._get_defaults_copy()

        # Merge defaults (environment & scaling).
        merged_env = dict(defaults.get("environment", {}))
        merged_env.update(func_config.get("environment", {}))

        merged_scaling = dict(defaults.get("scaling", {}))
        merged_scaling.update(func_config.get("scaling", {}))

        # Build data for entity.
        data = dict(func_config)
        data["environment"] = merged_env
        data["scaling"] = merged_scaling

        return FunctionEntity.from_dict(function_name, data)

    def list_functions(self) -> Dict[str, Dict[str, Any]]:
        """
        List all registered functions.

        Returns:
            Dict of function name -> config (without defaults merged)
        """
        return self._get_registry_copy()

    def get_function_names(self) -> list[str]:
        """
        Get list of all function names.

        Returns:
            List of function names
        """
        return list(self._get_registry_copy().keys())

    def get_defaults(self) -> Dict[str, Any]:
        """
        Get the default configuration.

        Returns:
            Default settings for functions
        """
        return self._get_defaults_copy()
