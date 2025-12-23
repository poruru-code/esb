import docker
from tools.generator import main as generator
from tools.cli.config import PROJECT_ROOT, E2E_DIR, TEMPLATE_YAML
from tools.cli.core import logging


def build_function_images(no_cache=False, verbose=False):
    """
    生成されたDockerfileを見つけてイメージをビルドする
    """
    client = docker.from_env()
    functions_dir = E2E_DIR / "functions"

    logging.step("Building function images...")

    if not functions_dir.exists():
        logging.warning(f"Functions directory {functions_dir} not found.")
        return

    # functionsディレクトリ以下のDockerfileを探索
    for dockerfile in sorted(functions_dir.rglob("Dockerfile")):
        func_dir = dockerfile.parent
        func_name = func_dir.name
        image_tag = f"lambda-{func_name}:latest"

        print(f"  • Building {logging.highlight(image_tag)} ...", end="", flush=True)
        try:
            # ビルドコンテキストを PROJECT_ROOT に設定し、
            # Dockerfile の相対パスを PROJECT_ROOT から計算する
            relative_dockerfile = dockerfile.relative_to(PROJECT_ROOT).as_posix()

            client.images.build(
                path=str(PROJECT_ROOT),
                dockerfile=relative_dockerfile,
                tag=image_tag,
                nocache=no_cache,
                rm=True,
            )
            print(f" {logging.Color.GREEN}✅{logging.Color.END}")
        except Exception as e:
            print(f" {logging.Color.RED}❌{logging.Color.END}")
            if verbose:
                logging.error(f"Build failed for {image_tag}: {e}")
                raise
            else:
                logging.error(f"Build failed for {image_tag}. Use --verbose for details.")
                # Non-verbose: exit or raise without trace?
                # CLI usually should stop on error.
                import sys

                sys.exit(1)


def run(args):
    # 1. 設定ファイル生成 (Phase 1 Generator)
    logging.step("Generating configurations...")
    logging.info(f"Using template: {logging.highlight(TEMPLATE_YAML)}")

    # Generator の設定をロード
    config_path = E2E_DIR / "generator.yml"
    if not config_path.exists():
        config_path = PROJECT_ROOT / "tests/fixtures/generator.yml"

    config = generator.load_config(config_path)

    # テンプレートパスを解決
    if "paths" not in config:
        config["paths"] = {}
    config["paths"]["sam_template"] = str(TEMPLATE_YAML)

    generator.generate_files(config=config, project_root=PROJECT_ROOT, dry_run=False, verbose=False)
    logging.success("Configurations generated.")

    # 2. イメージビルド
    no_cache = getattr(args, "no_cache", False)
    verbose = getattr(args, "verbose", False)
    build_function_images(no_cache=no_cache, verbose=verbose)

    logging.success("Build complete.")
