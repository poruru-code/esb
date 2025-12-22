#!/usr/bin/env python3
"""
SAM Template Generator

SAMテンプレートからローカル実行用のDockerfileとfunctions.ymlを生成します。

Usage:
    python -m tools.generator.main [options]

Options:
    --config PATH       Generator config path (default: tools/generator/generator.yml)
    --template PATH     SAM template path (overrides config)
    --dry-run           Show what would be generated without writing files
    --verbose           Verbose output
"""

import argparse
import sys
from pathlib import Path

import yaml

from .parser import parse_sam_template
from .renderer import render_dockerfile, render_functions_yml, render_routing_yml


def load_config(config_path: Path) -> dict:
    """設定ファイルをロード"""
    if not config_path.exists():
        return {}

    with open(config_path, encoding="utf-8") as f:
        return yaml.safe_load(f) or {}


def generate_files(
    config: dict,
    project_root: Path | None = None,
    dry_run: bool = False,
    verbose: bool = False,
) -> None:
    """
    SAMテンプレートからファイルを生成

    Args:
        config: ジェネレータ設定
        project_root: プロジェクトルート（デフォルト: カレントディレクトリ）
        dry_run: Trueの場合、ファイルを書き込まずに表示のみ
        verbose: 詳細出力
    """
    if project_root is None:
        project_root = Path.cwd()

    paths = config.get("paths", {})
    docker_config = config.get("docker", {})
    # Set default sitecustomize_source if not configured
    if "sitecustomize_source" not in docker_config:
        docker_config["sitecustomize_source"] = "tools/generator/runtime/sitecustomize.py"

    # SAMテンプレートを読み込み
    sam_template_path = project_root / paths.get("sam_template", "template.yaml")
    if not sam_template_path.exists():
        raise FileNotFoundError(f"SAM template not found: {sam_template_path}")

    if verbose:
        print(f"Loading SAM template: {sam_template_path}")

    with open(sam_template_path, encoding="utf-8") as f:
        sam_content = f.read()

    # パラメータ置換設定
    parameters = config.get("parameters", {})

    # パース
    parsed = parse_sam_template(sam_content, parameters)
    functions = parsed["functions"]

    if verbose:
        print(f"Found {len(functions)} function(s)")

    # 出力ディレクトリ
    project_root / paths.get("output_dir", "lambda_functions/")

    # 各関数のDockerfileを生成
    for func in functions:
        func["name"]
        code_uri = func["code_uri"]

        # code_uri からディレクトリ名を決定
        func_dir = project_root / code_uri
        dockerfile_path = func_dir / "Dockerfile"

        # requirements.txt の存在チェック
        requirements_path = func_dir / "requirements.txt"
        func["has_requirements"] = requirements_path.exists()

        # Dockerfileをレンダリング
        dockerfile_content = render_dockerfile(func, docker_config)

        if dry_run:
            print(f"\n=== {dockerfile_path} ===")
            print(dockerfile_content)
        else:
            if verbose:
                print(f"Generating: {dockerfile_path}")
            func_dir.mkdir(parents=True, exist_ok=True)
            with open(dockerfile_path, "w", encoding="utf-8") as f:
                f.write(dockerfile_content)

    # functions.yml を生成
    functions_yml_path = project_root / paths.get("functions_yml", "config/functions.yml")

    functions_yml_content = render_functions_yml(functions)

    if dry_run:
        print(f"\n=== {functions_yml_path} ===")
        print(functions_yml_content)
    else:
        if verbose:
            print(f"Generating: {functions_yml_path}")
        functions_yml_path.parent.mkdir(parents=True, exist_ok=True)
        with open(functions_yml_path, "w", encoding="utf-8") as f:
            f.write(functions_yml_content)

    # routing.yml を生成
    routing_yml_path = project_root / paths.get("routing_yml", "config/routing.yml")
    routing_yml_content = render_routing_yml(functions)

    if dry_run:
        print(f"\n=== {routing_yml_path} ===")
        print(routing_yml_content)
    else:
        if verbose:
            print(f"Generating: {routing_yml_path}")
        routing_yml_path.parent.mkdir(parents=True, exist_ok=True)
        with open(routing_yml_path, "w", encoding="utf-8") as f:
            f.write(routing_yml_content)

    if not dry_run:
        print(f"Generated {len(functions)} Dockerfile(s), functions.yml, and routing.yml")


def main():
    parser = argparse.ArgumentParser(description="Generate local Docker files from SAM template")
    parser.add_argument(
        "--config", default="tools/generator/generator.yml", help="Generator config path"
    )
    parser.add_argument("--template", help="SAM template path (overrides config)")
    parser.add_argument(
        "--dry-run", action="store_true", help="Show what would be generated without writing files"
    )
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")

    args = parser.parse_args()

    # 設定を読み込み
    config_path = Path(args.config)
    config = load_config(config_path)

    # コマンドラインオプションで上書き
    if args.template:
        if "paths" not in config:
            config["paths"] = {}
        config["paths"]["sam_template"] = args.template

    try:
        generate_files(
            config,
            dry_run=args.dry_run,
            verbose=args.verbose,
        )
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
