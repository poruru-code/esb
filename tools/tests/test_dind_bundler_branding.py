from __future__ import annotations

import os
import subprocess
from pathlib import Path


def test_dind_bundler_uses_brand_defaults(tmp_path: Path) -> None:
    repo_root = Path(__file__).resolve().parents[2]
    defaults = tmp_path / "defaults.env"
    defaults.write_text("CLI_CMD=acme\nENV_PREFIX=ACME\n", encoding="utf-8")

    env = os.environ.copy()
    env["DEFAULTS_FILE"] = str(defaults)
    env["DIND_BUNDLER_DRYRUN"] = "true"
    env["ACME_ENV"] = "dev"
    env["ACME_OUTPUT_DIR"] = ".acme"
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
    assert "CLI_CMD=acme" in output
    assert "ENV_PREFIX=ACME" in output
    assert "BRAND_SLUG=acme" in output
    assert "OUTPUT_TAG=acme-dind-bundle:latest" in output
    assert "ENV_NAME=dev" in output
    assert "OUTPUT_ROOT=.acme" in output
    assert "MANIFEST_PATH=.acme/dev/bundle/manifest.json" in output
    assert "CERT_DIR=/tmp/acme/certs" in output
