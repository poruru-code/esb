import subprocess
from pathlib import Path
from typing import List, Optional

# Project root
# Assuming this file is in e2e/runner/utils.py, parent.parent is "e2e", parent.parent.parent is root.
PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
GO_CLI_ROOT = PROJECT_ROOT / "cli"
E2E_STATE_ROOT = PROJECT_ROOT / "e2e" / "fixtures" / ".esb"


def resolve_env_file_path(env_file: Optional[str]) -> Optional[str]:
    if not env_file:
        return None
    env_file_path = Path(env_file)
    if not env_file_path.is_absolute():
        env_file_path = PROJECT_ROOT / env_file_path
    return str(env_file_path.absolute())


def build_esb_cmd(args: List[str], env_file: Optional[str]) -> List[str]:
    base_cmd = ["go", "run", "./cmd/esb"]
    env_file_path = resolve_env_file_path(env_file)
    if env_file_path:
        base_cmd.extend(["--env-file", env_file_path])
    return base_cmd + args


def run_esb(
    args: List[str], check: bool = True, env_file: Optional[str] = None, verbose: bool = False
) -> subprocess.CompletedProcess:
    """Helper to run the esb CLI."""
    if verbose and "build" in args:
        # Build command has its own verbose flag
        if "--verbose" not in args and "-v" not in args:
            args = ["build", "--verbose"] + [a for a in args if a != "build"]

    cmd = build_esb_cmd(args, env_file)
    if verbose:
        print(f"Running: {' '.join(cmd)}")
    return subprocess.run(cmd, cwd=GO_CLI_ROOT, check=check, stdin=subprocess.DEVNULL)
