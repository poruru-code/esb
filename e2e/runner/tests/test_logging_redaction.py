from __future__ import annotations

from e2e.runner.logging import _redact_cmd, _redact_proxy_url


def test_redact_proxy_url_masks_credentials() -> None:
    redacted = _redact_proxy_url("http://user:pass@proxy.example:8080/")
    assert redacted == "http://%2A%2A%2A:%2A%2A%2A@proxy.example:8080/"


def test_redact_proxy_url_without_credentials_is_unchanged() -> None:
    raw = "http://proxy.example:8080/"
    assert _redact_proxy_url(raw) == raw


def test_redact_cmd_masks_proxy_build_args() -> None:
    cmd = [
        "docker",
        "buildx",
        "build",
        "--build-arg",
        "HTTP_PROXY=http://user:pass@proxy.example:8080/",
        "--build-arg",
        "NO_PROXY=localhost,127.0.0.1",
    ]
    redacted = _redact_cmd(cmd)
    assert redacted[4] == "HTTP_PROXY=http://%2A%2A%2A:%2A%2A%2A@proxy.example:8080/"
    assert redacted[6] == "NO_PROXY=localhost,127.0.0.1"
