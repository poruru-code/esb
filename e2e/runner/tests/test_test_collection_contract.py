# Where: e2e/runner/tests/test_test_collection_contract.py
# What: Guards for pytest collection/package boundary expectations.
# Why: Prevent regressions caused by removing required package marker files.
from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parents[3]


def test_required_package_markers_exist() -> None:
    required = [
        "services/gateway/__init__.py",
        "services/gateway/tests/__init__.py",
        "services/common/tests/__init__.py",
        "services/common/core/tests/__init__.py",
    ]
    missing = [path for path in required if not (PROJECT_ROOT / path).exists()]
    assert missing == [], f"required package marker files are missing: {missing}"


def test_scheduler_collection_does_not_import_mismatch() -> None:
    env = os.environ.copy()
    env.setdefault("X_API_KEY", "test-api-key")
    env.setdefault("AUTH_USER", "test-admin")
    env.setdefault("AUTH_PASS", "test-secure-password")

    cmd = [
        sys.executable,
        "-m",
        "pytest",
        "--collect-only",
        "-q",
        "e2e/scenarios/standard/test_scheduler.py",
        "services/gateway/tests/test_scheduler.py",
    ]
    proc = subprocess.run(
        cmd,
        cwd=PROJECT_ROOT,
        env=env,
        capture_output=True,
        text=True,
    )
    assert proc.returncode == 0, (
        "pytest collect-only failed for scheduler test pair.\n"
        f"stdout:\n{proc.stdout}\n"
        f"stderr:\n{proc.stderr}"
    )
