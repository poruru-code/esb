#!/usr/bin/env python3
import argparse
import getpass
import hashlib
import os
import shutil
import socket
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from types import ModuleType

import toml

try:
    import grp as _grp
except ImportError:  # pragma: no cover - non-POSIX
    grp_module: ModuleType | None = None
else:
    grp_module = _grp

ROOT_CA_CERT_FILENAME = "rootCA.crt"
ROOT_CA_KEY_FILENAME = "rootCA.key"
CONFIG_REL_PATH = "tools/cert-gen/config.toml"


@dataclass(frozen=True)
class RuntimeConfig:
    repo_root: Path
    brand_dir: Path
    config: dict
    cert_cfg: dict
    trust_cfg: dict
    host_cfg: dict
    output_dir: str


@dataclass(frozen=True)
class RootCAPaths:
    cert: str
    key: str


@dataclass(frozen=True)
class LeafPaths:
    server_cert: str
    server_key: str
    client_cert: str
    client_key: str


@dataclass(frozen=True)
class LeafMaterial:
    label: str
    cert_file: str
    key_file: str
    subject: str
    sans: tuple[str, ...]
    validity: str


def get_local_ip():
    try:
        # 8.8.8.8にダミー接続して自身のルートIPを取得 (パケット送信はしない)
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("8.8.8.8", 80))
            return s.getsockname()[0]
    except Exception:
        return "127.0.0.1"


def resolve_step_path() -> str:
    step_path = shutil.which("step")
    if not step_path:
        return ""
    return step_path


def resolve_sudo_path() -> str:
    sudo_path = shutil.which("sudo")
    if not sudo_path:
        return ""
    return sudo_path


def check_step(step_path: str):
    if not step_path:
        print("Error: step CLI not found.")
        print("Please install step via mise or system package manager.")
        exit(1)


def install_root_ca(step_path: str, root_ca_cert: str, output_dir: str, trust_prefix: str):
    print(f"Installing local Root CA (prefix={trust_prefix})...")
    cmd = [step_path, "certificate", "install", "--prefix", trust_prefix, root_ca_cert]
    try:
        subprocess.check_call(cmd)
    except subprocess.CalledProcessError as exc:
        sudo_path = resolve_sudo_path()
        if not sudo_path:
            message = (
                "step certificate install failed and sudo is not available. "
                f"Try running: {step_path} certificate install {root_ca_cert}"
            )
            raise RuntimeError(message) from exc
        print("step certificate install failed, retrying with sudo...")
        env = os.environ.copy()
        subprocess.check_call([sudo_path, "-E", *cmd], env=env)
        ensure_user_ownership(output_dir)


def uninstall_root_ca(
    step_path: str, root_ca_cert: str, output_dir: str, trust_prefix: str
) -> bool:
    candidates: list[str] = []
    if os.path.isfile(root_ca_cert):
        candidates.append(root_ca_cert)

    # Debian/Ubuntu system trust store where step writes installed cert files.
    system_store = Path("/usr/local/share/ca-certificates")
    if system_store.is_dir():
        for path in sorted(system_store.glob(f"{trust_prefix}*.crt")):
            candidates.append(str(path))

    # Preserve order while deduplicating.
    ordered_candidates = list(dict.fromkeys(candidates))
    if not ordered_candidates:
        return False

    removed_any = False
    for candidate in ordered_candidates:
        print(f"Removing previously installed Root CA ({candidate})...")
        commands = [
            [step_path, "certificate", "uninstall", "--prefix", trust_prefix, candidate],
        ]
        commands.append([step_path, "certificate", "uninstall", candidate])
        for cmd in commands:
            try:
                subprocess.check_call(cmd)
                removed_any = True
                break
            except subprocess.CalledProcessError:
                sudo_path = resolve_sudo_path()
                if not sudo_path:
                    continue
                env = os.environ.copy()
                try:
                    subprocess.check_call([sudo_path, "-E", *cmd], env=env)
                    ensure_user_ownership(output_dir)
                    removed_any = True
                    break
                except subprocess.CalledProcessError:
                    continue

    if not removed_any:
        print(
            "Warning: failed to uninstall Root CA from trust store. "
            "Try manually: "
            f"{step_path} certificate uninstall --prefix "
            f"{trust_prefix} {root_ca_cert}",
            file=sys.stderr,
        )
    return removed_any


