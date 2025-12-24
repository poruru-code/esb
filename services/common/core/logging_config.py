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
    YAML設定ファイルを読み込み、環境変数を置換した上でロギングを初期化します。
    """
    if not os.path.exists(config_path):
        logging.basicConfig(level=logging.INFO)
        return

    with open(config_path, "r", encoding="utf-8") as f:
        # string.Templateを使用して環境変数を置換
        # ${LOG_LEVEL} などの形式に対応
        template = string.Template(f.read())

        # デフォルト値の設定
        mapping = os.environ.copy()
        if "LOG_LEVEL" not in mapping:
            mapping["LOG_LEVEL"] = "INFO"

        content = template.safe_substitute(mapping)
        config = yaml.safe_load(content)
        logging.config.dictConfig(config)


class VictoriaLogsHandler(logging.Handler):
    """
    VictoriaLogsへHTTPで直接ログを送信するハンドラー。
    失敗時は標準エラー出力へフォールバックし、Dockerのjson-fileログドライバーに任せる。
    """

    def __init__(self, url: str, stream_fields: dict = None, timeout: float = 0.5):
        super().__init__()
        self.url = url
        self.stream_fields = stream_fields or {}
        self.timeout = timeout

    def emit(self, record: logging.LogRecord):
        try:
            # ログメッセージの生成
            if self.formatter:
                msg = self.formatter.format(record)
            else:
                msg = record.getMessage()

            # JSON形式であることを期待するが、そうでなければラップする
            try:
                log_entry = json.loads(msg)
            except json.JSONDecodeError:
                log_entry = {"message": msg, "level": record.levelname}

            # stream_fields をログデータ本体にもマージする
            # URLパラメータだけでなく、JSONボディにも含めることで
            # VictoriaLogsが確実にストリームとして認識できるようにする
            if self.stream_fields:
                for k, v in self.stream_fields.items():
                    if k not in log_entry:
                        log_entry[k] = v

            # URLパラメータ構築
            params = [
                ("_stream_fields", ",".join(self.stream_fields.keys())),
                ("_msg_field", "message"),
                ("_time_field", "_time"),
            ]
            for k, v in self.stream_fields.items():
                params.append((k, str(v)))

            query_string = urllib.parse.urlencode(params)
            full_url = f"{self.url}?{query_string}"

            # データ送信
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
                # フォールバック: 標準エラー出力へ
                # sys.__stderr__ を使用して StreamToLogger による無限ループを回避
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
                    pass  # 最悪のケースでもアプリ停止を防ぐ

        except Exception:
            self.handleError(record)

    def flush(self):
        pass


def configure_queue_logging(service_name: str, vl_url: str = None):
    """
    非同期QueueLoggingを構成する。
    Gateway/Managerなどの常駐プロセスで使用。
    """
    if not vl_url:
        return

    # 1. 送信用の実ハンドラー (別スレッドで動作)
    real_handler = VictoriaLogsHandler(
        url=vl_url, stream_fields={"container_name": service_name, "job": "services"}
    )
    real_handler.setFormatter(CustomJsonFormatter())

    # 2. キューとQueueHandler (アプリ側)
    log_queue = queue.Queue(-1)
    queue_handler = logging.handlers.QueueHandler(log_queue)

    # 3. リスナー起動
    listener = logging.handlers.QueueListener(log_queue, real_handler)
    listener.start()
    atexit.register(listener.stop)

    # 4. ルートロガーに追加
    root = logging.getLogger()
    root.addHandler(queue_handler)
