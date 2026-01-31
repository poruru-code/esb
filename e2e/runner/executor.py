import json
import os
import subprocess
import sys
import threading
import time
from collections import deque
from concurrent.futures import ProcessPoolExecutor, ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Any, Callable, Tuple

import requests
import urllib3
from dotenv import load_dotenv
from rich import box
from rich.console import Console, Group
from rich.live import Live
from rich.markup import escape
from rich.panel import Panel
from rich.table import Table
from rich.text import Text

from e2e.runner import constants
from e2e.runner.env import (
    apply_ports_to_env,
    calculate_runtime_env,
    calculate_staging_dir,
    discover_ports,
    read_env_file,
)
from e2e.runner.utils import (
    BRAND_SLUG,
    E2E_STATE_ROOT,
    PROJECT_ROOT,
    build_unique_tag,
    env_key,
    run_esb,
)

# Terminal colors for parallel output
COLORS = [
    "\033[36m",  # Cyan
    "\033[32m",  # Green
    "\033[34m",  # Blue
    "\033[35m",  # Magenta
    "\033[33m",  # Yellow
]
COLOR_RESET = "\033[0m"
COLOR_NAMES = ["cyan", "green", "blue", "magenta", "yellow"]
_OUTPUT_LOCK = threading.Lock()


def safe_print(message: str = "", *, prefix: str | None = None) -> None:
    with _OUTPUT_LOCK:
        if prefix:
            print(f"{prefix} {message}", flush=True)
        else:
            print(message, flush=True)


class ParallelDisplay:
    def __init__(self, profiles: list[str], *, phase: str = "Parallel Run") -> None:
        self.console = Console()
        self._lock = threading.Lock()
        self._profiles = profiles
        self._colors = {
            profile: COLOR_NAMES[idx % len(COLOR_NAMES)] for idx, profile in enumerate(profiles)
        }
        self._statuses = {profile: "waiting" for profile in profiles}
        self._last_progress: dict[str, tuple[str, float]] = {}
        self._log_lines = deque(maxlen=200)
        self._phase = phase
        self._live = Live(
            self._render(),
            console=self.console,
            refresh_per_second=8,
            transient=False,
            screen=False,
        )

    def start(self) -> None:
        self._live.start()

    def stop(self) -> None:
        self._live.stop()

    def set_phase(self, phase: str) -> None:
        with self._lock:
            self._phase = phase
            self._refresh()

    def update_status(self, profile: str, message: str) -> None:
        if profile not in self._statuses:
            return
        with self._lock:
            self._statuses[profile] = message
            self._refresh()

    def log_progress(self, profile: str, message: str, *, min_interval: float = 0.6) -> None:
        color = self._colors.get(profile, "white")
        now = time.monotonic()
        last_msg, last_ts = self._last_progress.get(profile, ("", 0.0))
        if message == last_msg and (now - last_ts) < min_interval:
            return
        self._last_progress[profile] = (message, now)
        line = f"[{color}]{profile}[/] {escape(message)}"
        with self._lock:
            self._log_lines.append(line)
            self._refresh()

    def log(self, profile: str, message: str) -> None:
        color = self._colors.get(profile, "white")
        line = f"[{color}]{profile}[/] {escape(message)}"
        with self._lock:
            self._log_lines.append(line)
            self._refresh()

    def system(self, message: str) -> None:
        line = f"[bold]{escape(message)}[/]"
        with self._lock:
            self._log_lines.append(line)
            self._refresh()

    def _refresh(self) -> None:
        self._live.update(self._render(), refresh=True)

    def _render(self) -> Group:
        table = Table(show_header=True, box=box.SIMPLE, expand=True)
        table.add_column("Env", style="bold")
        table.add_column("Status")
        for profile in self._profiles:
            color = self._colors.get(profile, "white")
            status = escape(self._statuses.get(profile, ""))
            table.add_row(f"[{color}]{profile}[/]", status)
        log_text = Text.from_markup("\n".join(self._log_lines)) if self._log_lines else Text("")
        status_panel = Panel(table, title=self._phase, padding=(0, 1))
        log_panel = Panel(log_text, title="Log", padding=(0, 1))
        return Group(status_panel, log_panel)


def _is_progress_line(line: str) -> bool:
    prefixes = (
        "Resetting environment:",
        "Generating files...",
        "Building base image...",
        "Building OS base image...",
        "Building Python base image...",
        "Building function images",
        "Building control plane images",
        "Built Images for ",
        "Preparing environment:",
        "Building environment:",
        "Cleaning up environment:",
        "Skipping build for ",
        "Waiting for Gateway readiness",
    )
    if line.startswith(prefixes):
        return True
    stripped = line.lstrip()
    indented_prefixes = (
        "- Built function image:",
        "- Skipped function image",
        "- Built control plane image:",
    )
    return stripped.startswith(indented_prefixes)


PROGRESS_PREFIXES = (
    "Resetting environment:",
    "Generating files...",
    "Building base image...",
    "Building OS base image...",
    "Building Python base image...",
    "Building function images",
    "Building control plane images",
    "Built Images for ",
    "Preparing environment:",
    "Building environment:",
    "Cleaning up environment:",
    "Skipping build for ",
    "Waiting for Gateway readiness",
)


def split_progress_messages(line: str) -> list[str]:
    matches = []
    for prefix in PROGRESS_PREFIXES:
        start = 0
        while True:
            idx = line.find(prefix, start)
            if idx == -1:
                break
            matches.append(idx)
            start = idx + len(prefix)
    matches = sorted(set(matches))
    if len(matches) > 1:
        parts = []
        for i, idx in enumerate(matches):
            end = matches[i + 1] if i + 1 < len(matches) else len(line)
            chunk = line[idx:end].strip()
            if chunk:
                parts.append(chunk)
        return parts

    if "Generating files..." in line:
        for token in ("  Container ", "  Network ", "  Image "):
            if token in line:
                left, right = line.split(token, 1)
                left = left.strip()
                right = f"{token.strip()} {right.strip()}"
                return [left, right]
    return [line]


def _is_buildkit_progress_line(line: str) -> bool:
    return ("Image " in line and line.endswith(" Building")) or (
        line.startswith("Image ") and " Building" in line
    )


