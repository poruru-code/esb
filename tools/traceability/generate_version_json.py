#!/usr/bin/env python3
# Where: tools/traceability/generate_version_json.py
# What: Generate version metadata JSON for Docker build traceability.
# Why: Encode git-derived version info without relying on runtime env vars.

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import re
import subprocess
import sys
from typing import Iterable
from urllib.parse import urlsplit, urlunsplit

ALLOWED_COMPONENTS = {
    "gateway",
    "agent",
    "runtime-node",
    "provisioner",
    "base",
    "function",
}
ALLOWED_IMAGE_RUNTIMES = {"docker", "containerd", "shared"}


def run_git(args: Iterable[str], env: dict[str, str], allow_fail: bool = False) -> str:
    result = subprocess.run(
        ["git", *args],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        env=env,
        check=False,
    )
    stdout = result.stdout.strip()
    stderr = result.stderr.strip()
    if result.returncode != 0:
        if allow_fail:
            return ""
        msg = stderr or stdout or "git command failed"
        raise RuntimeError(f"git {' '.join(args)} failed: {msg}")
    if not stdout and not allow_fail:
        raise RuntimeError(f"git {' '.join(args)} returned empty output")
    return stdout


def strip_userinfo(url: str) -> str:
    parsed = urlsplit(url)
    if parsed.scheme and parsed.netloc:
        host = parsed.hostname or ""
        if parsed.port:
            host = f"{host}:{parsed.port}"
        return urlunsplit((parsed.scheme, host, parsed.path, parsed.query, parsed.fragment))
    if "@" in url and ":" in url.split("@", 1)[1]:
        return url.split("@", 1)[1]
    return url


def sanitize_repo_url(value: str) -> str:
    if not value:
        return ""
    trimmed = value.strip()
    if not trimmed:
        return ""
    if re.search(r"[\x00-\x1F\x7F]", trimmed):
        return ""
    return strip_userinfo(trimmed)


def build_date_rfc3339() -> str:
    now = dt.datetime.now(dt.timezone.utc).replace(microsecond=0)
    return now.isoformat().replace("+00:00", "Z")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate version.json for build traceability")
    parser.add_argument("--git-dir", required=True)
    parser.add_argument("--git-common-dir", required=True)
    parser.add_argument("--component", required=True)
    parser.add_argument("--image-runtime", required=True)
    parser.add_argument("--output", required=True)
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    component = args.component.strip()
    image_runtime = args.image_runtime.strip()
    if component not in ALLOWED_COMPONENTS:
        raise RuntimeError(f"invalid component: {component}")
    if image_runtime not in ALLOWED_IMAGE_RUNTIMES:
        raise RuntimeError(f"invalid image_runtime: {image_runtime}")

    env = os.environ.copy()
    env["GIT_DIR"] = args.git_dir
    env["GIT_COMMON_DIR"] = args.git_common_dir

    git_sha = run_git(["rev-parse", "HEAD"], env)
    git_sha_short = run_git(["rev-parse", "--short=12", "HEAD"], env)

    version = run_git(["describe", "--tags", "--always"], env, allow_fail=True)
    if not version:
        version = f"0.0.0-dev.{git_sha_short}"

    repo_url_raw = run_git(["config", "--get", "remote.origin.url"], env, allow_fail=True)
    repo_url = sanitize_repo_url(repo_url_raw)

    output_path = args.output
    output_dir = os.path.dirname(output_path)
    if output_dir:
        os.makedirs(output_dir, exist_ok=True)

    payload = {
        "version": version,
        "git_sha": git_sha,
        "git_sha_short": git_sha_short,
        "build_date": build_date_rfc3339(),
        "repo_url": repo_url,
        "source": "git",
        "component": component,
        "image_runtime": image_runtime,
    }

    with open(output_path, "w", encoding="utf-8") as handle:
        json.dump(payload, handle, ensure_ascii=True)
        handle.write("\n")

    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except Exception as exc:  # pylint: disable=broad-except
        print(f"ERROR: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
