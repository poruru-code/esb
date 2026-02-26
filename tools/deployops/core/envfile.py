"""Environment file parsing and bundle normalization helpers."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class EnvLine:
    raw: str
    key: str | None = None
    value: str | None = None


def parse_env_lines(path: Path) -> list[EnvLine]:
    lines: list[EnvLine] = []
    for raw in Path(path).read_text(encoding="utf-8").splitlines():
        stripped = raw.strip()
        if stripped == "" or stripped.startswith("#") or "=" not in raw:
            lines.append(EnvLine(raw=raw))
            continue
        key, value = raw.split("=", 1)
        key = key.strip()
        if key == "":
            lines.append(EnvLine(raw=raw))
            continue
        lines.append(EnvLine(raw=raw, key=key, value=value.strip()))
    return lines


def load_env_file(path: Path) -> dict[str, str]:
    env: dict[str, str] = {}
    if not Path(path).is_file():
        return env
    for line in parse_env_lines(path):
        if line.key is None:
            continue
        assert line.value is not None
        env[line.key] = _unquote(line.value)
    return env


def read_env_file_value(path: Path, key: str) -> str:
    env = load_env_file(path)
    return env.get(key, "")


def normalize_bundle_env(path: Path, *, brand_home: str) -> None:
    """Apply deterministic defaults for bundled DinD `.env` files."""

    credential_defaults = {
        "AUTH_USER": "test-admin",
        "AUTH_PASS": "test-secure-password",
        "JWT_SECRET_KEY": "test-secret-key-must-be-at-least-32-chars",
        "X_API_KEY": "test-api-key",
        "RUSTFS_ACCESS_KEY": "rustfsadmin",
        "RUSTFS_SECRET_KEY": "rustfsadmin",
    }

    port_defaults = {
        "PORT_GATEWAY_HTTPS": "8443",
        "PORT_VICTORIALOGS": "9428",
        "PORT_AGENT_METRICS": "9091",
        "PORT_S3": "9000",
        "PORT_S3_MGMT": "9001",
        "PORT_DATABASE": "8000",
        "PORT_REGISTRY": "5010",
    }

    forced_values = {
        "CERT_DIR": f"/root/{brand_home}/certs",
    }

    lines = parse_env_lines(path)
    rendered = [line.raw for line in lines]

    index_by_key: dict[str, int] = {}
    value_by_key: dict[str, str] = {}

    for idx, line in enumerate(lines):
        if line.key is None:
            continue
        assert line.value is not None
        index_by_key[line.key] = idx
        value_by_key[line.key] = _unquote(line.value)

    def upsert(key: str, value: str) -> None:
        if key in index_by_key:
            rendered[index_by_key[key]] = f"{key}={value}"
            return
        index_by_key[key] = len(rendered)
        rendered.append(f"{key}={value}")

    for key, value in forced_values.items():
        upsert(key, value)

    for key, default in port_defaults.items():
        current = value_by_key.get(key, "")
        if current not in ("", "0"):
            continue
        upsert(key, default)

    for key, default in credential_defaults.items():
        current = value_by_key.get(key, "")
        if current != "":
            continue
        upsert(key, default)

    path.write_text("\n".join(rendered) + "\n", encoding="utf-8")


def _unquote(value: str) -> str:
    raw = value.strip()
    if len(raw) >= 2 and ((raw[0] == raw[-1] == '"') or (raw[0] == raw[-1] == "'")):
        return raw[1:-1]
    return raw