def _registry_port(project: str, compose_file: Path) -> int | None:
    """Resolve host port mapped to registry:5010 for the given compose project."""
    try:
        result = subprocess.run(
            [
                "docker",
                "compose",
                "-p",
                project,
                "-f",
                str(compose_file),
                "port",
                "registry",
                "5010",
            ],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0 or not result.stdout.strip():
            return None
        return int(result.stdout.strip().split(":")[-1])
    except Exception:
        return None


def resolve_compose_file(scenario: dict[str, Any], mode: str) -> Path:
    env_dir = scenario.get("env_dir")
    if env_dir:
        compose_path = PROJECT_ROOT / env_dir / "docker-compose.yml"
        if compose_path.exists():
            return compose_path
        raise FileNotFoundError(f"Compose file not found in env_dir: {compose_path}")
    return PROJECT_ROOT / f"docker-compose.{mode}.yml"


def isolate_external_network(project_label: str) -> None:
    """Detach non-project containers from the external network to avoid DNS conflicts."""
    network_name = f"{project_label}-external"
    result = subprocess.run(
        ["docker", "network", "inspect", network_name],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        return
    if not data:
        return
    containers = data[0].get("Containers") or {}
    for entry in containers.values():
        name = entry.get("Name", "")
        if not name or name.startswith(project_label):
            continue
        subprocess.run(
            ["docker", "network", "disconnect", "-f", network_name, name],
            capture_output=True,
        )


def _image_safe_name(name: str) -> str:
    value = name.strip().lower()
    if not value:
        return ""
    out: list[str] = []
    prev_sep = False
    for ch in value:
        if ch.isascii() and ch.isalnum():
            out.append(ch)
            prev_sep = False
            continue
        if ch in "._-":
            if not prev_sep:
                out.append(ch)
                prev_sep = True
            continue
        if not prev_sep:
            out.append("-")
            prev_sep = True
    return "".join(out).strip("._-")


def _load_function_image_names(env_name: str) -> list[str]:
    config_path = E2E_STATE_ROOT / env_name / "config" / "functions.yml"
    if not config_path.exists():
        return []
    try:
        import yaml
    except Exception:
        return []
    try:
        data = yaml.safe_load(config_path.read_text()) or {}
    except Exception:
        return []
    functions = data.get("functions", {})
    if isinstance(functions, dict):
        image_prefix = BRAND_SLUG
        image_names: list[str] = []
        for name in functions.keys():
            if not isinstance(name, str):
                continue
            safe = _image_safe_name(name)
            if not safe:
                continue
            image_names.append(f"{image_prefix}-{safe}")
        return sorted(image_names)
    return []


def _state_env_path(env_name: str) -> Path:
    return E2E_STATE_ROOT / env_name / "config" / ".env"


def load_state_env(env_name: str) -> dict[str, str]:
    path = _state_env_path(env_name)
    if not path.exists():
        return {}
    return read_env_file(str(path))


def persist_runtime_env(env_name: str, runtime_env: dict[str, str]) -> None:
    path = _state_env_path(env_name)
    path.parent.mkdir(parents=True, exist_ok=True)

    keys: list[str] = []
    # Prefixed keys (tag/registry/ports)
    for suffix in (
        constants.ENV_TAG,
        constants.ENV_REGISTRY,
        constants.PORT_GATEWAY_HTTPS,
        constants.PORT_GATEWAY_HTTP,
        constants.PORT_AGENT_GRPC,
        constants.PORT_S3,
        constants.PORT_S3_MGMT,
        constants.PORT_DATABASE,
        constants.PORT_VICTORIALOGS,
        constants.PORT_REGISTRY,
    ):
        keys.append(env_key(suffix))
    # Non-prefixed runtime keys
    keys.extend(
        [
            constants.ENV_PROJECT_NAME,
            constants.ENV_CONTAINER_REGISTRY,
            constants.ENV_CONTAINER_REGISTRY_INSECURE,
            constants.ENV_NETWORK_EXTERNAL,
            constants.ENV_SUBNET_EXTERNAL,
            constants.ENV_RUNTIME_NET_SUBNET,
            constants.ENV_RUNTIME_NODE_IP,
            constants.ENV_LAMBDA_NETWORK,
            constants.ENV_CONFIG_DIR,
            constants.ENV_AUTH_USER,
            constants.ENV_AUTH_PASS,
            constants.ENV_JWT_SECRET_KEY,
            constants.ENV_X_API_KEY,
            constants.ENV_RUSTFS_ACCESS_KEY,
            constants.ENV_RUSTFS_SECRET_KEY,
            constants.ENV_ROOT_CA_FINGERPRINT,
            constants.ENV_ROOT_CA_CERT_FILENAME,
            constants.ENV_CERT_DIR,
            constants.ENV_MODE,
            constants.ENV_ENV,
        ]
    )

    seen: set[str] = set()
    ordered_keys: list[str] = []
    for key in keys:
        if key and key not in seen:
            seen.add(key)
            ordered_keys.append(key)

    lines = ["# Auto-generated for E2E runtime state. Do not edit manually."]
    for key in ordered_keys:
        value = runtime_env.get(key)
        if value is None or str(value).strip() == "":
            continue
        lines.append(f"{key}={value}")
    path.write_text("\n".join(lines) + "\n")


def ensure_build_artifacts(env_name: str) -> None:
    config_dir = E2E_STATE_ROOT / env_name / "config"
    required = ("functions.yml", "routing.yml", "resources.yml")
    missing = [name for name in required if not (config_dir / name).exists()]
    if missing:
        missing_list = ", ".join(missing)
        raise FileNotFoundError(
            f"Missing build artifacts for {env_name}: {config_dir} (missing: {missing_list})"
        )


def _pick_manifest_digest(index: dict) -> str | None:
    manifests = index.get("manifests", [])
    if not isinstance(manifests, list) or not manifests:
        return None
    for entry in manifests:
        platform = entry.get("platform") or {}
        if platform.get("os") == "linux" and platform.get("architecture") == "amd64":
            return entry.get("digest")
    return manifests[0].get("digest")


def _is_truthy(value: str | None) -> bool:
    if value is None:
        return False
    return value.strip().lower() in {"1", "true", "yes", "on"}


def verify_registry_images(env_name: str, project: str, mode: str, compose_file: Path) -> None:
    """Validate that registry blobs exist for built function images."""
    # Validating registry blobs is useful for all modes now that we use a local registry everywhere.

    port = _registry_port(project, compose_file)
    if port is None:
        print("[WARN] Registry port not discovered; skipping registry integrity check.")
        return

    image_names = _load_function_image_names(env_name)
    if not image_names:
        print("[WARN] functions.yml not found or empty; skipping registry integrity check.")
        return

    insecure = os.environ.get(constants.ENV_CONTAINER_REGISTRY_INSECURE)
    if not insecure:
        insecure = os.environ.get(env_key(constants.ENV_CONTAINER_REGISTRY_INSECURE))
    scheme = "http" if _is_truthy(insecure) else "https"
    base_url = f"{scheme}://localhost:{port}"
    missing: dict[str, list[str]] = {}

    headers_index = {"Accept": "application/vnd.oci.image.index.v1+json"}
    headers_manifest = {"Accept": "application/vnd.oci.image.manifest.v1+json"}

    for name in image_names:
        tag = os.environ.get(env_key(constants.ENV_TAG), "")
        if not tag:
            print("[WARN] TAG is empty; skipping registry integrity check.")
            return
        try:
            resp = requests.get(
                f"{base_url}/v2/{name}/manifests/{tag}",
                headers=headers_index,
                timeout=10,
                verify=False,
            )
        except Exception as exc:
            missing.setdefault(name, []).append(f"manifest fetch failed: {exc}")
            continue

        if resp.status_code != 200:
            missing.setdefault(name, []).append(f"manifest status {resp.status_code}")
            continue

        try:
            index = resp.json()
        except json.JSONDecodeError:
            missing.setdefault(name, []).append("manifest is not valid JSON")
            continue

        digest = _pick_manifest_digest(index)
        if not digest:
            missing.setdefault(name, []).append("manifest digest not found")
            continue

        resp = requests.get(
            f"{base_url}/v2/{name}/manifests/{digest}",
            headers=headers_manifest,
            timeout=10,
            verify=False,
        )
        if resp.status_code != 200:
            missing.setdefault(name, []).append(f"image manifest status {resp.status_code}")
            continue

        try:
            manifest = resp.json()
        except json.JSONDecodeError:
            missing.setdefault(name, []).append("image manifest is not valid JSON")
            continue

        layers = manifest.get("layers", [])
        for layer in layers:
            blob = layer.get("digest")
            if not blob:
                continue
            head = requests.head(
                f"{base_url}/v2/{name}/blobs/{blob}",
                timeout=10,
                verify=False,
            )
            if head.status_code != 200:
                missing.setdefault(name, []).append(blob)

    if missing:
        print("\n[ERROR] Registry is missing image blobs; containerd pulls will fail.")
        for name, blobs in sorted(missing.items()):
            sample = ", ".join(blobs[:5])
            suffix = "" if len(blobs) <= 5 else f" (+{len(blobs) - 5} more)"
            print(f"  - {name}: {sample}{suffix}")
        raise RuntimeError("registry missing blobs")


def print_built_images(
    env_name: str,
    project_name: str,
    prefix: str = "",
    duration_seconds: float | None = None,
    printer: Callable[[str], None] | None = None,
) -> None:
    label_prefix = f"com.{BRAND_SLUG}"
    project_label = f"{project_name}-{env_name}"
    sep = "\x1f"
    cmd = [
        "docker",
        "images",
        "--filter",
        f"label={label_prefix}.managed=true",
        "--filter",
        f"label={label_prefix}.project={project_label}",
        "--filter",
        f"label={label_prefix}.env={env_name}",
        "--format",
        (
            f"{{{{.Repository}}}}:{{{{.Tag}}}}{sep}{{{{.ID}}}}{sep}"
            f"{{{{.CreatedSince}}}}{sep}{{{{.Size}}}}{sep}{{{{.CreatedAt}}}}"
        ),
    ]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"[WARN] Failed to list built images for {env_name}: {result.stderr.strip()}")
        return

    raw_lines = [line.strip() for line in result.stdout.splitlines() if line.strip()]
    if not raw_lines:
        print(f"[WARN] No built images found for {env_name} ({project_label}).")
        return

    rows = []
    for line in raw_lines:
        parts = [part.strip() for part in line.split(sep)]
        if len(parts) != 5:
            continue
        rows.append(parts)
    if not rows:
        print(f"[WARN] No built images found for {env_name} ({project_label}).")
        return

    # If multiple localhost registry ports exist, show only the most recent one.
    localhost_ports: dict[str, str] = {}
    for repo, _, _, _, created_at in rows:
        if not repo.startswith("localhost:"):
            continue
        host = repo.split("/", 1)[0]
        if ":" not in host:
            continue
        port = host.split(":", 1)[1]
        if not port.isdigit():
            continue
        created_key = created_at[:19]
        if port not in localhost_ports or created_key > localhost_ports[port]:
            localhost_ports[port] = created_key

    if len(localhost_ports) > 1:
        latest_port = max(localhost_ports.items(), key=lambda item: item[1])[0]
        rows = [
            row
            for row in rows
            if not row[0].startswith("localhost:") or row[0].startswith(f"localhost:{latest_port}/")
        ]

    widths = [0, 0, 0, 0]
    for repo, image_id, created, size, _ in rows:
        widths[0] = max(widths[0], len(repo))
        widths[1] = max(widths[1], len(image_id))
        widths[2] = max(widths[2], len(created))
        widths[3] = max(widths[3], len(size))

    def emit(line: str) -> None:
        rendered = f"{prefix} {line}" if prefix else line
        if printer:
            printer(rendered)
            return
        print(rendered)

    duration_suffix = ""
    if duration_seconds is not None:
        duration_suffix = f" in {duration_seconds:.1f}s"
    header = f"🧱 Built Images for {env_name} ({project_label}){duration_suffix}:"
    if prefix or printer:
        emit(header)
    else:
        print(f"\n{header}")

    for repo, image_id, created, size, _ in rows:
        emit(
            f"   {repo.ljust(widths[0])}  "
            f"{image_id.ljust(widths[1])}  "
            f"{created.ljust(widths[2])}  "
            f"{size.rjust(widths[3])}"
        )
    if not prefix and printer is None:
        print("")


def thorough_cleanup(env_name: str):
    """Exhaustively remove Docker resources associated with an environment."""
    project_label = f"{BRAND_SLUG}-{env_name}"

    # 1. Containers
    container_filters = [
        f"name={env_name}",
        f"label=com.docker.compose.project={project_label}",
    ]
    for filt in container_filters:
        result = subprocess.run(
            ["docker", "ps", "-aq", "--filter", filt],
            capture_output=True,
            text=True,
        )
        container_ids = [cid.strip() for cid in result.stdout.split() if cid.strip()]
        if container_ids:
            print(f"  • Removing containers for {env_name} ({filt})...")
            subprocess.run(["docker", "rm", "-f"] + container_ids, capture_output=True)

    # 2. Networks
    network_filters = [
        f"label=com.docker.compose.project={project_label}",
        f"name={project_label}-external",
        f"name={project_label}_default",
    ]
    for filt in network_filters:
        result = subprocess.run(
            ["docker", "network", "ls", "-q", "--filter", filt],
            capture_output=True,
            text=True,
        )
        network_ids = [nid.strip() for nid in result.stdout.split() if nid.strip()]
        if network_ids:
            print(f"  • Removing networks for {env_name} ({filt})...")
            # Networks might still be in use if some containers weren't properly removed
            subprocess.run(["docker", "network", "rm"] + network_ids, capture_output=True)

    # 3. Volumes
    # We check by label AND by name prefix for maximum safety
    volume_filters = [
        f"label=com.docker.compose.project={project_label}",
        f"name={project_label}_",
    ]
    seen_volumes = set()
    for filt in volume_filters:
        result = subprocess.run(
            ["docker", "volume", "ls", "-q", "--filter", filt],
            capture_output=True,
            text=True,
        )
        volume_ids = [vid.strip() for vid in result.stdout.split() if vid.strip()]
        to_remove = [v for v in volume_ids if v not in seen_volumes]
        if to_remove:
            print(f"  • Removing volumes for {env_name} ({filt})...")
            subprocess.run(["docker", "volume", "rm"] + to_remove, capture_output=True)
            seen_volumes.update(to_remove)
    # Note: Image pruning removed from per-scenario cleanup.
    # 'docker image prune' is a global operation that interferes with parallel execution.
    # Run it manually or via a post-test cleanup script instead.


def cleanup_managed_images(env_name: str, project_name: str) -> None:
    """Remove ESB-managed images associated with an environment."""
    label_prefix = f"com.{BRAND_SLUG}"
    project_label = f"{project_name}-{env_name}"
    cmd = [
        "docker",
        "images",
        "-q",
        "--filter",
        f"label={label_prefix}.managed=true",
        "--filter",
        f"label={label_prefix}.project={project_label}",
        "--filter",
        f"label={label_prefix}.env={env_name}",
    ]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"[WARN] Failed to list images for cleanup ({env_name}): {result.stderr.strip()}")
        return
    image_ids = [img.strip() for img in result.stdout.splitlines() if img.strip()]
    if not image_ids:
        return
    print(f"  • Removing managed images for {env_name} ({len(image_ids)} images)...")
    try:
        subprocess.run(["docker", "rmi", "-f"] + image_ids, check=False, capture_output=True)
    except Exception as exc:
        print(f"[WARN] Failed to remove images for {env_name}: {exc}")