def is_root_ca_installed(root_ca_cert: str, trust_prefix: str) -> bool:
    if not os.path.isfile(root_ca_cert):
        return False

    system_store = Path("/usr/local/share/ca-certificates")
    if not system_store.is_dir():
        return False

    try:
        root_digest = hashlib.sha256(Path(root_ca_cert).read_bytes()).hexdigest()
    except OSError:
        return False

    for path in sorted(system_store.glob(f"{trust_prefix}*.crt")):
        if not path.is_file():
            continue
        try:
            if hashlib.sha256(path.read_bytes()).hexdigest() == root_digest:
                return True
        except OSError:
            continue
    return False


def normalize_trust_prefix(value: object | None, default: str = "ca") -> str:
    if value is None:
        return default
    text = str(value).strip().lower()
    if text == "":
        return default
    normalized = "".join(ch if (ch.isalnum() or ch in "-_") else "-" for ch in text)
    normalized = normalized.strip("-_")
    if normalized == "":
        return default
    return normalized


def parse_hash_length(value: object | None, default: int = 8) -> int:
    if value is None:
        return default
    text = str(value).strip()
    if text == "":
        return default
    try:
        parsed = int(text)
    except ValueError as exc:
        raise RuntimeError("trust.root_ca_hash_length must be an integer") from exc
    if parsed < 4 or parsed > 32:
        raise RuntimeError("trust.root_ca_hash_length must be between 4 and 32")
    return parsed


def resolve_root_ca_hash(output_dir: str, length: int) -> str:
    normalized = str(Path(output_dir).expanduser().resolve())
    return hashlib.sha256(normalized.encode("utf-8")).hexdigest()[:length]


def resolve_root_ca_subject(output_dir: str, prefix: str, hash_length: int) -> str:
    # Keep CN stable per cert directory while avoiding path disclosure in subject.
    return f"{prefix}-{resolve_root_ca_hash(output_dir, hash_length)}"


def resolve_trust_prefix(output_dir: str, prefix: str, hash_length: int) -> str:
    return f"{prefix}-{resolve_root_ca_hash(output_dir, hash_length)}_"


def collect_hosts(host_cfg: dict, local_ip: str | None = None) -> tuple[list[str], list[str]]:
    # Copy lists to avoid mutating TOML config structures.
    domains = list(host_cfg.get("domains", []))
    ips = list(host_cfg.get("ips", []))

    if host_cfg.get("include_local_ip", False):
        if local_ip is None:
            local_ip = get_local_ip()
        if local_ip and local_ip != "127.0.0.1" and local_ip not in ips:
            ips.append(local_ip)

    return domains, ips


def normalize_validity(value: object | None) -> str | None:
    if value is None:
        return None
    text = str(value).strip()
    return text or None


def require_validity(value: str | None, name: str) -> str:
    if not value:
        raise RuntimeError(f"certificate.{name} is required in config")
    return value


def resolve_subject(domains: list[str], ips: list[str], fallback: str) -> str:
    if domains:
        return domains[0]
    if ips:
        return ips[0]
    return fallback


def dedupe_sans(domains: list[str], ips: list[str], subject: str) -> list[str]:
    sans: list[str] = []
    seen: set[str] = set()
    for entry in [*domains, *ips]:
        if entry not in seen:
            sans.append(entry)
            seen.add(entry)
    if subject and subject not in seen:
        sans.insert(0, subject)
    return sans


def build_step_root_ca_command(
    step_path: str,
    subject: str,
    cert_file: str,
    key_file: str,
    not_after: str | None = None,
    overwrite: bool = False,
) -> list[str]:
    cmd = [
        step_path,
        "certificate",
        "create",
        subject,
        cert_file,
        key_file,
        "--profile",
        "root-ca",
        "--no-password",
        "--insecure",
    ]
    if not_after:
        cmd.extend(["--not-after", not_after])
    if overwrite:
        cmd.append("--force")
    return cmd


