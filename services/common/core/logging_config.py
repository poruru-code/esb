"""
Logging Configuration
Custom JSON Logger implementation optimized for VictoriaLogs.
"""

import logging
import json
from datetime import datetime, timezone


class CustomJsonFormatter(logging.Formatter):
    """
    VictoriaLogs optimized JSON Formatter.

    Fields:
      - _time: ISO8601 timestamp (millisecond precision)
      - level: Log level
      - logger: Logger name (e.g. uvicorn.access, gateway.main)
      - message: Log message
      - request_id: Request ID (if available in record or via ContextVar)
    """

    def format(self, record: logging.LogRecord) -> str:
        # RequestID resolution
        request_id = getattr(record, "request_id", None)
        if not request_id:
            try:
                from .request_context import get_request_id

                request_id = get_request_id()
            except ImportError:
                pass

        log_data = {
            "_time": datetime.fromtimestamp(record.created, tz=timezone.utc).isoformat(
                timespec="milliseconds"
            ),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }

        if request_id:
            log_data["request_id"] = request_id

        # Include extra fields
        standard_attrs = {
            "args",
            "asctime",
            "created",
            "exc_info",
            "exc_text",
            "filename",
            "funcName",
            "levelname",
            "levelno",
            "lineno",
            "module",
            "msecs",
            "message",
            "msg",
            "name",
            "pathname",
            "process",
            "processName",
            "relativeCreated",
            "stack_info",
            "thread",
            "threadName",
        }

        for key, value in record.__dict__.items():
            if key not in standard_attrs and not key.startswith("_"):
                log_data[key] = value

        if record.exc_info:
            log_data["exception"] = self.formatException(record.exc_info)

        return json.dumps(log_data, ensure_ascii=False)


def setup_logging(config_path: str = None):
    """
    Basic logging setup helper.
    Actually logging should be configured via dictionary or YAML usually.
    """
    pass
