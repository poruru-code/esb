from tools.cli.config import find_project_root, PROJECT_ROOT


def test_find_project_root():
    """プロジェクトルートが正しく検出されるか確認"""
    root = find_project_root()
    assert (root / "pyproject.toml").exists()
    assert (root / "tools" / "cli").exists()


def test_paths_are_absolute():
    """設定されたパスが絶対パスであることを確認"""
    assert PROJECT_ROOT.is_absolute()
