import subprocess
import sys
from tools.cli.config import PROJECT_ROOT
from dotenv import load_dotenv
from tools.cli.core import logging


def run(args):
    # .env.test の読み込み
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        load_dotenv(env_file, override=False)

    logging.step("Stopping services...")
    cmd = ["docker", "compose", "down", "--remove-orphans"]

    try:
        subprocess.check_call(cmd)
        logging.success("Services stopped.")
    except subprocess.CalledProcessError as e:
        logging.error(f"Failed to stop services: {e}")
        sys.exit(1)
