# Where: e2e/runner/tests/test_buildx_proxy_builder.py
# What: Unit tests for buildx builder proxy propagation and recreation logic.
# Why: Ensure proxy-auth environments can build images through buildx consistently.
from __future__ import annotations

from pathlib import Path

from e2e.runner import buildx


class _Result:
    def __init__(self, returncode: int, stdout: str = "", stderr: str = "") -> None:
        self.returncode = returncode
        self.stdout = stdout
        self.stderr = stderr

    def check_returncode(self) -> None:
        if self.returncode != 0:
            raise RuntimeError("command failed")


def test_ensure_buildx_builder_creates_with_proxy_driver_opts(monkeypatch, tmp_path: Path) -> None:
    config_path = tmp_path / "buildkitd.toml"
    config_path.write_text("debug = false\n", encoding="utf-8")
    calls: list[list[str]] = []

    def fake_run(cmd, capture_output=True, text=True):  # noqa: ANN001
        calls.append(list(cmd))
        if cmd[:4] == ["docker", "buildx", "inspect", "--builder"]:
            return _Result(1, stderr="not found")
        if cmd[:3] == ["docker", "buildx", "create"]:
            return _Result(0, stdout="created")
        return _Result(0)

    monkeypatch.setattr(buildx.subprocess, "run", fake_run)

    buildx.ensure_buildx_builder(
        "esb-buildx",
        config_path=str(config_path),
        proxy_source={
            "HTTP_PROXY": "http://user:pass@proxy.local:8080",
            "NO_PROXY": "localhost,127.0.0.1,registry",
        },
    )

    create_cmd = next(cmd for cmd in calls if cmd[:3] == ["docker", "buildx", "create"])
    assert "--buildkitd-config" in create_cmd
    assert str(config_path) in create_cmd
    assert "env.HTTP_PROXY=http://user:pass@proxy.local:8080" in create_cmd
    assert '"env.NO_PROXY=localhost,127.0.0.1,registry"' in create_cmd
    assert '"env.no_proxy=localhost,127.0.0.1,registry"' in create_cmd


def test_ensure_buildx_builder_recreates_when_proxy_differs(monkeypatch) -> None:
    calls: list[list[str]] = []

    def fake_run(cmd, capture_output=True, text=True):  # noqa: ANN001
        calls.append(list(cmd))
        if cmd[:4] == ["docker", "buildx", "inspect", "--builder"]:
            return _Result(0, stdout="Driver: docker-container\n")
        if cmd[:3] == ["docker", "inspect", "-f"] and "HostConfig.NetworkMode" in cmd[3]:
            return _Result(0, stdout="host\n")
        if cmd[:3] == ["docker", "inspect", "-f"] and ".Config.Env" in cmd[3]:
            return _Result(0, stdout="HTTP_PROXY=http://old:8080\nhttp_proxy=http://old:8080\n")
        if cmd[:4] == ["docker", "buildx", "rm", "esb-buildx"]:
            return _Result(0)
        if cmd[:3] == ["docker", "buildx", "create"]:
            return _Result(0)
        return _Result(0)

    monkeypatch.setattr(buildx.subprocess, "run", fake_run)

    buildx.ensure_buildx_builder(
        "esb-buildx",
        proxy_source={"HTTP_PROXY": "http://new:8080"},
    )

    assert any(cmd[:4] == ["docker", "buildx", "rm", "esb-buildx"] for cmd in calls)
    assert any(cmd[:3] == ["docker", "buildx", "create"] for cmd in calls)


def test_ensure_buildx_builder_reuses_when_proxy_matches(monkeypatch) -> None:
    calls: list[list[str]] = []

    def fake_run(cmd, capture_output=True, text=True):  # noqa: ANN001
        calls.append(list(cmd))
        if cmd[:4] == ["docker", "buildx", "inspect", "--builder"]:
            return _Result(0, stdout="Driver: docker-container\n")
        if cmd[:3] == ["docker", "inspect", "-f"] and "HostConfig.NetworkMode" in cmd[3]:
            return _Result(0, stdout="host\n")
        if cmd[:3] == ["docker", "inspect", "-f"] and ".Config.Env" in cmd[3]:
            return _Result(
                0,
                stdout=(
                    "HTTP_PROXY=http://proxy.local:8080\n"
                    "http_proxy=http://proxy.local:8080\n"
                    "HTTPS_PROXY=http://proxy.local:8080\n"
                    "https_proxy=http://proxy.local:8080\n"
                ),
            )
        if cmd[:4] == ["docker", "buildx", "use", "esb-buildx"]:
            return _Result(0)
        return _Result(0)

    monkeypatch.setattr(buildx.subprocess, "run", fake_run)

    buildx.ensure_buildx_builder(
        "esb-buildx",
        proxy_source={
            "HTTP_PROXY": "http://proxy.local:8080",
            "HTTPS_PROXY": "http://proxy.local:8080",
        },
    )

    assert any(cmd[:4] == ["docker", "buildx", "use", "esb-buildx"] for cmd in calls)
    assert not any(cmd[:3] == ["docker", "buildx", "create"] for cmd in calls)