def warmup_environment(
    env_scenarios: dict, matrix: list[dict], args, display: ParallelDisplay | None = None
):
    """
    Perform global reset and warm-up actions.
    This includes light validation to ensure shared inputs exist before parallel execution.
    """
    active_envs = list(env_scenarios.keys())
    if not active_envs:
        if display:
            display.system("[ERROR] No active environments in matrix.")
        else:
            print("[ERROR] No active environments in matrix.")
        sys.exit(1)

    esb_template = PROJECT_ROOT / "e2e" / "fixtures" / "template.yaml"
    if not esb_template.exists():
        if display:
            display.system(f"[ERROR] Missing E2E template: {esb_template}")
        else:
            print(f"[ERROR] Missing E2E template: {esb_template}")
        sys.exit(1)

    if display:
        display.system(f"[INIT] Using E2E template: {esb_template}")
    else:
        print(f"\n[INIT] Using E2E template: {esb_template}")


def run_scenario(args, scenario):
    """Run a single scenario."""
    # 0. Resolve scenario-specific parameters once
    env_name = scenario.get("esb_env", os.environ.get(env_key("ENV"), "e2e-docker"))
    raw_env_file = scenario.get("env_file")
    env_file = str((PROJECT_ROOT / raw_env_file).absolute()) if raw_env_file else None
    project_name = scenario.get("esb_project", BRAND_SLUG)
    do_reset = scenario.get("perform_reset", args.reset)
    do_build = scenario.get("perform_build", args.build)
    build_only = scenario.get("build_only", False) or args.build_only
    test_only = scenario.get("test_only", False) or args.test_only
    env_vars_override = scenario.get("env_vars", {})

    if build_only and test_only:
        raise ValueError("build_only and test_only cannot both be true")

    # 0.1 Set ESB variables for resolution and safety
    # We set these in os.environ so the CLI doesn't prompt even if .env fails.
    os.environ[env_key("PROJECT")] = project_name
    os.environ[env_key("ENV")] = env_name
    os.environ[env_key("HOME")] = str((E2E_STATE_ROOT / env_name).absolute())

    # Load Scenario-Specific Env File (Required for isolation)
    if env_file:
        p = Path(env_file)
        if p.exists():
            load_dotenv(p, override=True)
            print(f"Loaded scenario environment from: {p}")
        else:
            print(f"Warning: Scenario environment file not found: {p}")
    else:
        print("Warning: No env_file specified for this scenario. Operating with system env only.")

    state_env = load_state_env(env_name)
    if state_env:
        credential_keys = {
            constants.ENV_AUTH_USER,
            constants.ENV_AUTH_PASS,
            constants.ENV_JWT_SECRET_KEY,
            constants.ENV_X_API_KEY,
            constants.ENV_RUSTFS_ACCESS_KEY,
            constants.ENV_RUSTFS_SECRET_KEY,
        }
        if test_only:
            os.environ.update(state_env)
        else:
            os.environ.update({k: v for k, v in state_env.items() if k in credential_keys})

    # 2.5 Inject Proxy Settings
    # 3. Reload env vars into a dict for passing to subprocess (pytest)
    env = os.environ.copy()

    # Capture calculated values for convenience
    env["GATEWAY_PORT"] = env.get(env_key("PORT_GATEWAY_HTTPS"), "443")
    env["VICTORIALOGS_PORT"] = env.get(env_key("PORT_VICTORIALOGS"), "9428")
    env["GATEWAY_URL"] = f"https://localhost:{env['GATEWAY_PORT']}"
    env["VICTORIALOGS_URL"] = f"http://localhost:{env['VICTORIALOGS_PORT']}"
    env["AGENT_GRPC_ADDRESS"] = f"localhost:{env.get(env_key('PORT_AGENT_GRPC'), '50051')}"
    env[env_key("PROJECT_NAME")] = f"{project_name}-{env_name}"

    # Merge scenario-specific environment variables
    env.update(env_vars_override)

    # 1.5 Calculate Runtime Env (needed for reset/build too)
    mode = scenario.get("mode", "docker")
    compose_file_path = resolve_compose_file(scenario, mode)
    template_path = PROJECT_ROOT / "e2e" / "fixtures" / "template.yaml"
    if not template_path.exists():
        raise FileNotFoundError(f"Missing E2E template: {template_path}")
    runtime_env = calculate_runtime_env(project_name, env_name, mode, env_file)
    compose_project = f"{project_name}-{env_name}"
    staging_config_dir = calculate_staging_dir(compose_project, env_name)
    if staging_config_dir.exists():
        runtime_env[constants.ENV_CONFIG_DIR] = str(staging_config_dir)
    tag_key = env_key(constants.ENV_TAG)
    tag_override = env_vars_override.get(tag_key) or os.environ.get(tag_key)
    if tag_override:
        runtime_env[tag_key] = tag_override
    else:
        current_tag = runtime_env.get(tag_key, "").strip()
        if current_tag == "" or current_tag == "latest":
            runtime_env[tag_key] = build_unique_tag(env_name)
    os.environ[tag_key] = runtime_env.get(tag_key, "")
    insecure_key = env_key(constants.ENV_CONTAINER_REGISTRY_INSECURE)
    if insecure_key in runtime_env:
        os.environ[insecure_key] = runtime_env[insecure_key]
    build_env_base = runtime_env.copy()
    build_env_base["PROJECT_NAME"] = compose_project
    build_env_base["ESB_META_REUSE"] = "1"

    # Merge runtime environment into pytest environment to ensure variables like
    # ESB_REGISTRY are available to subprocesses (e.g. docker compose in tests).
    env.update(runtime_env)
    env.update(env_vars_override)

    did_up = False
    try:
        if not test_only:
            # 2. Reset / Build
            build_args = [
                "--template",
                str(template_path.absolute()),
                "build",
                "--no-save-defaults",
                "--env",
                env_name,
                "--mode",
                mode,
            ]
            if args.no_cache:
                build_args.append("--no-cache")
            if do_reset:
                print(f"Resetting environment: {env_name}")
                # 2.1 Robust cleanup using docker compose down
                # matches the "down" logic from legacy esb up replacement
                if compose_file_path.exists():
                    proj_key = f"{BRAND_SLUG}-{env_name}"
                    subprocess.run(
                        [
                            "docker",
                            "compose",
                            "--project-name",
                            proj_key,
                            "--file",
                            str(compose_file_path),
                            "down",
                            "--volumes",
                            "--remove-orphans",
                        ],
                        capture_output=True,
                    )
                # Fallback to manual cleanup just in case (e.g. if compose file invalid or project name mismatch previously)
                thorough_cleanup(env_name)
                cleanup_managed_images(env_name, project_name)
                isolate_external_network(f"{BRAND_SLUG}-{env_name}")

                # 2.2 Clean artifact directory for this environment
                env_state_dir = E2E_STATE_ROOT / env_name
                if env_state_dir.exists():
                    print(f"  • Cleaning artifact directory: {env_state_dir}")
                    import shutil

                    shutil.rmtree(env_state_dir)

                # Re-generate configurations and build images via ESB build
                run_esb(
                    build_args,
                    env_file=env_file,
                    verbose=args.verbose,
                    env=build_env_base,
                )
                ensure_build_artifacts(env_name)
                persist_runtime_env(env_name, build_env_base)
                verify_registry_images(
                    env_name,
                    f"{project_name}-{env_name}",
                    mode,
                    compose_file_path,
                )
                if not (build_only and os.environ.get("E2E_WORKER") == "1"):
                    print_built_images(env_name, project_name)

                if build_only:
                    return True

                # Manual orchestration will handle starting the services in Step 3
                did_up = False
            elif do_build:
                if not args.verbose:
                    safe_print(f"Building environment: {env_name}...")
                else:
                    print(f"Building environment: {env_name}")
                run_esb(
                    build_args,
                    env_file=env_file,
                    verbose=args.verbose,
                    env=build_env_base,
                )
                ensure_build_artifacts(env_name)
                persist_runtime_env(env_name, build_env_base)
                verify_registry_images(
                    env_name,
                    f"{project_name}-{env_name}",
                    mode,
                    compose_file_path,
                )
                if not (build_only and os.environ.get("E2E_WORKER") == "1"):
                    print_built_images(env_name, project_name)
                if not args.verbose:
                    print("Done")
            else:
                if not args.verbose:
                    safe_print(f"Preparing environment: {env_name}...")
                else:
                    print(f"Preparing environment: {env_name}")
                # In Zero-Config, we just rebuild if needed. stop/sync are gone.
                run_esb(build_args, env_file=env_file, verbose=args.verbose, env=build_env_base)
                ensure_build_artifacts(env_name)
                persist_runtime_env(env_name, build_env_base)
                verify_registry_images(
                    env_name,
                    f"{project_name}-{env_name}",
                    mode,
                    compose_file_path,
                )
                if not (build_only and os.environ.get("E2E_WORKER") == "1"):
                    print_built_images(env_name, project_name)
                if not args.verbose:
                    print("Done")
        else:
            ensure_build_artifacts(env_name)
            if args.verbose:
                print(f"Skipping build for {env_name} (test-only)")

        # If build_only, skip UP and tests
        if build_only:
            return

        # 3. UP (Manual Orchestration)
        if not did_up:
            # Critical: Override PROJECT_NAME to include env suffix for isolation (e.g. esb-e2e-docker)
            # This matches the logic in the Go CLI builder and ensures container names are unique.
            runtime_env["PROJECT_NAME"] = compose_project

            # Merge with existing system/process env to ensure PATH etc are preserved
            compose_env = os.environ.copy()
            compose_env.update(runtime_env)
            compose_env.update(env_vars_override)

            # Pass RESOURCES_YML to docker compose so provisioner can find it
            resources_yml_path = E2E_STATE_ROOT / env_name / "config" / "resources.yml"
            compose_env["RESOURCES_YML"] = str(resources_yml_path.absolute())

            if not compose_file_path.exists():
                print(f"[WARN] Compose file not found at {compose_file_path}.")
                raise FileNotFoundError(f"Compose file not found: {compose_file_path}")

            compose_base_cmd = [
                "docker",
                "compose",
                "--project-name",
                f"{BRAND_SLUG}-{env_name}",
                "--file",
                str(compose_file_path),
            ]

            registry_cmd = compose_base_cmd + ["up", "--detach", "registry"]
            if args.verbose:
                print(f"Running: {' '.join(registry_cmd)}")
            subprocess.run(registry_cmd, check=True, env=compose_env)

            compose_cmd = compose_base_cmd + ["up", "--detach", "--no-build"]

            if args.verbose:
                print(f"Running: {' '.join(compose_cmd)}")

            subprocess.run(compose_cmd, check=True, env=compose_env)

            # Sync is gone. Zero-Config provisioner service handles it.
            # We just need to discover ports for the host-side testing.
            ports = discover_ports(f"{BRAND_SLUG}-{env_name}", compose_file_path)

            # Wait for Gateway readiness (parity with legacy esb up)
            wait_for_gateway(env_name, verbose=args.verbose, ports=ports)

            # --- USER REQUEST: Print generated info ---
            if ports:
                print(f"\n🔌 Discovered Ports for {env_name}:")
                # Sort for stable output
                for k in sorted(ports.keys()):
                    print(f"   {k}: {ports[k]}")

            # Try to read generated credentials from the provisioner or env
            # The CLI generates them into .env, but we might be running in a clean env.
            # However, the compose environment `compose_env` has them if they were generated/loaded.
            # But `runtime_env` was calculated before build.

            # Actually, `esb build` might generate new credentials if they were missing.
            # We should try to read the .env file from the state dir if it exists.
            state_env_file = E2E_STATE_ROOT / env_name / "config" / ".env"
            if state_env_file.exists():
                print(f"\n🔑 Credentials (from {state_env_file}):")
                with open(state_env_file, "r") as f:
                    for line in f:
                        line = line.strip()
                        if line and not line.startswith("#"):
                            if any(k in line for k in ["AUTH_", "SECRET", "KEY"]):
                                print(f"   {line}")
                            elif "VICTORIALOGS" in line:  # Useful to see full URLs if present
                                print(f"   {line}")
            print("")
            # ------------------------------------------

            # Apply discovered ports to OS environment and local env dict for pytest
            apply_ports_to_env(ports)
            env.update({k: str(v) for k, v in ports.items()})
            # Re-apply composite variables that depend on ports
            if env_key(constants.PORT_GATEWAY_HTTPS) in ports:
                env["GATEWAY_PORT"] = str(ports[env_key(constants.PORT_GATEWAY_HTTPS)])
                env["GATEWAY_URL"] = f"https://localhost:{env['GATEWAY_PORT']}"
            if env_key(constants.PORT_VICTORIALOGS) in ports:
                env["VICTORIALOGS_PORT"] = str(ports[env_key(constants.PORT_VICTORIALOGS)])
                env["VICTORIALOGS_URL"] = f"http://localhost:{env['VICTORIALOGS_PORT']}"
        if ports:
            apply_ports_to_env(ports)

            # Update env dict for pytest subprocess
            for k, v in ports.items():
                env[k] = str(v)

            env["GATEWAY_PORT"] = str(
                ports.get(env_key("PORT_GATEWAY_HTTPS"), env.get("GATEWAY_PORT", "443"))
            )
            env["VICTORIALOGS_PORT"] = str(
                ports.get(env_key("PORT_VICTORIALOGS"), env.get("VICTORIALOGS_PORT", "9428"))
            )
            env["GATEWAY_URL"] = f"https://localhost:{env['GATEWAY_PORT']}"
            env["VICTORIALOGS_URL"] = f"http://localhost:{env['VICTORIALOGS_PORT']}"
            agent_key = env_key("PORT_AGENT_GRPC")
            if agent_key in ports:
                env["AGENT_GRPC_ADDRESS"] = f"localhost:{ports[agent_key]}"

            agent_metrics_key = env_key("PORT_AGENT_METRICS")
            if agent_metrics_key in ports:
                env["AGENT_METRICS_PORT"] = str(ports[agent_metrics_key])
                env["AGENT_METRICS_URL"] = f"http://localhost:{ports[agent_metrics_key]}"

        from e2e.runner.env import apply_gateway_env_from_container

        apply_gateway_env_from_container(env, f"{BRAND_SLUG}-{env_name}")

        # 4. Run Tests
        if not scenario["targets"]:
            # No test targets specified, skip test execution
            return

        print(f"\\n=== Running Tests for {scenario['name']} ===\n")

        pytest_cmd = [sys.executable, "-m", "pytest"] + scenario["targets"] + ["-v"]

        # Excludes
        for excl in scenario["exclude"]:
            pytest_cmd.extend(["--ignore", excl])

        # Pass the full env with calculated ports to pytest
        result = subprocess.run(pytest_cmd, cwd=PROJECT_ROOT, check=False, env=env)

        if result.returncode != 0:
            sys.exit(result.returncode)

    except subprocess.CalledProcessError as e:
        print(f"Error executing command: {e}")
        sys.exit(1)
    except RuntimeError as e:
        print(f"Error executing command: {e}")
        sys.exit(1)

    finally:
        if args.cleanup:
            if not args.verbose:
                safe_print(f"Cleaning up environment: {env_name}...")
            else:
                print(f"Cleaning up environment: {env_name}")
            # esb down is gone, use docker compose directly
            proj_key = f"{BRAND_SLUG}-{env_name}"
            subprocess.run(
                ["docker", "compose", "-p", proj_key, "-f", str(compose_file_path), "down"],
                capture_output=True,
            )
            if not args.verbose:
                print("Done")


