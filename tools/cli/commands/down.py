import subprocess
import sys
from tools.cli.config import PROJECT_ROOT
from dotenv import load_dotenv


def run(args):
    # .env.test の読み込み
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        load_dotenv(env_file, override=False)

    print("🛑 Stopping services...")
    cmd = ["docker", "compose", "down", "--remove-orphans"]

    try:
        subprocess.check_call(cmd)
        print("✅ Services stopped.")
    except subprocess.CalledProcessError as e:
        print(f"❌ Failed to stop services: {e}")
        sys.exit(1)
