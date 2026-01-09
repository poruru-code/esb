import os
import zipfile
from unittest.mock import patch

import pytest

from tools.generator import utils


@pytest.fixture
def temp_workspace(tmp_path):
    """Create a temporary workspace for file operations."""
    return tmp_path


def test_link_or_copy_links_successfully(temp_workspace):
    """Test that link_or_copy uses os.link when possible."""
    src = temp_workspace / "src.txt"
    src.write_text("content")
    dst = temp_workspace / "dst.txt"

    utils.link_or_copy(src, dst)

    assert dst.exists()
    assert dst.read_text() == "content"
    # Verify it is a hard link (same inode) on POSIX
    if os.name == "posix":
        assert src.stat().st_ino == dst.stat().st_ino


def test_link_or_copy_fallback_to_copy(temp_workspace):
    """Test fallback to shutil.copy2 when os.link raises OSError."""
    src = temp_workspace / "src.txt"
    src.write_text("content")
    dst = temp_workspace / "dst.txt"

    with patch("os.link", side_effect=OSError("Mock link error")):
        with patch("shutil.copy2") as mock_copy:
            utils.link_or_copy(src, dst)
            mock_copy.assert_called_once()


def test_link_or_copy_overwrites_destination(temp_workspace):
    """Test that existing destination is overwritten."""
    src = temp_workspace / "src.txt"
    src.write_text("new content")
    dst = temp_workspace / "dst.txt"
    dst.write_text("old content")

    utils.link_or_copy(src, dst)

    assert dst.read_text() == "new content"


def test_extract_zip_layer_caching(temp_workspace):
    """Test that extract_zip_layer uses cache."""
    cache_dir = temp_workspace / "cache"
    cache_dir.mkdir()

    # Create valid zip
    zip_path = temp_workspace / "layer.zip"
    with zipfile.ZipFile(zip_path, "w") as zf:
        zf.writestr("hello.txt", "world")

    # First extraction
    extracted_path = utils.extract_zip_layer(zip_path, cache_dir)
    assert extracted_path.exists()
    assert (extracted_path / "hello.txt").read_text() == "world"

    # Verify cache directory naming (stem + time + size)
    stat = zip_path.stat()
    expected_name = f"layer_{int(stat.st_mtime)}_{stat.st_size}"
    assert extracted_path.name == expected_name

    # Second call should return same path without re-extracting
    # We can mock ZipFile to ensure it's not called again
    with patch("zipfile.ZipFile") as mock_zip:
        cached_path = utils.extract_zip_layer(zip_path, cache_dir)
        assert cached_path == extracted_path
        mock_zip.assert_not_called()
