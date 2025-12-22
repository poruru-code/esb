import os
import json
import time
from datetime import datetime, timezone
import boto3
from botocore.config import Config

_original_boto3_client = boto3.client

# --- Constants & Config ---
LOG_LEVEL_MAP = {"DEBUG": 10, "INFO": 20, "WARNING": 30, "ERROR": 40, "CRITICAL": 50}

SERVICE_CONFIG = {
    "s3": {
        "env_var": "S3_ENDPOINT",
        "config": Config(s3={"addressing_style": "path"}, signature_version="s3v4"),
    },
    "dynamodb": {
        "env_var": "DYNAMODB_ENDPOINT",
        "config": Config(
            retries={"max_attempts": 10, "mode": "standard"}, connect_timeout=5, read_timeout=5
        ),
    },
}


def _get_iso8601_ms(ts_ms):
    """ミリ秒精度の epoch から ISO8601 文字列を生成 (boto3.mock 共通形式)"""
    dt = datetime.fromtimestamp(ts_ms / 1000.0, tz=timezone.utc)
    return dt.isoformat(timespec="milliseconds")


def _estimate_container_name(log_group):
    """log_group からコンテナ名を推定する"""
    if not log_group.startswith("/lambda/"):
        return "unknown"

    func_name = log_group[len("/lambda/") :]
    if func_name.endswith("-test"):
        func_name = func_name[:-5]

    name = f"lambda-{func_name}"
    return name[1:] if name.startswith("/") else name


# --- Patched Methods ---


def _patched_put_log_events(self, **kwargs):
    """PutLogEvents を標準出力への JSON 出力に差し替える (Transparent Logging)"""
    try:
        log_group = kwargs.get("logGroupName", "unknown")
        log_stream = kwargs.get("logStreamName", "unknown")
        log_events = kwargs.get("logEvents", [])

        container_name = _estimate_container_name(log_group)
        print(
            f"[sitecustomize] Estimated container_name: '{container_name}' (log_group: '{log_group}')"
        )

        # LOG_LEVEL によるフィルタリング閾値の取得
        current_threshold = LOG_LEVEL_MAP.get(os.environ.get("LOG_LEVEL", "INFO").upper(), 20)

        for event in log_events:
            msg = event.get("message", "")
            # AWS SDK は通常ミリ秒精度で送信する
            ts_ms = event.get("timestamp", int(time.time() * 1000))

            # ログレベルのパース ([DEBUG] message 形式)
            level = "INFO"
            clean_msg = msg
            for lvl in LOG_LEVEL_MAP.keys():
                if msg.startswith(f"[{lvl}]"):
                    level = lvl
                    clean_msg = msg[len(f"[{lvl}]") :].lstrip()
                    break

            # フィルタリング判定
            if LOG_LEVEL_MAP.get(level, 20) < current_threshold:
                continue

            log_entry = {
                "_time": _get_iso8601_ms(ts_ms),
                "level": level,
                "message": clean_msg,
                "log_group": log_group,
                "log_stream": log_stream,
                "logger": "boto3.mock",
                "container_name": container_name,
            }
            print(json.dumps(log_entry, ensure_ascii=False))

        return {"nextSequenceToken": "mock-token"}
    except Exception as e:
        print(f"[sitecustomize] Error in _patched_put_log_events: {e}")
        raise e


def _patched_boto3_client(service_name, *args, **kwargs):
    """boto3.client() をインターセプトし、エンドポイントの変更やモック化を行う"""
    try:
        # Logs サービスの場合はさらに _make_api_call を差し替える（stdoutモード & ローカルモック）
        if service_name == "logs":
            print("[sitecustomize] Creating original boto3 client for logs (local mock mode)...")
            client = _original_boto3_client(service_name, *args, **kwargs)

            _original_make_api_call = client._make_api_call

            def _patched_make_api_call(operation_name, api_params):
                if operation_name == "PutLogEvents":
                    return _patched_put_log_events(client, **api_params)

                # 管理系操作はスタブレスポンスを返す (Gateway 通信の回避)
                if operation_name in (
                    "CreateLogGroup",
                    "CreateLogStream",
                    "DeleteLogGroup",
                    "DeleteLogStream",
                ):
                    return {}

                if operation_name == "DescribeLogGroups":
                    return {"logGroups": []}

                if operation_name == "DescribeLogStreams":
                    return {"logStreams": []}

                return _original_make_api_call(operation_name, api_params)

            client._make_api_call = _patched_make_api_call
            print("[sitecustomize] boto3.client('logs') patched (Full Local Mock mode)")
            return client

        # その他のサービス (S3, DynamoDB等) のエンドポイントリダイレクト
        service_cfg = SERVICE_CONFIG.get(service_name)
        if service_cfg:
            endpoint = os.environ.get(service_cfg["env_var"])
            if endpoint:
                kwargs["endpoint_url"] = endpoint
                kwargs["verify"] = False
                if service_cfg["config"]:
                    existing = kwargs.get("config")
                    kwargs["config"] = (
                        existing.merge(service_cfg["config"]) if existing else service_cfg["config"]
                    )

                print(f"[sitecustomize] Redirecting {service_name} to {endpoint}")
                return _original_boto3_client(service_name, *args, **kwargs)

        return _original_boto3_client(service_name, *args, **kwargs)
    except Exception as e:
        print(f"[sitecustomize] Error in _patched_boto3_client for {service_name}: {e}")
        import traceback

        traceback.print_exc()
        raise e


# Monkey Patch
boto3.client = _patched_boto3_client
