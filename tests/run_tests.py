#!/usr/bin/env python3
import argparse
import os
import sys
import subprocess
from pathlib import Path
from dotenv import load_dotenv

# プロジェクトルート
PROJECT_ROOT = Path(__file__).parent.parent.resolve()


def run_esb(args: list[str], check: bool = True):
    """esb CLIを実行するヘルパー"""
    # インストール済みコマンドではなく、現在のソースコードを使用
    cmd = [sys.executable, "-m", "tools.cli.main"] + args
    print(f"Running: {' '.join(cmd)}")
    subprocess.run(cmd, cwd=PROJECT_ROOT, check=check)


def main():
    # 警告を抑制
    import warnings
    import urllib3

    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    warnings.filterwarnings("ignore", category=DeprecationWarning)

    parser = argparse.ArgumentParser(description="E2E Test Runner (ESB CLI Wrapper)")
    parser.add_argument("--build", action="store_true", help="Rebuild images before running")
    parser.add_argument("--cleanup", action="store_true", help="Stop containers after tests")
    parser.add_argument("--reset", action="store_true", help="Full reset before running")
    # --dind は config.py/CLI側で検知するか、COMPOSE_FILE で指定する
    parser.add_argument(
        "--dind", action="store_true", help="Use DinD mode (docker-compose.dind.yml)"
    )
    parser.add_argument("--unit", action="store_true", help="Run unit tests")
    parser.add_argument("--unit-only", action="store_true", help="Run unit tests only")

    args = parser.parse_args()

    # --- Unit Tests ---
    if args.unit or args.unit_only:
        print("\n=== Running Unit Tests ===\n")
        cmd = [sys.executable, "-m", "pytest", "services/gateway/tests", "tools/cli/tests", "-v"]
        res = subprocess.run(cmd, cwd=PROJECT_ROOT, check=False)
        if res.returncode != 0:
            print("\n❌ Unit Tests failed.")
            sys.exit(res.returncode)
        print("\n🎉 Unit Tests passed!")

        if args.unit_only:
            sys.exit(0)

    # --- 環境設定 ---
    # .env.test を最初にロード（ESB_TEMPLATE等の設定を取得）
    env_file = PROJECT_ROOT / "tests" / ".env.test"
    if env_file.exists():
        load_dotenv(env_file, override=False)

    env = os.environ.copy()

    # ESB_TEMPLATE: .env.test から読み込んだ相対パスを絶対パスに変換
    esb_template = os.getenv("ESB_TEMPLATE", "tests/fixtures/template.yaml")
    env["ESB_TEMPLATE"] = str(PROJECT_ROOT / esb_template)

    # COMPOSE_FILE: テスト用定義をマージする
    # Windows/Linuxで区切り文字が異なるため注意
    separator = ";" if os.name == "nt" else ":"

    base_compose = "docker-compose.dind.yml" if args.dind else "docker-compose.yml"
    compose_files = [base_compose, "tests/docker-compose.test.yml"]
    env["COMPOSE_FILE"] = separator.join(compose_files)

    # 子プロセス実行用に環境変数を適用
    os.environ.update(env)

    try:
        # --- ステップ実行 ---

        # 1. Reset (任意)
        if args.reset:
            run_esb(["reset"])

        # 2. Build (任意 - reset時は強制)
        # ESB_TEMPLATE が効いているため、自動的にテスト用Lambdaがビルドされる
        if args.build or args.reset:
            run_esb(["build"])

        # 3. Up
        # 証明書生成は内部で行われ、--waitで起動完了までブロックする
        # DinDモードかどうかのフラグは compose file で制御しているので up コマンド自体は変わらない
        up_args = ["up", "--detach", "--wait"]
        run_esb(up_args)

        # 4. Run Tests (Pytest)
        print("\n=== Running E2E Tests ===\n")
        # pytest実行時は環境変数(COMPOSE_FILE等)が渡った状態で実行される
        # .env.testの内容も必要だが、CLIのupコマンド内でload_dotenvされている。
        # pytest側でも読み込む必要があるため、環境変数をロードするか、pytest内で読み込ませる。
        # run_tests.pyでload_dotenvしておくのが無難。
        env_file = PROJECT_ROOT / "tests" / ".env.test"
        if env_file.exists():
            load_dotenv(env_file, override=False)

        # 環境変数を再取得（load_dotenv後）
        pytest_env = os.environ.copy()

        pytest_cmd = [sys.executable, "-m", "pytest", "tests/", "-v"]
        result = subprocess.run(pytest_cmd, cwd=PROJECT_ROOT, check=False, env=pytest_env)

        if result.returncode != 0:
            print("\n❌ Tests failed.")
            # テスト失敗時でもクリーンアップは finally で実行
            sys.exit(result.returncode)

        print("\n🎉 Tests passed successfully!")

    except subprocess.CalledProcessError as e:
        print(f"Error executing command: {e}")
        sys.exit(1)

    finally:
        # 5. Cleanup
        if args.cleanup:
            # downコマンドも COMPOSE_FILE を参照して正しく終了させる
            run_esb(["down"])


if __name__ == "__main__":
    sys.exit(main())
