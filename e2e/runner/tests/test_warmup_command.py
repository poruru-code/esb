# Where: e2e/runner/tests/test_warmup_command.py
# What: Unit tests for E2E warmup buildx setup behavior.
# Why: Ensure buildx preparation is stable and deduplicated across scenarios.
from __future__ import annotations

from e2e.runner import constants, warmup
from e2e.runner.models import Scenario


def _scenario(name: str, env_name: str) -> Scenario:
    return Scenario(
        name=name,
        env_name=env_name,
        mode="docker",
        env_file=None,
        env_dir=None,
        env_vars={},
        targets=[],
        exclude=[],
        project_name="esb",
    )


def test_ensure_buildx_builders_dedupes_same_signature(monkeypatch) -> None:
    scenarios = {
        "a": _scenario("a", "e2e-a"),
        "b": _scenario("b", "e2e-b"),
    }
    runtime_env = {
        "BUILDX_BUILDER": "esb-buildx",
        constants.ENV_BUILDKITD_CONFIG: "/tmp/buildkitd.toml",
        "HTTP_PROXY": "http://proxy.example:8080",
        "http_proxy": "http://proxy.example:8080",
    }
    calls: list[tuple[str, str | None, str | None]] = []

    monkeypatch.setattr(
        warmup,
        "_scenario_runtime_env_for_buildx",
        lambda *_args, **_kwargs: dict(runtime_env),
    )
    monkeypatch.setattr(
        warmup,
        "ensure_buildx_builder",
        lambda builder_name, network_mode="host", config_path=None, proxy_source=None: calls.append(
            (
                builder_name,
                config_path,
                (proxy_source or {}).get("HTTP_PROXY"),
            )
        ),
    )

    warmup._ensure_buildx_builders(scenarios)

    assert calls == [("esb-buildx", "/tmp/buildkitd.toml", "http://proxy.example:8080")]


def test_ensure_buildx_builders_calls_when_signature_differs(monkeypatch) -> None:
    scenarios = {
        "a": _scenario("a", "e2e-a"),
        "b": _scenario("b", "e2e-b"),
    }
    envs = {
        "a": {
            "BUILDX_BUILDER": "esb-buildx",
            constants.ENV_BUILDKITD_CONFIG: "/tmp/buildkitd.toml",
            "HTTP_PROXY": "http://proxy-a.example:8080",
        },
        "b": {
            "BUILDX_BUILDER": "esb-buildx",
            constants.ENV_BUILDKITD_CONFIG: "/tmp/buildkitd.toml",
            "HTTP_PROXY": "http://proxy-b.example:8080",
        },
    }
    calls: list[str] = []

    monkeypatch.setattr(
        warmup,
        "_scenario_runtime_env_for_buildx",
        lambda scenario: dict(envs[scenario.name]),
    )
    monkeypatch.setattr(
        warmup,
        "ensure_buildx_builder",
        lambda builder_name, network_mode="host", config_path=None, proxy_source=None: calls.append(
            builder_name
        ),
    )

    warmup._ensure_buildx_builders(scenarios)

    assert calls == ["esb-buildx", "esb-buildx"]
