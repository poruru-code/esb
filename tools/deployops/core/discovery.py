"""Auto-discovery helpers for deployops CLI inputs."""

from __future__ import annotations

import os
import sys
from pathlib import Path

from tools.deployops.core.runner import RunnerError


def resolve_artifact_manifest_path(
    *,
    project_root: Path,
    artifact: str | None,
    env_hint: str | None = None,
    allow_prompt: bool = True,
) -> Path:
    """Resolve artifact.yml path from explicit input or project conventions."""

    if artifact and artifact.strip():
        raw = Path(artifact).expanduser()
        path = raw.resolve()
        if path.is_dir():
            path = path / "artifact.yml"
        if not path.is_file():
            raise FileNotFoundError(f"artifact manifest not found: {path}")
        return path

    hinted = _collect_env_hints(env_hint)
    hinted_candidates: list[Path] = []
    for hint in hinted:
        hinted_candidates.extend(
            [
                (project_root / "e2e" / "artifacts" / hint / "artifact.yml").resolve(),
                (project_root / "artifacts" / hint / "artifact.yml").resolve(),
            ]
        )

    hinted_existing = _dedupe_existing_files(hinted_candidates)
    if hinted_existing:
        return hinted_existing[0]

    candidates: list[Path] = []
    candidates.append((project_root / "artifact.yml").resolve())
    candidates.extend(
        sorted(path.resolve() for path in project_root.glob("artifacts/*/artifact.yml"))
    )
    candidates.extend(
        sorted(path.resolve() for path in project_root.glob("e2e/artifacts/*/artifact.yml"))
    )

    existing = _dedupe_existing_files(candidates)
    if not existing:
        raise FileNotFoundError(
            "artifact manifest not found. "
            "Pass --artifact, or create one under "
            "artifact.yml / artifacts/*/artifact.yml / e2e/artifacts/*/artifact.yml"
        )
    if len(existing) == 1:
        return existing[0]

    if allow_prompt and _interactive_tty():
        return _prompt_select_path(
            paths=existing,
            label="artifact manifest",
            project_root=project_root,
        )

    raise RunnerError(
        _ambiguous_error(
            label="artifact manifests",
            paths=existing,
            project_root=project_root,
        )
    )


def resolve_artifact_dirs(
    *,
    project_root: Path,
    artifact_dirs: list[str] | None,
    env_hint: str | None = None,
    allow_prompt: bool = True,
) -> list[Path]:
    """Resolve one or more artifact directories (containing artifact.yml)."""

    if artifact_dirs:
        resolved: list[Path] = []
        for raw in artifact_dirs:
            directory = Path(raw).expanduser().resolve()
            if not directory.is_dir():
                raise FileNotFoundError(f"artifact dir not found: {directory}")
            manifest = directory / "artifact.yml"
            if not manifest.is_file():
                raise FileNotFoundError(f"artifact manifest not found: {manifest}")
            resolved.append(directory)
        return resolved

    manifest_path = resolve_artifact_manifest_path(
        project_root=project_root,
        artifact=None,
        env_hint=env_hint,
        allow_prompt=allow_prompt,
    )
    return [manifest_path.parent]


def resolve_env_file_path(
    *,
    project_root: Path,
    env_file: str | None,
    env_hint: str | None = None,
    required: bool,
    allow_prompt: bool = True,
) -> Path | None:
    """Resolve env file path from explicit input or project conventions."""

    if env_file and env_file.strip():
        path = Path(env_file).expanduser().resolve()
        if not path.is_file():
            raise FileNotFoundError(f"env file not found: {path}")
        return path

    hinted = _collect_env_hints(env_hint)
    hinted_candidates: list[Path] = []
    for hint in hinted:
        hinted_candidates.extend(
            [
                (project_root / "e2e" / "environments" / hint / ".env").resolve(),
                (project_root / "environments" / hint / ".env").resolve(),
            ]
        )

    hinted_existing = _dedupe_existing_files(hinted_candidates)
    if hinted_existing:
        return hinted_existing[0]

    candidates: list[Path] = []
    candidates.append((project_root / ".env").resolve())
    candidates.extend(sorted(path.resolve() for path in project_root.glob("environments/*/.env")))
    candidates.extend(
        sorted(path.resolve() for path in project_root.glob("e2e/environments/*/.env"))
    )

    existing = _dedupe_existing_files(candidates)
    if not existing:
        if required:
            raise FileNotFoundError(
                "env file not found. "
                "Pass --env-file, or create one under "
                ".env / environments/*/.env / e2e/environments/*/.env"
            )
        return None
    if len(existing) == 1:
        return existing[0]

    if allow_prompt and _interactive_tty():
        return _prompt_select_path(paths=existing, label="env file", project_root=project_root)

    if required:
        raise RunnerError(
            _ambiguous_error(label="env files", paths=existing, project_root=project_root)
        )
    return None


