"""
esb logs - Display service logs.

Usage:
    esb logs [service] [options]

Examples:
    esb logs                  # Show logs for all services
    esb logs gateway          # Gateway only
    esb logs -f               # Follow logs (like tail -f)
    esb logs gateway -f --tail 50  # Follow the latest 50 lines for Gateway
"""
import subprocess
import sys
from tools.cli.config import PROJECT_ROOT
from dotenv import load_dotenv
from tools.cli.core import logging


def run(args):
    """
    Display Docker Compose logs.
    """
    # Load .env.test.
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        load_dotenv(env_file, override=False)

    cmd = ["docker", "compose", "logs"]

    # --follow option (follow logs in real time).
    if getattr(args, "follow", False):
        cmd.append("--follow")

    # --tail option (show only the latest N lines).
    tail = getattr(args, "tail", None)
    if tail:
        cmd.extend(["--tail", str(tail)])

    # --timestamps option.
    if getattr(args, "timestamps", False):
        cmd.append("--timestamps")

    # Service name (all services when unspecified).
    service = getattr(args, "service", None)
    if service:
        cmd.append(service)

    try:
        # Run directly to allow interruption with Ctrl+C.
        subprocess.run(cmd, check=False)
    except KeyboardInterrupt:
        print()  # Newline.
        sys.exit(0)
