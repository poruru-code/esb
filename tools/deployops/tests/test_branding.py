from tools.deployops.core import branding


def test_resolve_brand_slug_defaults_and_sanitizes() -> None:
    assert branding.resolve_brand_slug(None) == branding.DEFAULT_BRAND_SLUG
    assert branding.resolve_brand_slug("Acme Prod") == "acme-prod"


def test_resolve_compose_project_name_normalizes_value() -> None:
    assert branding.resolve_compose_project_name("Acme", "Dev") == "acme-dev"


def test_resolve_compose_project_name_rejects_empty_result() -> None:
    try:
        branding.resolve_compose_project_name("___", None)
    except ValueError:
        pass
    else:
        raise AssertionError("expected ValueError")