def build_step_leaf_command(
    step_path: str,
    subject: str,
    cert_file: str,
    key_file: str,
    sans: list[str],
    ca_cert: str,
    ca_key: str,
    not_after: str | None = None,
    overwrite: bool = False,
) -> list[str]:
    cmd = [
        step_path,
        "certificate",
        "create",
        subject,
        cert_file,
        key_file,
        "--profile",
        "leaf",
        "--ca",
        ca_cert,
        "--ca-key",
        ca_key,
        "--no-password",
        "--insecure",
    ]
    if not_after:
        cmd.extend(["--not-after", not_after])
    for san in sans:
        cmd.extend(["--san", san])
    if overwrite:
        cmd.append("--force")
    return cmd


def generate_root_ca(
    cert_file: str,
    key_file: str,
    step_path: str,
    subject: str,
    not_after: str | None,
    overwrite: bool = False,
) -> None:
    output_dir = os.path.dirname(cert_file)
    cmd = build_step_root_ca_command(
        step_path,
        subject,
        cert_file,
        key_file,
        not_after,
        overwrite=overwrite,
    )

    print(f"Generating root CA in {output_dir}...")
    print(f"Certificate: {os.path.basename(cert_file)}")
    print(f"Subject: {subject}")
    if not_after:
        print(f"Valid for: {not_after}")

    subprocess.check_call(cmd)
    print("Root CA generation complete.")


def generate_leaf_cert(
    cert_file: str,
    key_file: str,
    sans: list[str],
    step_path: str,
    ca_cert: str,
    ca_key: str,
    label: str,
    subject: str,
    not_after: str | None,
    overwrite: bool = False,
) -> None:
    output_dir = os.path.dirname(cert_file)
    cmd = build_step_leaf_command(
        step_path,
        subject,
        cert_file,
        key_file,
        sans,
        ca_cert,
        ca_key,
        not_after,
        overwrite=overwrite,
    )

    print(f"Generating {label} certificate in {output_dir}...")
    print(f"Certificate: {os.path.basename(cert_file)}")
    print(f"Subject: {subject}")
    print(f"SANs: {sans}")
    if not_after:
        print(f"Valid for: {not_after}")

    subprocess.check_call(cmd)
    print(f"{label.capitalize()} certificate generation complete.")


def verify_leaf_cert_with_root(
    step_path: str, cert_file: str, root_ca_cert: str
) -> tuple[bool, str]:
    if not os.path.isfile(cert_file):
        return False, f"certificate file missing: {cert_file}"
    if not os.path.isfile(root_ca_cert):
        return False, f"root CA file missing: {root_ca_cert}"

    cmd = [step_path, "certificate", "verify", "--roots", root_ca_cert, cert_file]
    completed = subprocess.run(cmd, capture_output=True, text=True)
    if completed.returncode == 0:
        return True, ""

    detail = completed.stderr.strip() or completed.stdout.strip() or "verification failed"
    return False, detail


def resolve_host_cfg(config: dict, key: str, fallback: dict) -> dict:
    host_cfg = config.get(key)
    return fallback if not host_cfg else host_cfg


def resolve_repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def resolve_brand_home_dir() -> str:
    repo_root = resolve_repo_root()
    repo_root_str = str(repo_root)
    if repo_root_str not in sys.path:
        sys.path.insert(0, repo_root_str)

    from e2e.runner.branding import brand_home_dir

    return brand_home_dir()


def ensure_output_dir(output_dir: str) -> None:
    expanded = os.path.expanduser(output_dir)
    owner_hint = str(Path(expanded).parent)
    try:
        os.makedirs(expanded, exist_ok=True)
    except PermissionError as exc:
        if not attempt_fix_permissions(owner_hint):
            raise RuntimeError(
                f"Certificate output directory is not writable: {expanded}. "
                f"Fix ownership (e.g. sudo chown -R $USER:$USER {owner_hint}) and retry."
            ) from exc
    if not os.access(expanded, os.W_OK):
        if not attempt_fix_permissions(owner_hint):
            raise RuntimeError(
                f"Certificate output directory is not writable: {expanded}. "
                f"Fix ownership (e.g. sudo chown -R $USER:$USER {owner_hint}) and retry."
            )
    if not os.access(expanded, os.W_OK):
        raise RuntimeError(
            f"Certificate output directory is not writable: {expanded}. "
            f"Fix ownership (e.g. sudo chown -R $USER:$USER {owner_hint}) and retry."
        )


