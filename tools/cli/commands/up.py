import sys
from . import build
from tools.provisioner import main as provisioner
from tools.cli.config import PROJECT_ROOT
from dotenv import load_dotenv
import subprocess


def run(args):
    # .env.test の読み込み (run_tests.py と同様)
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        print(f"Loading environment variables from {env_file}")
        load_dotenv(env_file, override=False)

    # 1. ビルド要求があれば実行
    if getattr(args, "build", False):
        build.run(args)

    # 2. サービス起動
    print("🚀 Starting services...")
    cmd = ["docker", "compose", "up"]
    if getattr(args, "detach", True):
        cmd.append("-d")

    try:
        subprocess.check_call(cmd)
    except subprocess.CalledProcessError as e:
        print(f"❌ Failed to start services: {e}")
        sys.exit(1)

    # 3. インフラプロビジョニング
    print("🛠️ Preparing infrastructure...")
    from tools.cli.config import TEMPLATE_YAML

    provisioner.main(template_path=TEMPLATE_YAML)

    print("\n✅ Environment is ready! (https://localhost:443)")