def run_profile_subprocess(
    profile_name: str,
    cmd: list[str],
    color_code: str = "",
    verbose: bool = False,
    label_width: int = 0,
    display: ParallelDisplay | None = None,
) -> Tuple[int, str]:
    """Run a profile in a subprocess and stream output with prefix."""
    label = f"[{profile_name}]"
    if label_width > 0:
        label = label.ljust(label_width)
    prefix = f"{color_code}{label}{COLOR_RESET}"

    def emit(message: str, *, is_progress: bool = False) -> None:
        if display:
            if is_progress:
                display.update_status(profile_name, message)
                display.log_progress(profile_name, message)
            else:
                display.log(profile_name, message)
        else:
            safe_print(message, prefix=prefix)

    # Inject flags to force non-interactive behavior
    env = os.environ.copy()
    env["TERM"] = "dumb"
    env[env_key("INTERACTIVE")] = "0"
    env["E2E_WORKER"] = "1"

    process = subprocess.Popen(
        cmd,
        cwd=PROJECT_ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        stdin=subprocess.DEVNULL,
        text=True,
        bufsize=1,  # Line buffered
        env=env,
    )

    output_lines = []
    tests_started = False
    reported_tests = False
    in_special_block = False
    last_line_was_blank = True
    early_failure = False
    early_failure_patterns = (
        "Error executing command:",
        "ERROR: failed to build",
        "failed to solve:",
    )

    # Read output line by line as it becomes available
    try:
        if process.stdout:
            for line in iter(process.stdout.readline, ""):
                clean_line = line.rstrip()
                if clean_line:
                    if "test session starts" in clean_line:
                        tests_started = True
                        if display and not reported_tests:
                            display.update_status(profile_name, "tests running")
                            reported_tests = True

                    # Detect special info blocks (Auth credentials and Discovered Ports)
                    # Starts with Key or Plug emoji
                    is_special_header = clean_line.startswith("🔑") or clean_line.startswith("🔌")
                    if is_special_header:
                        in_special_block = True

                    is_buildkit_progress = _is_buildkit_progress_line(clean_line)
                    should_print = (
                        verbose
                        or tests_started
                        or _is_progress_line(clean_line)
                        or in_special_block
                        or is_buildkit_progress
                    )

                    if should_print:
                        for message in split_progress_messages(clean_line):
                            is_progress = _is_progress_line(message) or _is_buildkit_progress_line(
                                message
                            )
                            emit(message, is_progress=is_progress)
                        last_line_was_blank = False

                    # End of special block if we encounter a new progress line
                    # or if the line is not indented (and not the header itself)
                    if in_special_block and not is_special_header:
                        if _is_progress_line(clean_line) or not clean_line.startswith(" "):
                            in_special_block = False

                    if not tests_started and any(
                        pat in clean_line for pat in early_failure_patterns
                    ):
                        early_failure = True
                        emit("Build failed; stopping this environment.")
                        process.terminate()
                        break
                else:
                    # Empty line terminates a block
                    if in_special_block:
                        in_special_block = False

                    # Preserve blank lines only in verbose mode
                    if not last_line_was_blank:
                        if verbose:
                            emit("")
                        last_line_was_blank = True

                output_lines.append(line)

    except Exception as e:
        emit(f"Error reading output: {e}")

    try:
        returncode = process.wait(timeout=15)
    except subprocess.TimeoutExpired:
        if early_failure:
            process.kill()
        returncode = process.wait()

    if returncode != 0 and not verbose and not tests_started:
        emit("Subprocess failed before tests started. Printing cached logs...")
        emit("")
        for line in output_lines:
            emit(line.rstrip())

    # Write output to a log file for debugging
    log_file = PROJECT_ROOT / "e2e" / f".parallel-{profile_name}.log"
    with open(log_file, "w", encoding="utf-8") as f:
        f.write(f"=== {profile_name} combined output ===\n")
        f.writelines(output_lines)

    return returncode, "".join(output_lines)


