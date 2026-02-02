# Where: services/gateway/services/config_reloader.py
# What: Hot-reload watcher for runtime config files.
# Why: Reload Gateway config without restart after deploy.
"""
Config reloader for hot reload functionality.

Periodically reloads functions.yml and routing.yml when files change.
Works in conjunction with FunctionRegistry and RouteMatcher.
"""

import logging
import os
import stat
import threading
from typing import Callable, Optional

from services.gateway.config import config

logger = logging.getLogger("gateway.config_reloader")


class ConfigFileWatcher:
    """
    Watches a single config file for changes using modification time.
    """

    def __init__(self, file_path: str):
        """
        Args:
            file_path: Path to the config file to watch
        """
        self.file_path = file_path
        self._last_mtime: Optional[float] = None
        self._lock = threading.RLock()

    def has_changed(self) -> bool:
        """
        Check if the file has been modified since last check.

        Returns:
            True if file was modified, False otherwise
        """
        try:
            current_mtime = os.stat(self.file_path)[stat.ST_MTIME]
            with self._lock:
                if self._last_mtime is None:
                    self._last_mtime = current_mtime
                    return False
                if current_mtime > self._last_mtime:
                    self._last_mtime = current_mtime
                    return True
                return False
        except FileNotFoundError:
            logger.warning(f"Config file not found: {self.file_path}")
            return False
        except OSError as e:
            logger.error(f"Error checking config file {self.file_path}: {e}")
            return False

    def update_mtime(self) -> None:
        """
        Update the last known modification time.
        """
        try:
            with self._lock:
                self._last_mtime = os.stat(self.file_path)[stat.ST_MTIME]
        except (FileNotFoundError, OSError):
            self._last_mtime = None


