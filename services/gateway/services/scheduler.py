"""
APScheduler - ライフサイクル管理とcron実行

- アイドルコンテナのクリーンアップ
- 定期実行Lambda関数のトリガー
"""

from apscheduler.schedulers.blocking import BlockingScheduler
from apscheduler.triggers.cron import CronTrigger
import logging

import os

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# アイドルタイムアウト（分）- 環境変数で上書き可能
IDLE_TIMEOUT_MINUTES = int(os.environ.get("IDLE_TIMEOUT_MINUTES", 5))
IDLE_TIMEOUT = IDLE_TIMEOUT_MINUTES * 60  # 秒に変換


def cleanup_idle_containers():
    """
    アイドル状態のLambdaコンテナを停止

    ContainerManagerに委譲してアイドルコンテナを検出・停止
    """
    logger.info("Starting idle container cleanup...")

    try:
        from .container import get_manager

        manager = get_manager()

        # アイドルタイムアウトを超えたコンテナを停止
        manager.stop_idle_containers(timeout_seconds=IDLE_TIMEOUT)

        logger.info("Cleanup completed")

    except Exception as e:
        logger.error(f"Cleanup failed: {str(e)}")


def scheduled_lambda_execution():
    """
    定期実行されるLambda関数のトリガー
    """
    logger.info("Executing scheduled Lambda function...")

    # TODO: lambda_gatewayのinvoke機能を呼び出す
    # または直接コンテナを起動してイベントを送信

    pass


def main():
    """
    スケジューラーのメインループ
    """
    scheduler = BlockingScheduler()

    # アイドルコンテナのクリーンアップ（毎分実行）
    scheduler.add_job(
        cleanup_idle_containers,
        trigger=CronTrigger(minute="*"),
        id="cleanup_idle_containers",
        name="Cleanup idle Lambda containers",
    )

    # 定期実行Lambda（例：毎時0分に実行）
    # scheduler.add_job(
    #     scheduled_lambda_execution,
    #     trigger=CronTrigger(hour="*", minute="0"),
    #     id="scheduled_lambda",
    #     name="Scheduled Lambda execution"
    # )

    logger.info("APScheduler started")

    try:
        scheduler.start()
    except (KeyboardInterrupt, SystemExit):
        logger.info("APScheduler stopped")


if __name__ == "__main__":
    main()