def run_profiles_with_executor(
    env_scenarios: dict[str, dict[str, Any]],
    reset: bool,
    build: bool,
    cleanup: bool,
    fail_fast: bool,
    max_workers: int,
    verbose: bool = False,
    test_only: bool = False,
    display: ParallelDisplay | None = None,
) -> dict[str, tuple[bool, list[str]]]:
    """
    Run environments using threads for parallel runs to keep console output coherent.
    If max_workers is 1, use subprocess isolation without shared console state.
    Returns: dict mapping env_name to (success, failed_scenario_names)
    """
    results = {}

    # Calculate max profile name length for aligned logging (+2 for brackets)
    max_label_len = max(len(p) for p in env_scenarios.keys()) + 2 if env_scenarios else 0
    profiles = list(env_scenarios.keys())
    use_rich = max_workers > 1
    progress_display = display if display else (ParallelDisplay(profiles) if use_rich else None)
    manage_display = progress_display is not None and display is None
    executor_cls = ThreadPoolExecutor if use_rich else ProcessPoolExecutor

    if manage_display:
        progress_display.set_phase("Test Phase (Parallel)")
        progress_display.start()

    # We use 'spawn' or default context. For simple script execution, default is fine.
    try:
        with executor_cls(max_workers=max_workers) as executor:
            future_to_profile = {}

            # Submit all tasks
            for profile_name in profiles:
                # Build command for subprocess
                cmd = [
                    sys.executable,
                    "-u",  # Unbuffered output
                    "-m",
                    "e2e.run_tests",
                    "--profile",
                    profile_name,
                ]
                if test_only:
                    cmd.append("--test-only")
                else:
                    if reset:
                        cmd.append("--reset")
                    if build:
                        cmd.append("--build")
                if cleanup:
                    cmd.append("--cleanup")
                if fail_fast:
                    cmd.append("--fail-fast")
                if verbose:
                    cmd.append("--verbose")

                # Determine log prefix/color
                profile_index = profiles.index(profile_name)
                color_code = COLORS[profile_index % len(COLORS)]

                if max_workers > 1:
                    message = f"[PARALLEL] Scheduling environment: {profile_name}"
                    if progress_display:
                        progress_display.system(message)
                        progress_display.update_status(profile_name, "queued")
                    else:
                        print(message)

                future = executor.submit(
                    run_profile_subprocess,
                    profile_name,
                    cmd,
                    color_code,
                    verbose,
                    max_label_len,
                    progress_display,
                )
                future_to_profile[future] = profile_name

            # Process results as they complete
            for future in as_completed(future_to_profile):
                profile_name = future_to_profile[future]
                try:
                    returncode, output = future.result()
                    success = returncode == 0
                    failed_list = [] if success else [profile_name]

                    prefix = "[PARALLEL]" if max_workers > 1 else "[MATRIX]"

                    if success:
                        message = f"✅ {prefix} Environment {profile_name} PASSED"
                    else:
                        message = (
                            f"❌ {prefix} Environment {profile_name} FAILED "
                            f"(exit code: {returncode})"
                        )
                    if progress_display:
                        progress_display.system(message)
                        progress_display.update_status(
                            profile_name, "PASSED" if success else "FAILED"
                        )
                    else:
                        print(message)

                    results[profile_name] = (success, failed_list)
                except Exception as e:
                    message = f"[ERROR] Environment {profile_name} FAILED with exception: {e}"
                    if progress_display:
                        progress_display.system(message)
                        progress_display.update_status(profile_name, "FAILED")
                    else:
                        print(message)
                    results[profile_name] = (False, [f"Environment {profile_name} (exception)"])
    finally:
        if manage_display and progress_display:
            progress_display.stop()

    return results


