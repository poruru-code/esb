from __future__ import annotations

import os
import subprocess
from pathlib import Path


def test_dind_bundler_uses_brand_defaults(tmp_path: Path) -> None:
    repo_root = Path(__file__).resolve().parents[2]

    env = os.environ.copy()
    env["DIND_BUNDLER_DRYRUN"] = "true"
    env["ESB_ENV"] = "dev"
    env["ESB_OUTPUT_DIR"] = ".acme"
    env["CERT_DIR"] = "/tmp/acme/certs"

    result = subprocess.run(
        ["bash", "tools/dind-bundler/build.sh", "template.yaml"],
        cwd=repo_root,
        env=env,
        check=True,
        capture_output=True,
        text=True,
    )

    output = result.stdout
    assert "ESB_CMD=esb" in output
    assert "ENV_PREFIX=ESB" in output
    assert "BRAND_SLUG=esb" in output
    assert "TEMPLATES=template.yaml" in output
    assert "OUTPUT_TAG=esb-dind-bundle:latest" in output
    assert "ENV_NAME=dev" in output
    assert "OUTPUT_ROOTS=.acme" in output
    assert "MANIFEST_PATHS=.acme/dev/bundle/manifest.json" in output
    assert "CERT_DIR=/tmp/acme/certs" in output
