"""
Lambda Logging Utilities

Provides robust logging for short-lived Lambda environments.
Ensures logs are flushed before the Lambda execution context freezes.
"""

import functools
import logging
import os
import sys

from .logging_config import CustomJsonFormatter, VictoriaLogsHandler


class StreamToLogger:
    """
    Redirects stdout/stderr to a logger instance.
    Captures print() statements and sends them through the logging system.
    """

    def __init__(self, logger: logging.Logger, level: int):
        self.logger = logger
        self.level = level

    def write(self, buf: str):
        for line in buf.rstrip().splitlines():
            if line.strip():
                self.logger.log(self.level, line.rstrip())

    def flush(self):
        pass


def robust_lambda_logger(service_name: str = "lambda"):
    """
    Decorator for Lambda handlers to ensure logs are flushed and stdout is captured.

    Features:
    - Adds VictoriaLogsHandler if VICTORIALOGS_URL is set
    - Captures stdout/stderr and sends through logging
    - Flushes all handlers in finally block (important for Lambda freeze)

    Usage:
        @robust_lambda_logger(service_name="echo-func")
        def lambda_handler(event, context):
            print("This will be logged!")
            return {"statusCode": 200}
    """

    def decorator(func):
        @functools.wraps(func)
        def wrapper(event, context):
            vl_url = os.getenv("VICTORIALOGS_URL")
            original_stdout = sys.stdout
            original_stderr = sys.stderr

            logger = logging.getLogger()

            # Setup: add VictoriaLogsHandler if missing.
            if vl_url:
                # Check for existing handlers (avoid duplicates).
                if not any(isinstance(h, VictoriaLogsHandler) for h in logger.handlers):
                    handler = VictoriaLogsHandler(
                        url=vl_url,
                        stream_fields={"container_name": service_name, "job": "lambda"},
                    )
                    handler.setFormatter(CustomJsonFormatter())
                    logger.addHandler(handler)

                    # Adjust log level if needed.
                    if logger.getEffectiveLevel() > logging.INFO:
                        logger.setLevel(logging.INFO)

                # Hijack stdout/stderr.
                sys.stdout = StreamToLogger(logging.getLogger("stdout"), logging.INFO)
                sys.stderr = StreamToLogger(logging.getLogger("stderr"), logging.ERROR)

            try:
                return func(event, context)
            finally:
                # Teardown: flush and restore.

                # Call flush explicitly even for sync handlers
                # (in case buffering is added later).
                for h in logger.handlers:
                    if isinstance(h, VictoriaLogsHandler):
                        h.flush()

                sys.stdout = original_stdout
                sys.stderr = original_stderr

        return wrapper

    return decorator