def run_build_phase_serial(
    env_scenarios: dict[str, dict[str, Any]],
    reset: bool,
    build: bool,
    fail_fast: bool,
    verbose: bool = False,
) -> list[str]:
    """Run build phase sequentially in isolated subprocesses."""
    failed: list[str] = []
    if not env_scenarios:
        return failed

    durations: dict[str, float] = {}
    total_start = time.monotonic()
    max_label_len = max(len(p) for p in env_scenarios.keys()) + 2
    profiles = list(env_scenarios.keys())

    for idx, profile_name in enumerate(profiles):
        cmd = [
            sys.executable,
            "-u",
            "-m",
            "e2e.run_tests",
            "--profile",
            profile_name,
            "--build-only",
        ]
        if reset:
            cmd.append("--reset")
        if build:
            cmd.append("--build")
        if verbose:
            cmd.append("--verbose")

        color_code = COLORS[idx % len(COLORS)]
        start = time.monotonic()
        returncode, _ = run_profile_subprocess(
            profile_name, cmd, color_code, verbose, max_label_len
        )
        durations[profile_name] = time.monotonic() - start
        if returncode == 0:
            scenario = env_scenarios.get(profile_name, {})
            project_name = scenario.get("esb_project", BRAND_SLUG)
            print_built_images(
                profile_name,
                project_name,
                duration_seconds=durations[profile_name],
            )
        if returncode != 0:
            failed.append(profile_name)
            if fail_fast:
                break

    total_elapsed = time.monotonic() - total_start
    if durations:
        print("\n=== Build Phase Summary ===")
        for profile_name in profiles:
            if profile_name in durations:
                print(f"- {profile_name}: {durations[profile_name]:.1f}s")
        print(f"Total: {total_elapsed:.1f}s\n")

    return failed


