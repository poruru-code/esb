from __future__ import annotations

import hashlib
import json
import os
import tempfile
import time
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from pathlib import Path

from tools.cli.common import append_proxy_build_args, docker_image_exists, run_command

_MAVEN_SHIM_IMAGE_PREFIX = "esb-maven-shim"
_MAVEN_SHIM_TAG_SCHEMA = "v2"
_LOCK_ACQUIRE_TIMEOUT_SECONDS = 120.0
_LOCK_RETRY_INTERVAL_SECONDS = 0.2
_STALE_LOCK_SECONDS = 300.0
_ASSET_FILE_NAMES: tuple[str, ...] = ("Dockerfile", "mvn-wrapper.sh")


@dataclass(frozen=True)
class EnsureInput:
    base_image: str
    host_registry: str = ""
    no_cache: bool = False
    env: Mapping[str, str] | None = None


@dataclass(frozen=True)
class EnsureResult:
    shim_image: str


def _assets_dir() -> Path:
    # Keep runtime assets colocated with the Python implementation so pkg can be retired.
    return Path(__file__).resolve().parent / "assets" / "mavenshim"


def _asset_fingerprint() -> str:
    digest = hashlib.sha256()
    assets_dir = _assets_dir()
    for name in _ASSET_FILE_NAMES:
        content = (assets_dir / name).read_bytes()
        digest.update(name.encode("utf-8"))
        digest.update(b"\x00")
        digest.update(content)
        digest.update(b"\x00")
    return digest.hexdigest()


def derive_shim_image_tag(base_ref: str) -> str:
    hash_input = "\n".join((_MAVEN_SHIM_TAG_SCHEMA, base_ref, _asset_fingerprint()))
    short_hash = hashlib.sha256(hash_input.encode("utf-8")).hexdigest()[:16]
    return f"{_MAVEN_SHIM_IMAGE_PREFIX}:{short_hash}"


def _shim_lock_path(shim_ref: str) -> Path:
    digest = hashlib.sha256(shim_ref.encode("utf-8")).hexdigest()[:16]
    return Path(tempfile.gettempdir()) / f"esb-maven-shim-{digest}.lock"


def _acquire_lock(lock_path: Path) -> Callable[[], None]:
    started = time.monotonic()
    while True:
        try:
            fd = os.open(str(lock_path), os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        except FileExistsError as exc:
            _evict_stale_lock(lock_path)
            if time.monotonic() - started >= _LOCK_ACQUIRE_TIMEOUT_SECONDS:
                raise RuntimeError(f"timeout acquiring maven shim lock: {lock_path}") from exc
            time.sleep(_LOCK_RETRY_INTERVAL_SECONDS)
            continue
        metadata = {
            "pid": os.getpid(),
            "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        }
        with os.fdopen(fd, "w", encoding="utf-8") as file:
            json.dump(metadata, file)
            file.write("\n")

        def _release() -> None:
            try:
                lock_path.unlink()
            except FileNotFoundError:
                pass

        return _release


def _evict_stale_lock(lock_path: Path) -> None:
    try:
        stat = lock_path.stat()
    except FileNotFoundError:
        return
    if time.time() - stat.st_mtime < _STALE_LOCK_SECONDS:
        return
    try:
        lock_path.unlink()
    except FileNotFoundError:
        return


def _materialize_build_context() -> tuple[Path, Callable[[], None]]:
    context_dir = Path(tempfile.mkdtemp(prefix="esb-maven-shim-"))
    assets_dir = _assets_dir()
    for name in _ASSET_FILE_NAMES:
        src = assets_dir / name
        dst = context_dir / name
        dst.write_bytes(src.read_bytes())
        dst.chmod(0o755 if name.endswith(".sh") else 0o644)

    def _cleanup() -> None:
        for child in sorted(context_dir.glob("**/*"), reverse=True):
            if child.is_file() or child.is_symlink():
                child.unlink(missing_ok=True)
            elif child.is_dir():
                child.rmdir()
        context_dir.rmdir()

    return context_dir, _cleanup


def _buildx_command(
    tag: str,
    dockerfile: Path,
    context_dir: Path,
    *,
    no_cache: bool,
    build_args: dict[str, str],
    env: Mapping[str, str] | None,
) -> list[str]:
    cmd = ["docker", "buildx", "build", "--platform", "linux/amd64", "--load", "--pull"]
    if no_cache:
        cmd.append("--no-cache")
    cmd = append_proxy_build_args(cmd, env=env)
    for key in sorted(build_args.keys()):
        value = build_args[key].strip()
        if value == "":
            continue
        cmd.extend(["--build-arg", f"{key}={value}"])
    cmd.extend(["--tag", tag, "--file", str(dockerfile), str(context_dir)])
    return cmd


def ensure_image(input_data: EnsureInput) -> EnsureResult:
    base_ref = input_data.base_image.strip()
    if base_ref == "":
        raise RuntimeError("maven base image reference is empty")

    host_registry = input_data.host_registry.strip().rstrip("/")
    shim_image = derive_shim_image_tag(base_ref)
    shim_ref = shim_image if host_registry == "" else f"{host_registry}/{shim_image}"

    lock_path = _shim_lock_path(shim_ref)
    release = _acquire_lock(lock_path)
    try:
        if input_data.no_cache or not docker_image_exists(shim_ref):
            context_dir, cleanup = _materialize_build_context()
            try:
                build_cmd = _buildx_command(
                    shim_ref,
                    context_dir / "Dockerfile",
                    context_dir,
                    no_cache=input_data.no_cache,
                    build_args={"BASE_MAVEN_IMAGE": base_ref},
                    env=input_data.env,
                )
                run_command(build_cmd, check=True, env=input_data.env)
            finally:
                cleanup()

        if host_registry != "":
            run_command(["docker", "push", shim_ref], check=True, env=input_data.env)
        return EnsureResult(shim_image=shim_ref)
    finally:
        release()