def current_user_group() -> tuple[str, str]:
    user = os.environ.get("USER") or getpass.getuser()
    group = ""
    if grp_module is not None:
        try:
            group = grp_module.getgrgid(os.getgid()).gr_name
        except KeyError:
            group = ""
    if not group:
        group = user
    return user, group


def current_user_chown_spec() -> str:
    getuid = getattr(os, "getuid", None)
    getgid = getattr(os, "getgid", None)
    if callable(getuid) and callable(getgid):
        return f"{getuid()}:{getgid()}"
    user, group = current_user_group()
    return f"{user}:{group}"


def attempt_fix_permissions(path: str) -> bool:
    sudo_path = resolve_sudo_path()
    if not sudo_path:
        return False
    user_spec = current_user_chown_spec()
    print(f"Attempting to fix permissions with sudo for {path}...")
    try:
        subprocess.check_call([sudo_path, "chown", "-R", user_spec, "--", path])
    except subprocess.CalledProcessError:
        return False
    return True


def ensure_user_ownership(output_dir: str) -> None:
    expanded = os.path.expanduser(output_dir)
    if not os.path.exists(expanded):
        return

    needs_fix = not os.access(expanded, os.W_OK)
    scan_error = False

    def onerror(_: OSError) -> None:
        nonlocal scan_error
        scan_error = True

    if not needs_fix:
        for root, dirs, _files in os.walk(expanded, onerror=onerror):
            for name in dirs:
                candidate = os.path.join(root, name)
                if not os.access(candidate, os.W_OK):
                    needs_fix = True
                    break
            if needs_fix:
                break

    if not needs_fix and not scan_error:
        return
    if not attempt_fix_permissions(expanded):
        print(
            f"Warning: failed to fix ownership/permissions under {expanded}. "
            "Some cleanup operations may require sudo.",
            file=sys.stderr,
        )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Generate development certificates using step-cli")
    parser.add_argument(
        "--force",
        action="store_true",
        help="Force regeneration of certificates and CA installation",
    )
    parser.add_argument(
        "--no-trust-install",
        dest="no_trust_install",
        action="store_true",
        help="Skip local trust-store installation of the generated Root CA",
    )
    parser.add_argument(
        "--show-output-dir",
        dest="show_output_dir",
        action="store_true",
        help="Print resolved certificate output directory and exit",
    )
    return parser


def parse_args() -> argparse.Namespace:
    return build_parser().parse_args()


def load_generation_config(config_path: Path) -> dict:
    if config_path.exists():
        return toml.load(config_path)
    return {}


def resolve_output_dir(cert_cfg: dict, brand_dir: Path) -> str:
    # Derivation logic for output_dir:
    # 1. config.toml [certificate] output_dir
    # 2. CERT_DIR env
    # 3. Repo root: <repo_root>/.<brand>/certs
    output_dir = cert_cfg.get("output_dir") or os.environ.get("CERT_DIR")
    if not output_dir:
        output_dir = str(brand_dir / "certs")
    return os.path.expanduser(output_dir)


def resolve_runtime_config() -> RuntimeConfig:
    repo_root = resolve_repo_root()
    brand_dir = repo_root / resolve_brand_home_dir()

    config_path = repo_root / CONFIG_REL_PATH
    config = load_generation_config(config_path)

    cert_cfg = config.get("certificate", {})
    trust_cfg = config.get("trust", {})
    host_cfg = config.get("hosts", {})

    output_dir = resolve_output_dir(cert_cfg, brand_dir)
    return RuntimeConfig(
        repo_root=repo_root,
        brand_dir=brand_dir,
        config=config,
        cert_cfg=cert_cfg,
        trust_cfg=trust_cfg,
        host_cfg=host_cfg,
        output_dir=output_dir,
    )


def prepare_runtime_environment(output_dir: str) -> None:
    # Force CAROOT to follow branding output_dir to avoid stale env vars.
    if os.environ.get("CAROOT") and os.environ["CAROOT"] != output_dir:
        print(f"Overriding CAROOT to {output_dir} (was {os.environ['CAROOT']}).")
    os.environ["CAROOT"] = output_dir
    ensure_output_dir(output_dir)