def run_build_phase_parallel(
    env_scenarios: dict[str, dict[str, Any]],
    reset: bool,
    build: bool,
    fail_fast: bool,
    verbose: bool = False,
    display: ParallelDisplay | None = None,
) -> list[str]:
    """Run build phase in parallel subprocesses with shared console output."""
    failed: list[str] = []
    if not env_scenarios:
        return failed

    durations: dict[str, float] = {}
    total_start = time.monotonic()
    max_label_len = max(len(p) for p in env_scenarios.keys()) + 2
    profiles = list(env_scenarios.keys())
    max_workers = len(profiles)
    future_to_profile: dict[Any, tuple[str, float, str]] = {}
    progress_display = (
        display if display else (ParallelDisplay(profiles) if max_workers > 1 else None)
    )
    manage_display = progress_display is not None and display is None
    if manage_display:
        progress_display.set_phase("Build Phase (Parallel)")
        progress_display.start()

    try:
        with ThreadPoolExecutor(max_workers=max_workers) as executor:
            for idx, profile_name in enumerate(profiles):
                cmd = [
                    sys.executable,
                    "-u",
                    "-m",
                    "e2e.run_tests",
                    "--profile",
                    profile_name,
                    "--build-only",
                ]
                if reset:
                    cmd.append("--reset")
                if build:
                    cmd.append("--build")
                if verbose:
                    cmd.append("--verbose")

                color_code = COLORS[idx % len(COLORS)]
                if max_workers > 1:
                    message = f"[PARALLEL] Scheduling environment: {profile_name}"
                    if progress_display:
                        progress_display.system(message)
                        progress_display.update_status(profile_name, "queued")
                    else:
                        print(message)
                start = time.monotonic()
                future = executor.submit(
                    run_profile_subprocess,
                    profile_name,
                    cmd,
                    color_code,
                    verbose,
                    max_label_len,
                    progress_display,
                )
                future_to_profile[future] = (profile_name, start, color_code)

            for future in as_completed(future_to_profile):
                profile_name, start, color_code = future_to_profile[future]
                try:
                    returncode, _ = future.result()
                except Exception as e:
                    message = f"[ERROR] Environment {profile_name} FAILED with exception: {e}"
                    if progress_display:
                        progress_display.system(message)
                        progress_display.update_status(profile_name, "FAILED")
                    else:
                        print(message)
                    returncode = 1

                durations[profile_name] = time.monotonic() - start

                label = f"[{profile_name}]"
                if max_label_len > 0:
                    label = label.ljust(max_label_len)
                prefix = f"{color_code}{label}{COLOR_RESET}"

                if returncode == 0:
                    scenario = env_scenarios.get(profile_name, {})
                    project_name = scenario.get("esb_project", BRAND_SLUG)
                    printer = (
                        (lambda line, profile=profile_name: progress_display.log(profile, line))
                        if progress_display
                        else None
                    )
                    print_built_images(
                        profile_name,
                        project_name,
                        prefix="" if progress_display else prefix,
                        duration_seconds=durations[profile_name],
                        printer=printer,
                    )
                    if progress_display:
                        progress_display.update_status(profile_name, "Build done")
                else:
                    failed.append(profile_name)
                    if progress_display:
                        progress_display.update_status(profile_name, "Build failed")
                    if fail_fast:
                        for pending in future_to_profile:
                            pending.cancel()
                        break
    finally:
        if manage_display and progress_display:
            progress_display.stop()

    total_elapsed = time.monotonic() - total_start
    if durations:
        print("\n=== Build Phase Summary ===")
        for profile_name in profiles:
            if profile_name in durations:
                print(f"- {profile_name}: {durations[profile_name]:.1f}s")
        print(f"Total: {total_elapsed:.1f}s\n")

    return failed


