#!/usr/bin/env python3
# Where: services/contracts/scripts/fix_python_grpc_imports.py
# What: Rewrites grpc Python stub imports to package-relative form.
# Why: grpc plugin emits absolute imports (import *_pb2) and breaks
# package usage in services/gateway.
from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
PB_DIR = ROOT / "services" / "gateway" / "pb"
IMPORT_RE = re.compile(r"^import (\w+_pb2) as (\w+)$", flags=re.MULTILINE)


def rewrite_file(path: Path) -> bool:
    original = path.read_text(encoding="utf-8")
    rewritten = IMPORT_RE.sub(r"from . import \1 as \2", original)
    if rewritten == original:
        return False
    path.write_text(rewritten, encoding="utf-8")
    return True


def main() -> int:
    PB_DIR.mkdir(parents=True, exist_ok=True)
    init_file = PB_DIR / "__init__.py"
    if not init_file.exists():
        init_file.write_text("", encoding="utf-8")

    changed: list[Path] = []
    for path in sorted(PB_DIR.glob("*_pb2_grpc.py")):
        if rewrite_file(path):
            changed.append(path)

    if changed:
        print("Rewrote imports:")
        for path in changed:
            print(f"- {path.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
