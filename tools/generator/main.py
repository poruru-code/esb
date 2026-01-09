#!/usr/bin/env python3
"""
SAM Template Generator

Generate Dockerfiles and functions.yml for local runs from a SAM template.

Usage:
    python -m tools.generator.main [options]

Options:
    --config PATH       Generator config path (default: tools/generator/generator.yml)
    --template PATH     SAM template path (overrides config)
    --dry-run           Show what would be generated without writing files
    --verbose           Verbose output
"""

import argparse
import os
import shutil
import sys
from pathlib import Path

import yaml

from .parser import parse_sam_template
from .renderer import render_dockerfile, render_functions_yml, render_routing_yml
from .utils import extract_zip_layer, link_or_copy


def load_config(config_path: Path) -> dict:
    """Load the configuration file."""
    if not config_path.exists():
        return {}

    with open(config_path, encoding="utf-8") as f:
        return yaml.safe_load(f) or {}


def generate_files(
    config: dict,
    project_root: Path | None = None,
    dry_run: bool = False,
    verbose: bool = False,
    registry_external: str | None = None,
    registry_internal: str | None = None,
    parameters: dict | None = None,
    tag: str | None = None,
) -> list:
    """
    Generate files from a SAM template.

    Args:
        config: generator config
        project_root: project root (default: current directory)
        dry_run: when True, show output without writing files
        verbose: verbose output
    """
    if project_root is None:
        project_root = Path.cwd()

    paths = config.get("paths", {})
    app_config = config.get("app", {})
    docker_config = config.get("docker", {})

    # Resolve tag: explicit arg > config > default "latest"
    resolved_tag = tag or app_config.get("tag", "latest")
    # Set default sitecustomize_source if not configured
    if "sitecustomize_source" not in docker_config:
        docker_config["sitecustomize_source"] = (
            "cli/internal/generator/assets/site-packages/sitecustomize.py"
        )

    # Load the SAM template.
    # If sam_template is absolute, use it as-is; otherwise resolve from project_root.
    sam_template_path = Path(paths.get("sam_template", "template.yaml"))
    if not sam_template_path.is_absolute():
        sam_template_path = (project_root / sam_template_path).resolve()

    if not sam_template_path.exists():
        raise FileNotFoundError(f"SAM template not found: {sam_template_path}")

    # Use the template's parent directory as the base for resolving relative paths.
    base_dir = sam_template_path.parent

    if verbose:
        print(f"Loading SAM template: {sam_template_path}")
        print(f"Base directory for resolution: {base_dir}")

    with open(sam_template_path, encoding="utf-8") as f:
        sam_content = f.read()

    # Parameter substitution settings.
    # Merge parameters from config and function arguments.
    gen_params = config.get("parameters", {})
    if parameters:
        gen_params.update(parameters)

    # Parse.
    parsed = parse_sam_template(sam_content, gen_params)
    functions = parsed["functions"]

    if verbose:
        print(f"Found {len(functions)} function(s)")

    # Output directory (relative to base_dir).
    output_dir_raw = Path(paths.get("output_dir", ".esb/"))
    if not output_dir_raw.is_absolute():
        output_dir = (base_dir / output_dir_raw).resolve()
    else:
        output_dir = output_dir_raw

    functions_staging_dir = output_dir / "functions"
    layers_staging_dir = output_dir / "layers"

    if not dry_run and functions_staging_dir.exists():
        if verbose:
            print(f"Cleaning up staging directory: {functions_staging_dir}")
        shutil.rmtree(functions_staging_dir)

    # Initialize layers cache directory
    layers_cache_dir = output_dir / ".layers_cache"
    if not dry_run:
        layers_cache_dir.mkdir(parents=True, exist_ok=True)
    if not dry_run and layers_staging_dir.exists():
        if verbose:
            print(f"Cleaning up layer staging directory: {layers_staging_dir}")
        shutil.rmtree(layers_staging_dir)

    def _resolve_resource_path(p: str) -> Path:
        """Resolve a relative path from the template file."""
        # Strip leading slash and resolve relative to the template (base_dir).
        path_str = p.lstrip("/")
        target = (base_dir / path_str).resolve()
        if not target.exists():
            if verbose:
                print(f"WARNING: Resource not found at: {target}")
        return target

    # Generate Dockerfiles for each function.
    for func in functions:
        func_name = func["name"]
        code_uri = func["code_uri"]

        # 1. Prepare staging directory (<output_dir>/functions/<func_name>).
        dockerfile_dir = functions_staging_dir / func_name
        dockerfile_dir.mkdir(parents=True, exist_ok=True)

        # 2. Copy source code (within staging).
        func_src_dir = _resolve_resource_path(code_uri)
        staging_src_dir = dockerfile_dir / "src"
        if func_src_dir.exists() and func_src_dir.is_dir():
            shutil.copytree(func_src_dir, staging_src_dir, dirs_exist_ok=True)

        # Pass relative staging paths to the renderer.
        func["code_uri"] = f"functions/{func_name}/src/"
        func["dockerfile_path"] = str(dockerfile_dir / "Dockerfile")
        func["context_path"] = str(output_dir)

        # 3. Copy layers (Hard Link Strategy)
        new_layers = []
        for layer in func.get("layers", []):
            layer_copy = layer.copy()
            content_uri = layer_copy.get("content_uri", "")
            if not content_uri:
                continue

            layer_src = _resolve_resource_path(content_uri)

            if layer_src.exists():
                # Target dir name in Docker build context
                # Use stem if zip (remove .zip extension)
                target_name = layer_src.stem if layer_src.suffix == ".zip" else layer_src.name

                # Function-specific layer directory
                layers_dir = dockerfile_dir / "layers"
                dest_layer_root = layers_dir / target_name

                # Clean up if exists
                if dest_layer_root.exists():
                    if dest_layer_root.is_dir():
                        shutil.rmtree(dest_layer_root)
                    else:
                        os.unlink(dest_layer_root)

                final_src_dir = None

                if layer_src.is_file() and layer_src.suffix == ".zip":
                    # Extract to cache
                    if verbose:
                        print(f"Processing Zip layer: {layer_src}")
                    final_src_dir = extract_zip_layer(layer_src, layers_cache_dir)

                elif layer_src.is_dir():
                    final_src_dir = layer_src

                else:
                    if verbose:
                        print(f"WARNING: Skipping unsupported layer type: {layer_src}")
                    continue

                if final_src_dir:
                    # Link/Copy contents to staging
                    # Special handling: If source directory is named 'python', nest it
                    # so that it copies as /opt/python/...
                    final_dest = dest_layer_root
                    if layer_src.is_dir() and layer_src.name == "python":
                        final_dest = dest_layer_root / "python"

                    final_dest.mkdir(parents=True, exist_ok=True)
                    shutil.copytree(
                        final_src_dir,
                        final_dest,
                        dirs_exist_ok=True,
                        copy_function=link_or_copy,
                    )

                    # Update ContentUri for template (relative to build context root)
                    # We use 'functions/<func>/layers/<name>' which is a directory.
                    # Template will append '/' to copy contents.
                    layer_copy["content_uri"] = f"functions/{func_name}/layers/{target_name}"
                    new_layers.append(layer_copy)

        # Layer list rewritten to local paths for this function.
        func["layers"] = new_layers

        # 4. Copy sitecustomize.py.
        # Try resolving sitecustomize_source relative to base_dir, otherwise project_root.
        site_path_raw = Path(docker_config.get("sitecustomize_source"))
        if not site_path_raw.is_absolute():
            site_src = (base_dir / site_path_raw).resolve()
            if not site_src.exists():
                # Fallback: project root (e.g., defaults within generator package).
                site_src = (project_root / site_path_raw).resolve()
        else:
            site_src = site_path_raw

        if verbose:
            print(f"DEBUG: site_src={site_src}, exists={site_src.exists()}")
        if site_src.exists():
            # Use link_or_copy for sitecustomize too
            link_or_copy(site_src, dockerfile_dir / "sitecustomize.py")
        else:
            if verbose:
                print(f"WARNING: sitecustomize.py not found at {site_src}")
        # Override to reference from the build context root.
        docker_config_copy = docker_config.copy()
        docker_config_copy["sitecustomize_source"] = f"functions/{func_name}/sitecustomize.py"

        # Check for requirements.txt (within context).
        func["has_requirements"] = (staging_src_dir / "requirements.txt").exists()

        # Render Dockerfile.
        dockerfile_content = render_dockerfile(
            func, docker_config_copy, registry=registry_external, tag=resolved_tag
        )

        if dry_run:
            print(f"\n📄 [DryRun] Staging: {dockerfile_dir} (Source: {func_src_dir})")
            print("-" * 60)
            print(dockerfile_content.strip())
            print("-" * 60)
        else:
            if verbose:
                print(f"Staging build files: {dockerfile_dir}")
            dockerfile_path = dockerfile_dir / "Dockerfile"
            with open(dockerfile_path, "w", encoding="utf-8") as f:
                f.write(dockerfile_content)

    # Generate functions.yml (relative to base_dir, default under output_dir/config/).
    functions_yml_raw = paths.get("functions_yml")
    if functions_yml_raw:
        functions_yml_path_raw = Path(functions_yml_raw)
        if not functions_yml_path_raw.is_absolute():
            functions_yml_path = (base_dir / functions_yml_path_raw).resolve()
        else:
            functions_yml_path = functions_yml_path_raw
    else:
        # Default convention: output_dir/config/functions.yml.
        functions_yml_path = output_dir / "config" / "functions.yml"

    def _ensure_safe_write(target_path: Path, content: str):
        """Write content to file, ensuring target is not a directory (Docker clobber fix)."""
        if verbose:
            print(f"Generating: {target_path}")
        target_path.parent.mkdir(parents=True, exist_ok=True)
        # Fix for Docker behavior: if target is a directory, remove it.
        if target_path.exists() and not target_path.is_file():
            import shutil

            shutil.rmtree(target_path)
        with open(target_path, "w", encoding="utf-8") as f:
            f.write(content)

    functions_yml_content = render_functions_yml(
        functions, registry=registry_internal, tag=resolved_tag
    )

    # Also render routing.yml

    if dry_run:
        print(f"\n📄 [DryRun] Target: {functions_yml_path}")
        print("-" * 60)
        print(functions_yml_content.strip())
        print("-" * 60)
    else:
        _ensure_safe_write(functions_yml_path, functions_yml_content)

    # Generate routing.yml (relative to base_dir, default under output_dir/config/).
    routing_yml_raw = paths.get("routing_yml")
    if routing_yml_raw:
        routing_yml_path_raw = Path(routing_yml_raw)
        if not routing_yml_path_raw.is_absolute():
            routing_yml_path = (base_dir / routing_yml_path_raw).resolve()
        else:
            routing_yml_path = routing_yml_path_raw
    else:
        # Default convention: output_dir/config/routing.yml.
        routing_yml_path = output_dir / "config" / "routing.yml"

    routing_yml_content = render_routing_yml(functions)

    if dry_run:
        print(f"\n📄 [DryRun] Target: {routing_yml_path}")
        print("-" * 60)
        print(routing_yml_content.strip())
        print("-" * 60)
    else:
        _ensure_safe_write(routing_yml_path, routing_yml_content)

    if not dry_run:
        print(f"Generated {len(functions)} Dockerfile(s), functions.yml, and routing.yml")

    return functions


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

    # Load configuration.
    config_path = Path(args.config)
    config = load_config(config_path)

    # Override with command-line options.
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
