# V3-0.2.4 (アダプタパターン版 + VictoriaLogs)
#
# 環境変数 LOCAL_LAMBDA_ENV=true の場合、VictoriaLogsへ送信
# それ以外は CloudWatchLogClient を使用 (AWS CloudWatch Logs API)

import time
import datetime
import os
from abc import ABC, abstractmethod

LOG_CATEGORY_ERROR = "ERROR"
LOG_CATEGORY_WARN = "WARNING"
LOG_CATEGORY_INFO = "INFO"
LOG_CATEGORY_DEBUG = "DEBUG"
LOG_CATEGORY_DEBUG_LARGE = "DEBUG(LARGE_DATA)"

ENVIRON_LOG_DEBUG = "LOG_DEBUG"
ENVIRON_LOG_DEBUG_LARGE_DATA = "LOG_DEBUG_LARGE_DATA"
ENVIRON_LOG_STREAM_NAME = "AWS_LAMBDA_LOG_STREAM_NAME"
ENVIRON_LOG_GROUP_NAME = "AWS_LAMBDA_LOG_GROUP_NAME"
ENVIRON_LAMBDA_FUNCTION_NAME = "AWS_LAMBDA_FUNCTION_NAME"

_IS_LOCAL = os.environ.get("LOCAL_LAMBDA_ENV", "false").lower() == "true"


# ============================================
# ログクライアント アダプターインターフェース
# ============================================
class LogClientAdapter(ABC):
    @abstractmethod
    def put_log_events(self, group_name: str, stream_name: str, log_events: list):
        pass

    @abstractmethod
    def create_log_group(self, group_name: str):
        pass

    @abstractmethod
    def create_log_stream(self, group_name: str, stream_name: str):
        pass

    @abstractmethod
    def put_large_data(self, user_name: str, item_name: str, message: str) -> str:
        pass


# ============================================
# ローカル環境用アダプター (VictoriaLogs送信)
# ============================================
class LocalLogClient(LogClientAdapter):
    VICTORIALOGS_URL = "http://victorialogs:9428/insert/jsonline"

    def put_log_events(self, group_name: str, stream_name: str, log_events: list):
        import json

        for event in log_events:
            log_entry = {
                "_msg": event["message"],
                "_time": event["timestamp"] / 1000,
                "log_group": group_name,
                "log_stream": stream_name,
            }
            # Direct stdout output as JSON line.
            # Docker Logging Driver (Fluentd) will pick this up.
            print(json.dumps(log_entry))

    def create_log_group(self, group_name: str):
        pass

    def create_log_stream(self, group_name: str, stream_name: str):
        pass

    def put_large_data(self, user_name: str, item_name: str, message: str) -> str:
        log_dir = "/logs/debug_large"
        os.makedirs(log_dir, exist_ok=True)
        dt_now = datetime.datetime.now()
        file_name = f"{user_name}.{item_name}.{dt_now.strftime('%Y%m%d%H%M%S%f')[:-3]}.log"
        file_path = os.path.join(log_dir, file_name)
        with open(file_path, "w", encoding="utf-8") as f:
            f.write(message)
        return f"outputLocalFile:{file_path}"


# ============================================
# 本番環境用アダプター (AWS CloudWatch Logs)
# ============================================
class CloudWatchLogClient(LogClientAdapter):
    def __init__(self):
        import boto3

        self._client = boto3.client("logs")
        self._s3 = boto3.resource("s3")

    def put_log_events(self, group_name: str, stream_name: str, log_events: list):
        try:
            self._client.put_log_events(
                logGroupName=group_name, logStreamName=stream_name, logEvents=log_events
            )
        except Exception as e:
            now_str = datetime.datetime.now().strftime("%Y/%m/%d %H:%M:%S.%f")
            print(f"{now_str} [ERROR] put_log_events failed: {e}")

    def create_log_group(self, group_name: str):
        try:
            self._client.create_log_group(logGroupName=group_name)
        except self._client.exceptions.ResourceAlreadyExistsException:
            pass
        except Exception as e:
            print(f"[ERROR] create_log_group failed: {e}")

    def create_log_stream(self, group_name: str, stream_name: str):
        try:
            self._client.create_log_stream(logGroupName=group_name, logStreamName=stream_name)
        except self._client.exceptions.ResourceAlreadyExistsException:
            pass
        except Exception as e:
            print(f"[ERROR] create_log_stream failed: {e}")

    def put_large_data(self, user_name: str, item_name: str, message: str) -> str:
        bucket_name = os.environ.get("PADMA_LOG_LARGE_BUCKET_NAME")
        dt_now = datetime.datetime.now()
        stream_name = os.environ.get(ENVIRON_LOG_STREAM_NAME, "").replace("/", "")
        file_name = (
            f"{user_name}.{item_name}.{dt_now.strftime('%Y%m%d%H%M%S%f')[:-3]}.{stream_name}.log"
        )
        file_path = f"log_debug_large_data/{user_name}/{os.environ.get(ENVIRON_LAMBDA_FUNCTION_NAME)}/{file_name}"
        s3_object = self._s3.Object(bucket_name, file_path)
        result = s3_object.put(Body=message.encode("utf-8"))
        return f"outputS3File:{file_path} result:{result}"


# ============================================
# CloudWatchLogger (アダプターを使用)
# ============================================
class CloudWatchLogger:
    def __init__(self, user_name: str):
        self.user_name = user_name
        self.log_client: LogClientAdapter = LocalLogClient() if _IS_LOCAL else CloudWatchLogClient()

        self.lambda_path = os.environ.get(ENVIRON_LOG_GROUP_NAME, "/local/lambda")
        self.group_name = f"{self.lambda_path}/{user_name}"
        self.stream_name = os.environ.get(ENVIRON_LOG_STREAM_NAME, "local-stream")

        self.is_output_debug_log = os.environ.get(ENVIRON_LOG_DEBUG, "").lower() == "true"
        self.is_output_debug_large_data = (
            os.environ.get(ENVIRON_LOG_DEBUG_LARGE_DATA, "").lower() == "true"
        )

        self.log_client.create_log_group(self.group_name)
        self.log_client.create_log_stream(self.group_name, self.stream_name)

    def error(self, func):
        self._logging_message(LOG_CATEGORY_ERROR, func())

    def warning(self, func):
        self._logging_message(LOG_CATEGORY_WARN, func())

    def info(self, func):
        self._logging_message(LOG_CATEGORY_INFO, func())

    def debug(self, func):
        if self.is_output_debug_log:
            self._logging_message(LOG_CATEGORY_DEBUG, func())

    def output_debug_large_data(self, item_name: str, func):
        if self.is_output_debug_large_data:
            try:
                message = func()
                result = self.log_client.put_large_data(self.user_name, item_name, message)
                self._logging_message(LOG_CATEGORY_DEBUG_LARGE, result)
            except Exception as e:
                self._logging_message(LOG_CATEGORY_ERROR, f"LargeDataError:{e}")

    def _logging_message(self, log_category: str, message: str):
        timestamp = int(time.time()) * 1000
        dt_now = datetime.datetime.now()
        log_message = f"{dt_now.strftime('%Y/%m/%d %H:%M:%S.%f')} [{log_category}] {message}"
        log_event = {"timestamp": timestamp, "message": log_message}
        self.log_client.put_log_events(self.group_name, self.stream_name, [log_event])
