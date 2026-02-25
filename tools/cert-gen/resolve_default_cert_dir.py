#!/usr/bin/env python3
"""Resolve default CERT_DIR from shared branding contract."""

from pathlib import Path

from e2e.runner.branding import cert_dir


def main() -> int:
    print(cert_dir(Path.cwd()))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
