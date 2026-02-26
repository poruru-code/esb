"""Certificate path resolution and validation helpers."""

from __future__ import annotations

from pathlib import Path

from tools.deployops.core.branding import brand_home_dir

REQUIRED_CERT_FILES: tuple[str, ...] = (
    "rootCA.crt",
    "server.crt",
    "server.key",
    "client.crt",
    "client.key",
)


def resolve_cert_dir(*, project_root: Path, project_name: str, override: str | None) -> Path:
    if override and override.strip():
        return Path(override).expanduser().resolve()
    return (project_root / brand_home_dir(project_name) / "certs").resolve()


def ensure_required_certs(cert_dir: Path) -> None:
    missing = [name for name in REQUIRED_CERT_FILES if not (cert_dir / name).is_file()]
    if missing:
        joined = ", ".join(missing)
        raise FileNotFoundError(f"required cert files are missing in {cert_dir}: {joined}")
