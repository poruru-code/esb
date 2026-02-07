#!/usr/bin/env python3
# Where: tools/image-import/import_images.py
# What: Manual helper to sync image sources into the internal registry.
# Why: Allow operators to pre-import image functions outside deploy-time prewarm.
from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path


def run_cmd(args: list[str], dry_run: bool) -> None:
    print("+", " ".join(args))
    if dry_run:
        return
    result = subprocess.run(args, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"command failed ({result.returncode}): {' '.join(args)}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Import images listed in image-import.json")
    parser.add_argument("manifest", type=Path, help="Path to image-import.json")
    parser.add_argument("--dry-run", action="store_true", help="Show commands without executing")
    args = parser.parse_args()

    payload = json.loads(args.manifest.read_text(encoding="utf-8"))
    images = payload.get("images", [])
    if not isinstance(images, list) or not images:
        print("No images to import.")
        return 0

    for item in images:
        source = str(item.get("image_source", "")).strip()
        target = str(item.get("image_ref", "")).strip()
        if not source or not target:
            continue
        run_cmd(["docker", "pull", source], args.dry_run)
        run_cmd(["docker", "tag", source, target], args.dry_run)
        run_cmd(["docker", "push", target], args.dry_run)

    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
