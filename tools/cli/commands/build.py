import docker
from tools.generator import main as generator
from tools.cli.config import PROJECT_ROOT, E2E_DIR


def build_function_images(no_cache=False):
    """
    生成されたDockerfileを見つけてイメージをビルドする
    """
    client = docker.from_env()
    functions_dir = E2E_DIR / "functions"

    print("🐳 Building function images...")

    if not functions_dir.exists():
        print(f"  Warning: Functions directory {functions_dir} not found.")
        return

    # functionsディレクトリ以下のDockerfileを探索
    for dockerfile in sorted(functions_dir.rglob("Dockerfile")):
        func_dir = dockerfile.parent
        func_name = func_dir.name
        image_tag = f"lambda-{func_name}:latest"

        print(f"  • Building {image_tag} ...", end="", flush=True)
        try:
            # ビルドコンテキストを PROJECT_ROOT に設定し、
            # Dockerfile の相対パスを PROJECT_ROOT から計算する
            # Docker 側での解決のため as_posix() でスラッシュに統一
            relative_dockerfile = dockerfile.relative_to(PROJECT_ROOT).as_posix()

            client.images.build(
                path=str(PROJECT_ROOT),
                dockerfile=relative_dockerfile,
                tag=image_tag,
                nocache=no_cache,
                rm=True,
            )
            print(" ✅")
        except Exception as e:
            print(f" ❌\nBuild failed for {image_tag}: {e}")
            raise


def run(args):
    # 1. 設定ファイル生成 (Phase 1 Generator)
    print("📝 Generating configurations...")

    # Generator の設定をロード
    # Phase 3 ではカレントディレクトリ等の考慮が必要だが、現状は E2E 向けに固定
    config_path = PROJECT_ROOT / "tests/e2e/generator.yml"
    config = generator.load_config(config_path)

    # 必要に応じてテンプレートパスなどを上書き
    # 現状はデフォルト設定を使用

    generator.generate_files(config=config, project_root=PROJECT_ROOT, dry_run=False, verbose=False)

    # 2. イメージビルド
    no_cache = getattr(args, "no_cache", False)
    build_function_images(no_cache=no_cache)

    print("✨ Build complete.")
