"""
esb logs - サービスログの表示

Usage:
    esb logs [service] [options]

Examples:
    esb logs                  # 全サービスのログを表示
    esb logs gateway          # Gateway のみ
    esb logs -f               # ログをフォロー（tail -f のように）
    esb logs gateway -f --tail 50  # Gateway の最新50行をフォロー
"""
import subprocess
import sys
from tools.cli.config import PROJECT_ROOT
from dotenv import load_dotenv
from tools.cli.core import logging


def run(args):
    """
    Docker Compose のログを表示する
    """
    # .env.test の読み込み
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        load_dotenv(env_file, override=False)

    cmd = ["docker", "compose", "logs"]

    # --follow オプション（リアルタイムでログを追跡）
    if getattr(args, "follow", False):
        cmd.append("--follow")

    # --tail オプション（最新N行のみ表示）
    tail = getattr(args, "tail", None)
    if tail:
        cmd.extend(["--tail", str(tail)])

    # --timestamps オプション
    if getattr(args, "timestamps", False):
        cmd.append("--timestamps")

    # サービス名（指定がない場合は全サービス）
    service = getattr(args, "service", None)
    if service:
        cmd.append(service)

    try:
        # Ctrl+C で中断可能にするため、直接実行
        subprocess.run(cmd, check=False)
    except KeyboardInterrupt:
        print()  # 改行
        sys.exit(0)
