"""
Where: tools/tests/test_sbom_support.py
What: Regression tests for CycloneDX SBOM helper command construction.
Why: Prevent unsupported `uv export` options from breaking strict CI SBOM generation.
"""

import sys
from importlib import util
from pathlib import Path

MODULE_PATH = Path(__file__).resolve().parents[2] / "tools" / "ci" / "sbom_support.py"
SPEC = util.spec_from_file_location("sbom_support", MODULE_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("Failed to load sbom_support module for tests.")
sbom_support = util.module_from_spec(SPEC)
sys.modules[SPEC.name] = sbom_support
SPEC.loader.exec_module(sbom_support)


def test_build_python_export_command_uses_supported_uv_options() -> None:
    project_dir = Path("/tmp/project")
    output_file = Path("/tmp/python-project.cdx.json")

    command = sbom_support.build_python_export_command(project_dir, output_file)

    assert command == [
        "uv",
        "export",
        "--project",
        str(project_dir),
        "--format",
        "cyclonedx1.5",
        "--frozen",
        "--no-dev",
        "--output-file",
        str(output_file),
    ]
    assert "--preview-features" not in command
    assert "--preview" not in command
