from pathlib import Path

from tools.deployops.core.envfile import load_env_file, normalize_bundle_env, read_env_file_value


def test_load_env_file_unquotes_values(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"
    env_path.write_text("A=1\nB=\"two\"\nC='three'\n", encoding="utf-8")

    loaded = load_env_file(env_path)
    assert loaded["A"] == "1"
    assert loaded["B"] == "two"
    assert loaded["C"] == "three"


def test_read_env_file_value_returns_empty_for_missing_key(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"
    env_path.write_text("A=1\n", encoding="utf-8")
    assert read_env_file_value(env_path, "MISSING") == ""


def test_normalize_bundle_env_applies_defaults(tmp_path: Path) -> None:
    env_path = tmp_path / ".env"
    env_path.write_text("PORT_S3=0\nAUTH_USER=\n", encoding="utf-8")

    normalize_bundle_env(env_path, brand_home=".acme")

    content = env_path.read_text(encoding="utf-8")
    assert "PORT_S3=9000" in content
    assert "AUTH_USER=test-admin" in content
    assert "CERT_DIR=/root/.acme/certs" in content
