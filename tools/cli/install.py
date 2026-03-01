#!/usr/bin/env python3
from __future__ import annotations

from pathlib import Path

from e2e.runner.branding_constants_gen import DEFAULT_CTL_BIN


def main() -> int:
    repo_root = Path(__file__).resolve().parents[2]
    target_dir = Path.home() / ".local" / "bin"
    target_dir.mkdir(parents=True, exist_ok=True)
    target_path = target_dir / DEFAULT_CTL_BIN

    wrapper = "\n".join(
        (
            "#!/usr/bin/env bash",
            "set -euo pipefail",
            f"cd {str(repo_root)!r}",
            'exec uv run python -m tools.cli.cli "$@"',
            "",
        )
    )
    target_path.write_text(wrapper, encoding="utf-8")
    target_path.chmod(0o755)
    print(f"[build-ctl] installed {DEFAULT_CTL_BIN} to {target_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
