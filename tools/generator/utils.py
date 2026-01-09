import logging
import os
import shutil
import zipfile
from pathlib import Path

logger = logging.getLogger(__name__)


def link_or_copy(src, dst, *, follow_symlinks=True):
    """
    Link file from src to dst. Fallback to copy if link fails.
    Intended for use with shutil.copytree(copy_function=link_or_copy).
    """
    # If dst exists, remove it (standard copy behavior overwrite)
    try:
        if os.path.exists(dst):
            os.unlink(dst)
    except OSError:
        pass

    try:
        os.link(src, dst)
    except OSError:
        # Fallback to copy
        shutil.copy2(src, dst, follow_symlinks=follow_symlinks)


def extract_zip_layer(zip_path: Path, cache_dir: Path) -> Path:
    """
    Extract zip layer to a persistent cache directory.
    Returns the path to the extracted directory.
    Identifier is based on filename + mtime + size.
    """
    try:
        stat = zip_path.stat()
    except FileNotFoundError:
        # Fallback if file missing (should be checked by caller)
        return cache_dir / "missing"

    # Create unique identifier
    identifier = f"{zip_path.stem}_{int(stat.st_mtime)}_{stat.st_size}"
    extract_dst = cache_dir / identifier

    if extract_dst.exists():
        return extract_dst

    # Temporary extraction
    tmp_extract = extract_dst.with_suffix(".tmp")
    if tmp_extract.exists():
        shutil.rmtree(tmp_extract)
    tmp_extract.mkdir(parents=True, exist_ok=True)

    print(f"Unzipping layer (caching): {zip_path} -> {extract_dst}")
    try:
        with zipfile.ZipFile(zip_path, "r") as zf:
            zf.extractall(tmp_extract)

        # Atomic rename (move)
        tmp_extract.rename(extract_dst)
    except Exception as e:
        if tmp_extract.exists():
            shutil.rmtree(tmp_extract)
        raise e

    return extract_dst
