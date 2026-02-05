#!/usr/bin/env python3
# Where: tools/dind-bundler/merge_manifest.py
# What: Merge multiple bundle manifests into a single manifest.
# Why: Support multi-template bundling with a unified source of truth.

from __future__ import annotations

import argparse
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

REQUIRED_BUILD_KEYS = ["project", "env", "mode", "image_prefix", "image_tag"]


def load_manifest(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as handle:
        data = json.load(handle)
    if not isinstance(data, dict):
        raise ValueError(f"invalid manifest: {path}")
    return data


def extract_templates(data: dict[str, Any], path: Path) -> list[dict[str, Any]]:
    templates = data.get("templates")
    if isinstance(templates, list) and templates:
        return templates
    template = data.get("template")
    if isinstance(template, dict) and template:
        return [template]
    raise ValueError(f"manifest missing template info: {path}")


def merge_manifests(paths: list[Path]) -> dict[str, Any]:
    merged_templates: list[dict[str, Any]] = []
    template_keys: set[tuple[str, str, str]] = set()
    images: dict[str, dict[str, Any]] = {}
    base_build: dict[str, Any] | None = None

    for path in paths:
        data = load_manifest(path)
        build = data.get("build")
        if not isinstance(build, dict):
            raise ValueError(f"manifest missing build metadata: {path}")
        if base_build is None:
            base_build = build
        else:
            for key in REQUIRED_BUILD_KEYS:
                if build.get(key) != base_build.get(key):
                    current = build.get(key)
                    expected = base_build.get(key)
                    raise ValueError(f"build metadata mismatch ({key}): {current} != {expected}")

        for tpl in extract_templates(data, path):
            tpl_path = str(tpl.get("path", ""))
            tpl_hash = str(tpl.get("sha256", ""))
            params = tpl.get("parameters")
            if not isinstance(params, dict):
                params = {}
            params_key = json.dumps(params, sort_keys=True, ensure_ascii=True)
            key = (tpl_path, tpl_hash, params_key)
            if key in template_keys:
                continue
            template_keys.add(key)
            merged_templates.append(
                {
                    "path": tpl_path,
                    "sha256": tpl_hash,
                    "parameters": params,
                }
            )

        for img in data.get("images", []):
            if not isinstance(img, dict):
                continue
            name = str(img.get("name", ""))
            if not name:
                continue
            digest = str(img.get("digest", ""))
            if name in images:
                if digest and images[name].get("digest") != digest:
                    raise ValueError(f"image digest mismatch for {name}")
                continue
            images[name] = img

    if base_build is None:
        raise ValueError("no manifests to merge")

    merged_images = sorted(images.values(), key=lambda item: str(item.get("name", "")))
    if not merged_templates:
        raise ValueError("no templates found in manifests")

    generated_at = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    return {
        "schema_version": "1.1",
        "generated_at": generated_at,
        "templates": merged_templates,
        "template": merged_templates[0],
        "build": base_build,
        "images": merged_images,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="Merge bundle manifests")
    parser.add_argument("--output", required=True, help="Output manifest path")
    parser.add_argument("manifests", nargs="+", help="Input manifest paths")
    args = parser.parse_args()

    paths = [Path(p) for p in args.manifests]
    merged = merge_manifests(paths)

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", encoding="utf-8") as handle:
        json.dump(merged, handle, indent=2, ensure_ascii=True)
        handle.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
