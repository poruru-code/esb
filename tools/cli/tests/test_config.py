from tools.cli.config import find_project_root, PROJECT_ROOT


def test_find_project_root():
    """Ensure the project root is detected correctly."""
    root = find_project_root()
    assert (root / "pyproject.toml").exists()
    assert (root / "tools" / "cli").exists()


def test_paths_are_absolute():
    """Ensure configured paths are absolute."""
    assert PROJECT_ROOT.is_absolute()