def resolve_root_ca_paths(caroot: str) -> RootCAPaths:
    return RootCAPaths(
        cert=os.path.join(caroot, ROOT_CA_CERT_FILENAME),
        key=os.path.join(caroot, ROOT_CA_KEY_FILENAME),
    )


def fail_directory_mount(path: str) -> None:
    print(f"ERROR: {path} is a directory, but it should be a file.")
    print("This often happens when Docker mistakenly creates a directory for a file mount.")
    print(f"Please run: sudo rm -rf {path}")
    sys.exit(1)


def validate_root_ca_paths(paths: RootCAPaths) -> None:
    # Safety check: Docker sometimes creates a directory if the mount target is missing.
    if os.path.isdir(paths.cert):
        fail_directory_mount(paths.cert)
    if os.path.isdir(paths.key):
        fail_directory_mount(paths.key)


def resolve_validities(cert_cfg: dict) -> tuple[str, str, str]:
    ca_validity = require_validity(normalize_validity(cert_cfg.get("ca_validity")), "ca_validity")
    server_validity = require_validity(
        normalize_validity(cert_cfg.get("server_validity")), "server_validity"
    )
    client_validity = require_validity(
        normalize_validity(cert_cfg.get("client_validity")), "client_validity"
    )
    return ca_validity, server_validity, client_validity


def resolve_trust_details(trust_cfg: dict, output_dir: str) -> tuple[str, str]:
    trust_prefix_base = normalize_trust_prefix(trust_cfg.get("root_ca_prefix"), default="ca")
    trust_hash_length = parse_hash_length(trust_cfg.get("root_ca_hash_length"), default=8)
    root_ca_subject = resolve_root_ca_subject(output_dir, trust_prefix_base, trust_hash_length)
    trust_prefix = resolve_trust_prefix(output_dir, trust_prefix_base, trust_hash_length)
    return root_ca_subject, trust_prefix


def ensure_root_ca_state(
    step_path: str,
    paths: RootCAPaths,
    output_dir: str,
    root_ca_subject: str,
    ca_validity: str,
    trust_prefix: str,
    force: bool,
    skip_root_ca_install: bool,
) -> bool:
    root_ca_regenerated = False
    root_cert_exists = os.path.exists(paths.cert)
    root_key_exists = os.path.exists(paths.key)
    if force or not (root_cert_exists and root_key_exists):
        overwrite_root = force or root_cert_exists or root_key_exists
        generate_root_ca(
            paths.cert,
            paths.key,
            step_path,
            root_ca_subject,
            ca_validity,
            overwrite=overwrite_root,
        )
        root_ca_regenerated = True
        if skip_root_ca_install:
            print("Skipping local Root CA install (--no-trust-install).")
        else:
            # Uninstall first using stable prefix after regeneration
            # so git-cleaned trees still clean up.
            uninstall_root_ca(step_path, paths.cert, output_dir, trust_prefix)
            install_root_ca(step_path, paths.cert, output_dir, trust_prefix)
    else:
        print(f"Root CA exists at {paths.cert}. Skipping generation. Use --force to regenerate.")
        if not skip_root_ca_install:
            if is_root_ca_installed(paths.cert, trust_prefix):
                print(f"Root CA trust already installed (prefix={trust_prefix}).")
            else:
                print("Root CA trust entry is missing or stale. Installing...")
                uninstall_root_ca(step_path, paths.cert, output_dir, trust_prefix)
                install_root_ca(step_path, paths.cert, output_dir, trust_prefix)

    return root_ca_regenerated


def resolve_leaf_paths(output_dir: str, cert_cfg: dict) -> LeafPaths:
    return LeafPaths(
        server_cert=os.path.join(output_dir, cert_cfg.get("filename_cert", "server.crt")),
        server_key=os.path.join(output_dir, cert_cfg.get("filename_key", "server.key")),
        client_cert=os.path.join(output_dir, cert_cfg.get("filename_client_cert", "client.crt")),
        client_key=os.path.join(output_dir, cert_cfg.get("filename_client_key", "client.key")),
    )