class ConfigReloader:
    """
    Manages hot reloading of gateway configuration files.

    Periodically checks functions.yml and routing.yml for changes and
    triggers callbacks when files are modified.
    """

    def __init__(
        self,
        functions_reload_callback: Optional[Callable[[], None]] = None,
        routing_reload_callback: Optional[Callable[[], None]] = None,
    ):
        """
        Args:
            functions_reload_callback: Callback when functions.yml changes
            routing_reload_callback: Callback when routing.yml changes
        """
        self._enabled = config.CONFIG_RELOAD_ENABLED
        self._interval = max(0.5, config.CONFIG_RELOAD_INTERVAL)  # Minimum 0.5s
        self._lock_timeout = config.CONFIG_RELOAD_LOCK_TIMEOUT

        self._functions_watcher: Optional[ConfigFileWatcher] = None
        self._routing_watcher: Optional[ConfigFileWatcher] = None
        self._resources_watcher: Optional[ConfigFileWatcher] = None

        self._functions_reload_callback = functions_reload_callback
        self._routing_reload_callback = routing_reload_callback

        self._stop_event = threading.Event()
        self._thread: Optional[threading.Thread] = None
        self._reload_lock = threading.RLock()

        self._last_functions_mtime: Optional[float] = None
        self._last_routing_mtime: Optional[float] = None
        self._last_resources_mtime: Optional[float] = None

        # Track initialization state
        self._initialized = False

    def initialize(self) -> None:
        """
        Initialize watchers with current file modification times.
        Must be called before start().
        """
        if self._functions_watcher is None and os.path.exists(config.FUNCTIONS_CONFIG_PATH):
            self._functions_watcher = ConfigFileWatcher(config.FUNCTIONS_CONFIG_PATH)
            self._functions_watcher.update_mtime()
            self._last_functions_mtime = self._functions_watcher._last_mtime

        if self._routing_watcher is None and os.path.exists(config.ROUTING_CONFIG_PATH):
            self._routing_watcher = ConfigFileWatcher(config.ROUTING_CONFIG_PATH)
            self._routing_watcher.update_mtime()
            self._last_routing_mtime = self._routing_watcher._last_mtime

        if self._resources_watcher is None and os.path.exists(config.RESOURCES_CONFIG_PATH):
            self._resources_watcher = ConfigFileWatcher(config.RESOURCES_CONFIG_PATH)
            self._resources_watcher.update_mtime()
            self._last_resources_mtime = self._resources_watcher._last_mtime

        self._initialized = True
        logger.info(
            f"Config reloader initialized (interval={self._interval}s, enabled={self._enabled})"
        )

    def start(self) -> None:
        """
        Start the background reloader thread.
        """
        if not self._enabled:
            logger.info("Config reloader is disabled")
            return

        if not self._initialized:
            self.initialize()

        if self._thread is not None and self._thread.is_alive():
            logger.warning("Config reloader already running")
            return

        self._stop_event.clear()
        self._thread = threading.Thread(target=self._run_loop, daemon=True, name="config-reloader")
        self._thread.start()
        logger.info("Config reloader started")

    def stop(self) -> None:
        """
        Stop the background reloader thread.
        """
        self._stop_event.set()
        if self._thread is not None and self._thread.is_alive():
            self._thread.join(timeout=self._lock_timeout + 1.0)
            self._thread = None
            logger.info("Config reloader stopped")

    def _run_loop(self) -> None:
        """
        Main loop for periodic config checking.
        """
        while not self._stop_event.is_set():
            try:
                self._check_and_reload()
            except Exception as e:
                logger.error(f"Error in config reload loop: {e}")

            self._stop_event.wait(timeout=self._interval)

    def _check_and_reload(self) -> None:
        """
        Check all watched files and trigger reload callbacks if changed.
        """
        with self._reload_lock:
            # Check functions.yml
            if self._functions_watcher is not None and self._functions_watcher.has_changed():
                logger.info("Detected changes in functions.yml, reloading...")
                if self._functions_reload_callback is not None:
                    try:
                        self._functions_reload_callback()
                        logger.info("Functions config reloaded successfully")
                    except Exception as e:
                        logger.error(f"Error reloading functions config: {e}")

            # Check routing.yml
            if self._routing_watcher is not None and self._routing_watcher.has_changed():
                logger.info("Detected changes in routing.yml, reloading...")
                if self._routing_reload_callback is not None:
                    try:
                        self._routing_reload_callback()
                        logger.info("Routing config reloaded successfully")
                    except Exception as e:
                        logger.error(f"Error reloading routing config: {e}")

    def add_functions_watcher(self, file_path: str) -> None:
        """
        Add a watcher for functions.yml at runtime.

        Args:
            file_path: Path to the functions.yml file
        """
        if os.path.exists(file_path):
            self._functions_watcher = ConfigFileWatcher(file_path)
            self._functions_watcher.update_mtime()
            self._last_functions_mtime = self._functions_watcher._last_mtime

    def add_routing_watcher(self, file_path: str) -> None:
        """
        Add a watcher for routing.yml at runtime.

        Args:
            file_path: Path to the routing.yml file
        """
        if os.path.exists(file_path):
            self._routing_watcher = ConfigFileWatcher(file_path)
            self._routing_watcher.update_mtime()
            self._last_routing_mtime = self._routing_watcher._last_mtime


# Global reloader instance (initialized by gateway app)
_reloader: Optional[ConfigReloader] = None


def get_reloader() -> Optional[ConfigReloader]:
    """
    Get the global config reloader instance.

    Returns:
        ConfigReloader instance or None if not initialized
    """
    return _reloader


def init_reloader(
    functions_callback: Optional[Callable[[], None]] = None,
    routing_callback: Optional[Callable[[], None]] = None,
) -> ConfigReloader:
    """
    Initialize and return the global config reloader.

    Args:
        functions_callback: Callback when functions.yml changes
        routing_callback: Callback when routing.yml changes

    Returns:
        ConfigReloader instance
    """
    global _reloader  # noqa: PLW0603
    _reloader = ConfigReloader(
        functions_reload_callback=functions_callback,
        routing_reload_callback=routing_callback,
    )
    _reloader.initialize()
    return _reloader


def start_reloader() -> None:
    """
    Start the global config reloader.
    """
    global _reloader  # noqa: PLW0603
    if _reloader is not None:
        _reloader.start()


def stop_reloader() -> None:
    """
    Stop the global config reloader.
    """
    global _reloader  # noqa: PLW0603
    if _reloader is not None:
        _reloader.stop()
