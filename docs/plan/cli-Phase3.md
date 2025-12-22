# 🚀 Phase 3 詳細設計: "ESB CLI" (Edge Serverless Box CLI)

## 1. ディレクトリ構成

CLI の機能ごとにモジュールを分割し、保守性を高めます。

```text
.
├── tools/
│   ├── cli/
│   │   ├── __init__.py
│   │   ├── main.py              # CLI エントリポイント
│   │   ├── config.py            # CLI設定 (プロジェクトルート判定など)
│   │   ├── core/                # 共通ロジック
│   │   │   ├── docker_client.py # Docker SDK ラッパー
│   │   │   └── shell.py         # サブプロセス実行ヘルパー
│   │   └── commands/            # 各コマンドの実装
│   │       ├── build.py         # 定義ファイル生成 & イメージビルド
│   │       ├── up.py            # 起動 & プロビジョニング
│   │       ├── watch.py         # ファイル監視 & ホットリロード
│   │       └── logs.py          # ログ閲覧
│   ├── generator/               # (Phase 1 成果物)
│   └── provisioner/             # (Phase 2 成果物)
└── pyproject.toml               # 依存関係定義

```

## 2. 依存ライブラリの追加 (`pyproject.toml`)

Docker操作とファイル監視のためにライブラリを追加します。
※ `docker` は Python Docker SDK です。

```bash
uv add --dev docker watchdog

```

## 3. 実装詳細

### A. CLI エントリポイント (`tools/cli/main.py`)

サブコマンド方式で設計し、拡張性を持たせます。

```python
#!/usr/bin/env python3
import argparse
import sys
from pathlib import Path

# モジュールパスの解決
sys.path.append(str(Path(__file__).parent.parent.parent))

from tools.cli.commands import build, up, watch, down

def main():
    parser = argparse.ArgumentParser(
        description="Edge Serverless Box CLI",
        formatter_class=argparse.RawDescriptionHelpFormatter
    )
    subparsers = parser.add_subparsers(dest="command", required=True, help="Command to execute")

    # --- build command ---
    build_parser = subparsers.add_parser("build", help="Generate config and build function images")
    build_parser.add_argument("--no-cache", action="store_true", help="Do not use cache when building images")

    # --- up command ---
    up_parser = subparsers.add_parser("up", help="Start the environment")
    up_parser.add_argument("--build", action="store_true", help="Rebuild before starting")
    up_parser.add_argument("--detach", "-d", action="store_true", default=True, help="Run in background")

    # --- watch command ---
    watch_parser = subparsers.add_parser("watch", help="Watch for changes and hot-reload")

    # --- down command ---
    subparsers.add_parser("down", help="Stop the environment")

    args = parser.parse_args()

    try:
        if args.command == "build":
            build.run(args)
        elif args.command == "up":
            up.run(args)
        elif args.command == "watch":
            watch.run(args)
        elif args.command == "down":
            down.run(args)
    except KeyboardInterrupt:
        print("\nAborted.")
        sys.exit(0)
    except Exception as e:
        print(f"\n❌ Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()

```

### B. ビルドロジック (`tools/cli/commands/build.py`)

Phase 1 のジェネレータを実行するだけでなく、**生成された Dockerfile を元に実際に Docker イメージをビルドする** 機能を追加します。これがManagerによるコンテナ起動の前提となります。

```python
import docker
from pathlib import Path
from tools.generator import main as generator

PROJECT_ROOT = Path(__file__).parent.parent.parent.parent

def build_function_images(no_cache=False):
    """
    生成されたDockerfileを見つけてイメージをビルドする
    """
    client = docker.from_env()
    functions_dir = PROJECT_ROOT / "tests/e2e/functions" # ※実際の設定に合わせてパス調整

    print("🐳 Building function images...")
    
    # functionsディレクトリ以下のDockerfileを探索
    for dockerfile in functions_dir.rglob("Dockerfile"):
        func_dir = dockerfile.parent
        func_name = func_dir.name
        image_tag = f"lambda-{func_name}:latest" # イメージ名の命名規則
        
        print(f"  • Building {image_tag} ...", end="", flush=True)
        try:
            client.images.build(
                path=str(func_dir),
                tag=image_tag,
                nocache=no_cache,
                rm=True
            )
            print(" ✅")
        except docker.errors.BuildError as e:
            print(f" ❌\nBuild failed: {e}")
            raise

def run(args):
    # 1. 設定ファイル生成 (Phase 1 Generator)
    print("📝 Generating configurations...")
    generator.main() # 引数調整が必要な場合は generator.generate_files を直接呼ぶ

    # 2. イメージビルド
    build_function_images(no_cache=getattr(args, "no_cache", False))
    
    print("✨ Build complete.")

```

