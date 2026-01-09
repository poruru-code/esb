# Where: cli/go_cli.py
# What: Go CLI entrypoint shim.
# Why: Route the `esb` command to the Go implementation during migration.
from __future__ import annotations

import subprocess
import sys
from pathlib import Path


def main() -> None:
    root = Path(__file__).resolve().parents[1]
    go_root = root / "cli"
    cmd = ["go", "run", "./cmd/esb", *sys.argv[1:]]

    try:
        result = subprocess.run(cmd, cwd=go_root, check=False)
    except FileNotFoundError as exc:
        print("go executable not found. Install Go or run `mise install`.", file=sys.stderr)
        raise SystemExit(1) from exc

    raise SystemExit(result.returncode)