def wait_for_gateway(
    env_name: str,
    timeout: float = 60.0,
    interval: float = 1.0,
    verbose: bool = False,
    ports: dict | None = None,
) -> None:
    """
    Waits for the Gateway to be ready by polling its /health endpoint.
    Parity with cli/internal/helpers/wait.go.
    """
    if not ports:
        from e2e.runner.env import load_ports

        ports = load_ports(env_name)

    gw_port = ports.get(env_key("PORT_GATEWAY_HTTPS"))
    if not gw_port:
        if verbose:
            print(f"[WARN] Gateway port not found for {env_name}, skipping readiness wait.")
        return

    url = f"https://localhost:{gw_port}/health"
    if verbose:
        print(f"Waiting for Gateway readiness at {url}...")

    # Suppress certificate warnings for local dev
    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

    deadline = time.time() + timeout
    last_err = None

    while time.time() < deadline:
        try:
            # We use a short timeout for the check itself
            response = requests.get(url, timeout=2.0, verify=False)
            if response.status_code == 200:
                if verbose:
                    print(f"✓ Gateway is ready ({response.status_code})")
                return
            last_err = f"Status code {response.status_code}"
        except requests.exceptions.RequestException as e:
            last_err = str(e)

        time.sleep(interval)

    raise RuntimeError(
        f"Gateway failed to start in time ({timeout}s) for {env_name}. Last error: {last_err}"
    )
