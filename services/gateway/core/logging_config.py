"""
Logging Configuration
Python JSON Logger を使用した構造化ログの設定を行います。
"""

import logging
import sys
from pythonjsonlogger import jsonlogger


class CustomJsonFormatter(jsonlogger.JsonFormatter):
    """
    アプリケーション固有のフィールドを追加するカスタムフォーマッタ
    """

    def add_fields(self, log_record, record, message_dict):
        super(CustomJsonFormatter, self).add_fields(log_record, record, message_dict)
        if not log_record.get("timestamp"):
            # ISO format with timezone info
            from datetime import datetime

            log_record["timestamp"] = datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%S.%fZ")
        if log_record.get("level"):
            log_record["level"] = log_record["level"].upper()
        else:
            log_record["level"] = record.levelname


def setup_logging(level=logging.INFO):
    """
    JSONロギングのセットアップ
    """
    logger = logging.getLogger()
    logger.setLevel(level)

    handler = logging.StreamHandler(sys.stdout)
    formatter = CustomJsonFormatter("%(timestamp)s %(level)s %(name)s %(message)s")
    handler.setFormatter(formatter)

    # 既存のハンドラをクリア
    logger.handlers = []
    logger.addHandler(handler)

    # ライブラリのログレベル調整
    logging.getLogger("uvicorn.access").setLevel(logging.WARNING)
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("urllib3").setLevel(logging.WARNING)
