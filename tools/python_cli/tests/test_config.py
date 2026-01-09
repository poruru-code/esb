import os

from tools.python_cli.config import PROJECT_ROOT, find_project_root, get_image_tag


def test_find_project_root():
    """Ensure the project root is detected correctly."""
    root = find_project_root()
    assert (root / "pyproject.toml").exists()
    assert (root / "tools" / "cli").exists()


def test_paths_are_absolute():
    """Ensure configured paths are absolute."""
    assert PROJECT_ROOT.is_absolute()


def test_get_image_tag(monkeypatch):
    """Ensure image tag is resolved correctly."""
    monkeypatch.delenv("ESB_IMAGE_TAG", raising=False)
    # Default without env var should be the env name
    assert get_image_tag("prod") == "prod"

    # With env var should override
    os.environ["ESB_IMAGE_TAG"] = "custom-tag"
    try:
        assert get_image_tag("prod") == "custom-tag"
    finally:
        del os.environ["ESB_IMAGE_TAG"]