### C. 起動 & プロビジョニング (`tools/cli/commands/up.py`)

`docker compose` と Phase 2 のプロビジョナーを統合します。

```python
import subprocess
from tools.provisioner import main as provisioner
from . import build

def run(args):
    if args.build:
        build.run(args)

    print("🚀 Starting services...")
    cmd = ["docker", "compose", "up"]
    if args.detach:
        cmd.append("-d")
    
    # ユーザーに見えるようにサブプロセス実行
    subprocess.check_call(cmd)

    print("urop Preparing infrastructure...")
    # Phase 2 Provisioner 実行
    provisioner.main()
    
    print("\n✅ Environment is ready! (https://localhost:443)")

```

### D. 高機能 Watcher (`tools/cli/commands/watch.py`)

ここが DX 向上の要です。ファイルの変更種類に応じて、最適な「最小限のアクション」を選択して実行します。

```python
import time
import subprocess
from pathlib import Path
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler
import docker

from tools.generator import main as generator
from tools.provisioner import main as provisioner

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
        generator.main()

        # 2. Gateway再起動 (ルーティング反映のため)
        print("  • Restarting Gateway...")
        subprocess.run(["docker", "compose", "restart", "gateway"], check=True)

        # 3. リソース再プロビジョニング (DBテーブル追加など)
        print("  • Provisioning resources...")
        provisioner.main()
        print("✅ System updated.")

    def handle_function_change(self, path: Path):
        # パスから関数名を特定 (例: .../functions/hello/lambda_function.py -> hello)
        # ディレクトリ構造に依存するため、generatorの出力ロジックと合わせる必要あり
        func_dir_name = path.parent.name
        image_tag = f"lambda-{func_dir_name}:latest"

        print(f"\n🔄 Code change detected: {func_dir_name}")
        
        # 1. イメージのリビルド
        print(f"  • Rebuilding image: {image_tag}...", end="", flush=True)
        self.docker_client.images.build(
            path=str(path.parent),
            tag=image_tag,
            rm=True
        )
        print(" ✅")

        # 2. (Optional) 実行中の古いコンテナがあれば停止
        # Gateway/Managerは、次にInvokeされたときに新しいイメージを使ってコンテナを立て直す
        containers = self.docker_client.containers.list(filters={"ancestor": image_tag})
        for c in containers:
            print(f"  • Killing running container: {c.name}")
            c.kill() # 強制停止して、次のリクエストでFreshなコンテナを使わせる

        print("✅ Function updated.")

def run(args):
    print("👀 Watching for changes...")
    print("   - template.yaml: Reconfigures Gateway & Resources")
    print("   - functions/**/*.py: Rebuilds Lambda Images")

    event_handler = SmartReloader()
    observer = Observer()
    
    # 監視対象ディレクトリの設定
    root_dir = Path(".").resolve()
    observer.schedule(event_handler, str(root_dir), recursive=True)
    
    observer.start()
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        observer.stop()
    observer.join()

```

## 4. 運用イメージ

開発者は、`uv run esb watch` (またはエイリアス設定して `esb watch`) を実行するだけで作業を開始できます。

1. **関数の修正**: `lambda_function.py` を保存 → **即座にイメージビルド & 既存コンテナ破棄**。次の `curl` で新しいコードが走る。
2. **API追加**: `template.yaml` に追記して保存 → **Gateway再起動 & ルーティング更新**。即座に新しいパスにアクセス可能。
3. **DB追加**: `template.yaml` に `AWS::DynamoDB::Table` を追記 → **即座にテーブル作成**。

これにより、クラウド(AWS)へのデプロイ待ち時間をゼロにし、ローカルならではの爆速開発体験を提供できます。