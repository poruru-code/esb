from __future__ import annotations

from pathlib import Path

FORBIDDEN_TOKENS = (
    "tools." + "artifact" + "ctl",
    "tools/" + "artifact" + "ctl",
    "pkg/" + "deployops",
    "pkg/" + "artifactcore",
    "github.com/poruru-code/esb/" + "pkg/",
)


def test_cli_runtime_has_no_pkg_or_artifactctl_dependency() -> None:
    cli_root = Path(__file__).resolve().parents[1]
    runtime_files = sorted(
        path for path in cli_root.rglob("*.py") if "/tests/" not in str(path).replace("\\", "/")
    )
    assert runtime_files, "expected runtime python files under tools/cli"

    violations: list[str] = []
    for path in runtime_files:
        content = path.read_text(encoding="utf-8")
        for token in FORBIDDEN_TOKENS:
            if token in content:
                rel = path.relative_to(cli_root)
                violations.append(f"{rel}: contains {token!r}")

    assert not violations, "forbidden dependencies found:\n" + "\n".join(violations)
