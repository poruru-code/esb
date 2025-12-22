import time
import subprocess
from pathlib import Path
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler
import docker

from tools.provisioner import main as provisioner
from tools.cli.config import PROJECT_ROOT
from dotenv import load_dotenv


class SmartReloader(FileSystemEventHandler):
    def __init__(self):
        self.docker_client = docker.from_env()
        self.last_trigger = 0
        self.cooldown = 1.0  # 重複実行防止用クールダウン(秒)

    def on_modified(self, event):
        if event.is_directory:
            return

        current_time = time.time()
        if current_time - self.last_trigger < self.cooldown:
            return

        path = Path(event.src_path)
        filename = path.name

        try:
            # Case 1: template.yaml の変更
            if filename == "template.yaml":
                self.handle_template_change()
                self.last_trigger = time.time()

            # Case 2: Lambda関数コードの変更
            elif path.suffix == ".py" and "functions" in str(path):
                self.handle_function_change(path)
                self.last_trigger = time.time()

        except Exception as e:
            print(f"⚠️ Error during reload: {e}")

    def handle_template_change(self):
        print("\n🔄 Template change detected.")

        # 1. Config再生成
        print("  • Regenerating configs...")
        from tools.cli.commands.build import generator
        from tools.cli.config import PROJECT_ROOT

        config_path = PROJECT_ROOT / "tests/e2e/generator.yml"
        config = generator.load_config(config_path)
        generator.generate_files(config=config, project_root=PROJECT_ROOT)

        # 2. Gateway再起動 (ルーティング反映のため)
        print("  • Restarting Gateway...")
        try:
            subprocess.run(["docker", "compose", "restart", "gateway"], check=True)
        except subprocess.CalledProcessError as e:
            print(f"  ❌ Failed to restart gateway: {e}")

        # 3. リソース再プロビジョニング (DBテーブル追加など)
        print("  • Provisioning resources...")
        from tools.cli.config import TEMPLATE_YAML

        provisioner.main(template_path=TEMPLATE_YAML)
        print("✅ System updated.")

    def handle_function_change(self, path: Path):
        # パスから関数名を特定 (例: .../functions/hello/lambda_function.py -> hello)
        # ディレクトリ構造: tests/e2e/functions/{name}/...
        try:
            # "functions" の直後のディレクトリ名を取得
            parts = path.parts
            if "functions" in parts:
                idx = parts.index("functions")
                if len(parts) > idx + 1:
                    func_dir_name = parts[idx + 1]
                    image_tag = f"lambda-{func_dir_name}:latest"
                    # Dockerfile を探す
                    func_dir = PROJECT_ROOT / "tests" / "e2e" / "functions" / func_dir_name
                    dockerfile_path = func_dir / "Dockerfile"

                    if not dockerfile_path.exists():
                        print(f"  ⚠️ Dockerfile not found at {dockerfile_path}")
                        return

                    print(f"\n🔄 Code change detected: {func_dir_name}")

                    # 1. イメージのリビルド
                    print(f"  • Rebuilding image: {image_tag}...", end="", flush=True)
                    # build.py と同様に PROJECT_ROOT をコンテキストにする
                    relative_dockerfile = dockerfile_path.relative_to(PROJECT_ROOT).as_posix()
                    self.docker_client.images.build(
                        path=str(PROJECT_ROOT),
                        dockerfile=relative_dockerfile,
                        tag=image_tag,
                        rm=True,
                    )
                    print(" ✅")

                    # 2. 実行中の古いコンテナを停止 (Freshな起動を促す)
                    containers = self.docker_client.containers.list(
                        filters={"ancestor": f"{image_tag}"}
                    )
                    if containers:
                        for c in containers:
                            print(f"  • Killing running container: {c.name}")
                            c.kill()

                    print("✅ Function updated.")
        except Exception as e:
            print(f"  ❌ Failed to update function: {e}")


def run(args):
    # .env.test の読み込み
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        print(f"Loading environment variables from {env_file}")
        load_dotenv(env_file, verbose=True, override=False)

    print("👀 Watching for changes in project root...")
    print("   - template.yaml: Reconfigures Gateway & Resources")
    print("   - functions/**/*.py: Rebuilds Lambda Images")

    event_handler = SmartReloader()
    observer = Observer()

    # 監視対象ディレクトリの設定 (プロジェクトルート)
    observer.schedule(event_handler, str(PROJECT_ROOT), recursive=True)

    observer.start()
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nStopping watcher...")
        observer.stop()
    observer.join()
