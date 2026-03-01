from __future__ import annotations

from pathlib import Path

import yaml

from tools.cli import artifact


def test_read_artifact_manifest_and_resolve_paths(tmp_path: Path) -> None:
    manifest_path = tmp_path / "artifact.yml"
    entry_root = tmp_path / "entry"
    runtime_config = entry_root / "runtime-config"
    runtime_config.mkdir(parents=True)

    manifest_path.write_text(
        yaml.safe_dump(
            {
                "schema_version": "1",
                "project": "test-project",
                "env": "dev",
                "mode": "docker",
                "artifacts": [
                    {
                        "artifact_root": "entry",
                        "runtime_config_dir": "runtime-config",
                    }
                ],
            },
            sort_keys=False,
        ),
        encoding="utf-8",
    )

    manifest = artifact.read_artifact_manifest(str(manifest_path), validate=True)
    assert manifest.project == "test-project"
    assert manifest.resolve_artifact_root(str(manifest_path), 0) == str(entry_root.resolve())
    assert manifest.resolve_runtime_config_dir(str(manifest_path), 0) == str(
        runtime_config.resolve()
    )


def test_validate_relative_path_rejects_escape_path() -> None:
    try:
        artifact.validate_relative_path("runtime_config_dir", "../outside")
    except RuntimeError as exc:
        assert "must not escape artifact root" in str(exc)
    else:
        raise AssertionError("expected RuntimeError")


def test_merge_functions_yml_merges_functions_and_preserves_existing_defaults(
    tmp_path: Path,
) -> None:
    src_dir = tmp_path / "src"
    dst_dir = tmp_path / "dst"
    src_dir.mkdir()
    dst_dir.mkdir()

    (src_dir / "functions.yml").write_text(
        yaml.safe_dump(
            {
                "functions": {
                    "fn-new": {"handler": "index.handler"},
                },
                "defaults": {
                    "environment": {"NEW_VAR": "new"},
                    "scaling": {"min": 2},
                },
            },
            sort_keys=False,
        ),
        encoding="utf-8",
    )
    (dst_dir / "functions.yml").write_text(
        yaml.safe_dump(
            {
                "functions": {
                    "fn-existing": {"handler": "old.handler"},
                },
                "defaults": {
                    "environment": {"NEW_VAR": "keep-existing", "EXISTING_VAR": "old"},
                    "scaling": {"min": 1},
                },
            },
            sort_keys=False,
        ),
        encoding="utf-8",
    )

    artifact.merge_functions_yml(str(src_dir), str(dst_dir))
    merged = yaml.safe_load((dst_dir / "functions.yml").read_text(encoding="utf-8"))

    assert set(merged["functions"].keys()) == {"fn-existing", "fn-new"}
    assert merged["defaults"]["environment"]["NEW_VAR"] == "keep-existing"
    assert merged["defaults"]["environment"]["EXISTING_VAR"] == "old"
    assert merged["defaults"]["scaling"]["min"] == 1


def test_merge_routing_yml_replaces_same_route_key(tmp_path: Path) -> None:
    src_dir = tmp_path / "src"
    dst_dir = tmp_path / "dst"
    src_dir.mkdir()
    dst_dir.mkdir()

    (src_dir / "routing.yml").write_text(
        yaml.safe_dump(
            {
                "routes": [
                    {"path": "/hello", "method": "GET", "function": "fn-new"},
                ]
            },
            sort_keys=False,
        ),
        encoding="utf-8",
    )
    (dst_dir / "routing.yml").write_text(
        yaml.safe_dump(
            {
                "routes": [
                    {"path": "/hello", "method": "GET", "function": "fn-old"},
                    {"path": "/other", "method": "POST", "function": "fn-other"},
                ]
            },
            sort_keys=False,
        ),
        encoding="utf-8",
    )

    artifact.merge_routing_yml(str(src_dir), str(dst_dir))
    merged = yaml.safe_load((dst_dir / "routing.yml").read_text(encoding="utf-8"))
    routes = {f"{r['path']}:{r['method']}": r["function"] for r in merged["routes"]}
    assert routes["/hello:GET"] == "fn-new"
    assert routes["/other:POST"] == "fn-other"
