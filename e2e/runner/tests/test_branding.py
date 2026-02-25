# Where: e2e/runner/tests/test_branding.py
# What: Unit tests for shared branding helper functions.
# Why: Keep brand/path derivation centralized and stable across runner modules.
from __future__ import annotations

from pathlib import Path

from e2e.runner import branding


def test_resolve_project_name_defaults_and_preserves_explicit_value() -> None:
    assert branding.resolve_project_name(None) == "esb"
    assert branding.resolve_project_name("  ") == "esb"
    assert branding.resolve_project_name("acme") == "acme"


def test_resolve_brand_slug_sanitizes_project_name() -> None:
    assert branding.resolve_brand_slug("Acme Prod") == "acme-prod"
    assert branding.resolve_brand_slug("___") == "esb"


def test_path_and_name_helpers_follow_resolved_slug(tmp_path: Path) -> None:
    assert branding.brand_home_dir("Acme Prod") == ".acme-prod"
    assert branding.lambda_network_name("Acme Prod", "e2e-x") == "acme-prod_int_e2e-x"
    assert branding.root_ca_mount_id("Acme Prod") == "acme-prod_root_ca"
    assert branding.buildx_builder_name("Acme Prod") == "acme-prod-buildx"
    assert branding.infra_registry_container_name("Acme Prod") == "acme-prod-infra-registry"

    assert branding.cert_dir(tmp_path, "Acme Prod") == tmp_path / ".acme-prod" / "certs"
    assert branding.buildkitd_config_path(tmp_path, "Acme Prod") == (
        tmp_path / ".acme-prod" / "buildkitd.toml"
    )