def resolve_leaf_materials(
    config: dict,
    host_cfg: dict,
    paths: LeafPaths,
    server_validity: str,
    client_validity: str,
) -> tuple[LeafMaterial, LeafMaterial]:
    server_domains, server_ips = collect_hosts(host_cfg)
    client_host_cfg = resolve_host_cfg(config, "client_hosts", host_cfg)
    client_domains, client_ips = collect_hosts(client_host_cfg)

    server_subject = resolve_subject(server_domains, server_ips, "localhost")
    client_subject = resolve_subject(client_domains, client_ips, "client")
    server_sans = tuple(dedupe_sans(server_domains, server_ips, server_subject))
    client_sans = tuple(dedupe_sans(client_domains, client_ips, client_subject))

    server_leaf = LeafMaterial(
        label="server",
        cert_file=paths.server_cert,
        key_file=paths.server_key,
        subject=server_subject,
        sans=server_sans,
        validity=server_validity,
    )
    client_leaf = LeafMaterial(
        label="client",
        cert_file=paths.client_cert,
        key_file=paths.client_key,
        subject=client_subject,
        sans=client_sans,
        validity=client_validity,
    )
    return server_leaf, client_leaf


def ensure_leaf_state(
    leaf: LeafMaterial,
    output_dir: str,
    step_path: str,
    root_ca_cert: str,
    root_ca_key: str,
    force: bool,
    root_ca_regenerated: bool,
) -> None:
    leaf_cert_exists = os.path.exists(leaf.cert_file)
    leaf_key_exists = os.path.exists(leaf.key_file)
    needs_regen = force or root_ca_regenerated or not (leaf_cert_exists and leaf_key_exists)
    if not needs_regen:
        verified, detail = verify_leaf_cert_with_root(step_path, leaf.cert_file, root_ca_cert)
        if not verified:
            print(
                f"{leaf.label.capitalize()} certificate does not match current Root CA. "
                f"Regenerating ({detail})."
            )
            needs_regen = True

    if needs_regen:
        overwrite_leaf = force or leaf_cert_exists or leaf_key_exists
        generate_leaf_cert(
            leaf.cert_file,
            leaf.key_file,
            list(leaf.sans),
            step_path,
            root_ca_cert,
            root_ca_key,
            leaf.label,
            leaf.subject,
            leaf.validity,
            overwrite=overwrite_leaf,
        )
        return

    print(
        f"{leaf.label.capitalize()} certificates exist at {output_dir}. "
        "Skipping generation. Use --force to regenerate."
    )


def main() -> int:
    args = parse_args()
    runtime = resolve_runtime_config()
    if args.show_output_dir:
        print(runtime.output_dir)
        return 0

    prepare_runtime_environment(runtime.output_dir)
    step_path = resolve_step_path()
    check_step(step_path)

    root_paths = resolve_root_ca_paths(os.environ["CAROOT"])
    validate_root_ca_paths(root_paths)

    ca_validity, server_validity, client_validity = resolve_validities(runtime.cert_cfg)
    root_ca_subject, trust_prefix = resolve_trust_details(runtime.trust_cfg, runtime.output_dir)

    root_ca_regenerated = ensure_root_ca_state(
        step_path=step_path,
        paths=root_paths,
        output_dir=runtime.output_dir,
        root_ca_subject=root_ca_subject,
        ca_validity=ca_validity,
        trust_prefix=trust_prefix,
        force=args.force,
        skip_root_ca_install=args.no_trust_install,
    )

    leaf_paths = resolve_leaf_paths(runtime.output_dir, runtime.cert_cfg)
    server_leaf, client_leaf = resolve_leaf_materials(
        config=runtime.config,
        host_cfg=runtime.host_cfg,
        paths=leaf_paths,
        server_validity=server_validity,
        client_validity=client_validity,
    )

    ensure_leaf_state(
        leaf=server_leaf,
        output_dir=runtime.output_dir,
        step_path=step_path,
        root_ca_cert=root_paths.cert,
        root_ca_key=root_paths.key,
        force=args.force,
        root_ca_regenerated=root_ca_regenerated,
    )
    ensure_leaf_state(
        leaf=client_leaf,
        output_dir=runtime.output_dir,
        step_path=step_path,
        root_ca_cert=root_paths.cert,
        root_ca_key=root_paths.key,
        force=args.force,
        root_ca_regenerated=root_ca_regenerated,
    )

    # Ensure repo-scoped brand dir is writable after sudo operations.
    ensure_user_ownership(str(runtime.brand_dir))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
