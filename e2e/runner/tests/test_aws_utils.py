# Where: e2e/runner/tests/test_aws_utils.py
# What: Unit tests for AWS helper credential handling.
# Why: Prevent silent fallback credentials in S3 client setup.
from __future__ import annotations

import pytest

from e2e.runner.aws_utils import AWSUtils


def test_create_s3_client_requires_credentials(monkeypatch) -> None:
    monkeypatch.delenv("RUSTFS_ACCESS_KEY", raising=False)
    monkeypatch.delenv("RUSTFS_SECRET_KEY", raising=False)
    with pytest.raises(RuntimeError):
        AWSUtils.create_s3_client()


def test_create_s3_client_uses_env_credentials(monkeypatch) -> None:
    monkeypatch.setenv("RUSTFS_ACCESS_KEY", "ak")
    monkeypatch.setenv("RUSTFS_SECRET_KEY", "sk")
    captured: dict[str, object] = {}

    def fake_client(service, **kwargs):
        captured["service"] = service
        captured["kwargs"] = kwargs
        return object()

    monkeypatch.setattr("e2e.runner.aws_utils.boto3.client", fake_client)
    AWSUtils.create_s3_client(9000)
    assert captured["service"] == "s3"
    kwargs = captured["kwargs"]
    assert isinstance(kwargs, dict)
    assert kwargs["aws_access_key_id"] == "ak"
    assert kwargs["aws_secret_access_key"] == "sk"
