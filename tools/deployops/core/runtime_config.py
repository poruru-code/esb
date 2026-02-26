"""Runtime-config merge helpers for DinD bundle creation."""

from __future__ import annotations

import filecmp
import shutil
from pathlib import Path


def merge_runtime_config_dirs(source_dirs: list[Path], destination: Path) -> None:
    destination.mkdir(parents=True, exist_ok=True)

    for runtime_source in source_dirs:
        src = runtime_source.resolve()
        if not src.is_dir():
            raise FileNotFoundError(f"runtime config source not found: {src}")

        for source_file in sorted(src.rglob("*")):
            if not source_file.is_file():
                continue
            rel = source_file.relative_to(src)
            dest_file = destination / rel
            dest_file.parent.mkdir(parents=True, exist_ok=True)

            if dest_file.is_file():
                if not filecmp.cmp(source_file, dest_file, shallow=False):
                    raise RuntimeError(
                        "runtime-config merge conflict for path "
                        f"'{rel}' (existing={dest_file}, incoming={source_file})"
                    )
                continue

            shutil.copy2(source_file, dest_file)

    _assert_required_files(destination)


def _assert_required_files(runtime_config_dir: Path) -> None:
    for required in ("functions.yml", "routing.yml"):
        target = runtime_config_dir / required
        if not target.is_file():
            raise FileNotFoundError(f"runtime-config/{required} not found after merge")
