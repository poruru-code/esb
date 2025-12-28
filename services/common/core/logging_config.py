"""
Logging Configuration
Custom JSON Logger implementation optimized for VictoriaLogs.

Provides:
- CustomJsonFormatter: VictoriaLogs optimized JSON formatter
- VictoriaLogsHandler: Direct HTTP logging with stdout fallback
- configure_queue_logging: Async logging for long-lived processes
"""

import atexit
import json
import logging
import logging.config
import logging.handlers
import os
import queue
import string
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone

import yaml


class CustomJsonFormatter(logging.Formatter):
    """
    VictoriaLogs optimized JSON Formatter.

    Fields:
      - _time: ISO8601 timestamp (millisecond precision)
      - level: Log level
      - logger: Logger name (e.g. uvicorn.access, gateway.main)
      - message: Log message
      - trace_id: Trace ID for distributed tracing (X-Amzn-Trace-Id root)
    """

    def format(self, record: logging.LogRecord) -> str:
        # Trace ID resolution
        trace_id = getattr(record, "trace_id", None)
        if not trace_id:
            try:
                from .request_context import get_trace_id

                trace_id = get_trace_id()
            except ImportError:
                pass

        # Request ID resolution
        request_id = getattr(record, "aws_request_id", None)
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

        if trace_id:
            log_data["trace_id"] = trace_id
        if request_id:
            log_data["aws_request_id"] = request_id

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


def setup_logging(config_path: str = "logging.yml"):
    """
    Load the YAML config, substitute environment variables, and initialize logging.
    """
    if not os.path.exists(config_path):
        logging.basicConfig(level=logging.INFO)
        return

    with open(config_path, "r", encoding="utf-8") as f:
        # Substitute environment variables using string.Template.
        # Supports ${LOG_LEVEL} format.
        template = string.Template(f.read())

        # Default values.
        mapping = os.environ.copy()
        if "LOG_LEVEL" not in mapping:
            mapping["LOG_LEVEL"] = "INFO"

        content = template.safe_substitute(mapping)
        config = yaml.safe_load(content)
        logging.config.dictConfig(config)


class VictoriaLogsHandler(logging.Handler):
    """
    Handler that sends logs directly to VictoriaLogs over HTTP.
    On failure, fall back to stderr and rely on Docker's json-file driver.
    """

    def __init__(self, url: str, stream_fields: dict = None, timeout: float = 0.5):
        super().__init__()
        self.url = url
        self.stream_fields = stream_fields or {}
        self.timeout = timeout

    def emit(self, record: logging.LogRecord):
        try:
            # Build log message.
            if self.formatter:
                msg = self.formatter.format(record)
            else:
                msg = record.getMessage()

            # Expect JSON; wrap otherwise.
            try:
                log_entry = json.loads(msg)
            except json.JSONDecodeError:
                log_entry = {"message": msg, "level": record.levelname}

            # Merge stream_fields into the log payload.
            # Include them in JSON body as well as URL params so VictoriaLogs
            # recognizes the stream reliably.
            if self.stream_fields:
                for k, v in self.stream_fields.items():
                    if k not in log_entry:
                        log_entry[k] = v

            # Build URL parameters.
            params = [
                ("_stream_fields", ",".join(self.stream_fields.keys())),
                ("_msg_field", "message"),
                ("_time_field", "_time"),
            ]
            for k, v in self.stream_fields.items():
                params.append((k, str(v)))

            query_string = urllib.parse.urlencode(params)
            full_url = f"{self.url}?{query_string}"

            # Send data.
            data = json.dumps(log_entry, ensure_ascii=False).encode("utf-8")
            req = urllib.request.Request(
                full_url,
                data=data,
                headers={"Content-Type": "application/json"},
                method="POST",
            )

            try:
                with urllib.request.urlopen(req, timeout=self.timeout) as res:
                    res.read()
            except (OSError, urllib.error.URLError) as e:
                # Fallback: write to stderr.
                # Use sys.__stderr__ to avoid StreamToLogger infinite loops.
                fallback_msg = json.dumps(
                    {
                        "fallback": "victorialogs_failed",
                        "error": str(e),
                        "original_log": log_entry,
                    },
                    ensure_ascii=False,
                )
                stream = getattr(sys, "__stderr__", sys.stderr)
                try:
                    stream.write(fallback_msg + "\n")
                except Exception:
                    pass  # Prevent app from crashing in worst case.

        except Exception:
            self.handleError(record)

    def flush(self):
        pass


def configure_queue_logging(service_name: str, vl_url: str = None):
    """
    Configure async QueueLogging.
    Used for long-running processes like Gateway/Manager.
    """
    if not vl_url:
        return

    # 1. Real handler for sending (runs on a separate thread).
    real_handler = VictoriaLogsHandler(
        url=vl_url, stream_fields={"container_name": service_name, "job": "services"}
    )
    real_handler.setFormatter(CustomJsonFormatter())

    # 2. Queue and QueueHandler (app side).
    log_queue = queue.Queue(-1)
    queue_handler = logging.handlers.QueueHandler(log_queue)

    # 3. Start listener.
    listener = logging.handlers.QueueListener(log_queue, real_handler)
    listener.start()
    atexit.register(listener.stop)

    # 4. Add to root logger.
    root = logging.getLogger()
    root.addHandler(queue_handler)