def resolve_compose_file_path(
    *,
    project_root: Path,
    compose_file: str | None,
    env_file: Path | None,
    mode_hint: str | None = None,
) -> Path:
    """Resolve compose file from explicit input and stable conventions."""

    if compose_file and compose_file.strip():
        path = Path(compose_file).expanduser().resolve()
        if not path.is_file():
            raise FileNotFoundError(f"compose file not found: {path}")
        return path

    normalized_mode = (mode_hint or "").strip().lower()
    mode_candidates = {
        "docker": project_root / "docker-compose.docker.yml",
        "containerd": project_root / "docker-compose.containerd.yml",
        "firecracker": project_root / "docker-compose.firecracker.yml",
    }
    mode_candidate = mode_candidates.get(normalized_mode)
    mode_candidate_path = mode_candidate.resolve() if mode_candidate else None

    if env_file is not None:
        local = (env_file.parent / "docker-compose.yml").resolve()
        if local.is_file():
            # If env is project-root `.env` and mode-specific compose exists,
            # prefer mode-specific compose over generic root docker-compose.yml.
            root_compose = (project_root / "docker-compose.yml").resolve()
            if (
                env_file.parent.resolve() == project_root.resolve()
                and local == root_compose
                and mode_candidate_path is not None
                and mode_candidate_path.is_file()
            ):
                return mode_candidate_path
            return local

    if mode_candidate_path is not None and mode_candidate_path.is_file():
        return mode_candidate_path

    root_default = (project_root / "docker-compose.yml").resolve()
    if root_default.is_file():
        return root_default

    raise FileNotFoundError(
        "compose file not found. Pass --compose-file, or provide "
        "docker-compose.yml near the env file/project root"
    )


def derive_env_hint_from_env_file(env_file: str | None) -> str | None:
    """Extract env hint from env file path or file content."""

    if env_file is None or env_file.strip() == "":
        return None

    path = Path(env_file).expanduser().resolve()
    if not path.is_file():
        return None

    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if line == "" or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        if key.strip() != "ENV":
            continue
        resolved = value.strip().strip("'").strip('"')
        if resolved != "":
            return resolved

    # Fall back to directory name convention (e.g. e2e/environments/e2e-docker/.env)
    parent_name = path.parent.name.strip()
    if parent_name != "":
        return parent_name
    return None


def _collect_env_hints(explicit_hint: str | None) -> list[str]:
    hints: list[str] = []
    for raw in [
        explicit_hint,
        os.environ.get("ESB_ENV"),
        os.environ.get("ENV"),
    ]:
        hint = (raw or "").strip()
        if hint == "" or hint in hints:
            continue
        hints.append(hint)
    return hints


def _dedupe_existing_files(paths: list[Path]) -> list[Path]:
    result: list[Path] = []
    seen: set[Path] = set()
    for path in paths:
        if path in seen or not path.is_file():
            continue
        seen.add(path)
        result.append(path)
    return result


def _interactive_tty() -> bool:
    return sys.stdin.isatty() and sys.stdout.isatty()


def _prompt_select_path(*, paths: list[Path], label: str, project_root: Path) -> Path:
    print(f"Multiple {label} candidates found:")
    for idx, path in enumerate(paths, start=1):
        print(f"  {idx}. {_display_path(path, project_root)}")
    while True:
        raw = input(f"Select {label} [1-{len(paths)}] (default: 1): ").strip()
        if raw == "":
            return paths[0]
        if raw.isdigit():
            index = int(raw)
            if 1 <= index <= len(paths):
                return paths[index - 1]
        print("Invalid selection. Enter a number from the list.")


def _display_path(path: Path, project_root: Path) -> str:
    try:
        return str(path.relative_to(project_root))
    except ValueError:
        return str(path)


def _ambiguous_error(*, label: str, paths: list[Path], project_root: Path) -> str:
    rendered = "\n".join(f"  - {_display_path(path, project_root)}" for path in paths)
    return f"multiple {label} found; specify explicitly.\n{rendered}"
