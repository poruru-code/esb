from pathlib import Path

from tools.deployops.core.runtime_config import merge_runtime_config_dirs


def test_merge_runtime_config_dirs_merges_identical_files(tmp_path: Path) -> None:
    a = tmp_path / "a"
    b = tmp_path / "b"
    dest = tmp_path / "dest"
    for source in (a, b):
        source.mkdir(parents=True)
        (source / "functions.yml").write_text("functions: {}\n", encoding="utf-8")
        (source / "routing.yml").write_text("routes: []\n", encoding="utf-8")

    merge_runtime_config_dirs([a, b], dest)
    assert (dest / "functions.yml").is_file()
    assert (dest / "routing.yml").is_file()


def test_merge_runtime_config_dirs_detects_conflict(tmp_path: Path) -> None:
    a = tmp_path / "a"
    b = tmp_path / "b"
    dest = tmp_path / "dest"
    a.mkdir(parents=True)
    b.mkdir(parents=True)

    (a / "functions.yml").write_text("functions: {}\n", encoding="utf-8")
    (a / "routing.yml").write_text("routes: []\n", encoding="utf-8")
    (b / "functions.yml").write_text("functions:\n  x: {}\n", encoding="utf-8")
    (b / "routing.yml").write_text("routes: []\n", encoding="utf-8")

    try:
        merge_runtime_config_dirs([a, b], dest)
    except RuntimeError:
        pass
    else:
        raise AssertionError("expected RuntimeError")
