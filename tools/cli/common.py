from __future__ import annotations

import os
import subprocess
from collections.abc import Mapping

_PROXY_ALIAS_PAIRS: tuple[tuple[str, str], ...] = (
    ("HTTP_PROXY", "http_proxy"),
    ("HTTPS_PROXY", "https_proxy"),
    ("NO_PROXY", "no_proxy"),
)


def run_command(
    cmd: list[str],
    *,
    cwd: str | None = None,
    env: Mapping[str, str] | None = None,
    capture_output: bool = False,
    quiet_stdout: bool = False,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    if not cmd:
        raise RuntimeError("command is empty")
    stdout = None
    stderr = None
    if capture_output:
        stdout = subprocess.PIPE
        stderr = subprocess.PIPE
    elif quiet_stdout:
        stdout = subprocess.DEVNULL
    completed = subprocess.run(
        cmd,
        cwd=cwd,
        env=dict(os.environ) if env is None else dict(env),
        stdout=stdout,
        stderr=stderr,
        text=True,
        check=False,
    )
    if check and completed.returncode != 0:
        details = (completed.stderr or completed.stdout or "").strip()
        if details:
            raise RuntimeError(f"{' '.join(cmd)} failed: {details}")
        raise RuntimeError(f"{' '.join(cmd)} failed with exit code {completed.returncode}")
    return completed


def docker_image_exists(image_ref: str) -> bool:
    if image_ref.strip() == "":
        return False
    result = subprocess.run(
        ["docker", "image", "inspect", image_ref],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        text=True,
        check=False,
    )
    return result.returncode == 0


def append_proxy_build_args(cmd: list[str], env: Mapping[str, str] | None = None) -> list[str]:
    source = dict(os.environ) if env is None else dict(env)
    out = list(cmd)
    for upper, lower in _PROXY_ALIAS_PAIRS:
        value = (source.get(upper, "") or "").strip()
        if value == "":
            value = (source.get(lower, "") or "").strip()
        if value == "":
            continue
        out.extend(["--build-arg", f"{upper}={value}"])
        out.extend(["--build-arg", f"{lower}={value}"])
    return out


def sorted_unique_non_empty(values: list[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for value in values:
        normalized = value.strip()
        if normalized == "" or normalized in seen:
            continue
        seen.add(normalized)
        out.append(normalized)
    out.sort()
    return out
