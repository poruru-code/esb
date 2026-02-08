import os
import subprocess
import sys
from pathlib import Path

# Windows path handling might require specific check
PROJECT_ROOT = Path(__file__).resolve().parent.parent
PROTO_DIR = PROJECT_ROOT / "proto"
GO_OUT_DIR = PROJECT_ROOT / "services/agent/pkg/api/v1"
PYTHON_OUT_DIR = PROJECT_ROOT / "services/gateway/pb"


def run_command(cmd):
    print(f"Running: {' '.join(cmd)}")
    subprocess.check_call(cmd, cwd=PROJECT_ROOT)


def fix_python_imports():
    # Fix generated imports for Python
    # 'import agent_pb2' -> 'from . import agent_pb2'
    for file in PYTHON_OUT_DIR.glob("*.py"):
        if file.name == "__init__.py":
            continue
        with open(file, "r", encoding="utf-8") as f:
            content = f.read()

        # Simple replacement for common case
        new_content = content.replace("import agent_pb2", "from . import agent_pb2")

        with open(file, "w", encoding="utf-8") as f:
            f.write(new_content)


def gen_python():
    # Python output needs to be adjusted so that imports work correctly.
    # Running from project root.
    cmd = [
        sys.executable,
        "-m",
        "grpc_tools.protoc",
        "-Iproto",
        f"--python_out={PYTHON_OUT_DIR}",
        f"--grpc_python_out={PYTHON_OUT_DIR}",
        "agent.proto",
    ]
    run_command(cmd)

    fix_python_imports()

    # Create __init__.py if not exists
    init_file = PYTHON_OUT_DIR / "__init__.py"
    if not init_file.exists():
        with open(init_file, "w") as f:
            f.write("")


def gen_go_docker():
    # Use Docker to generate Go code to avoid requiring local Go setup
    # rvolosatovs/protoc contains protoc-gen-go and protoc-gen-go-grpc

    # Resolve absolute path for Docker volume mount
    # On Windows, Path object might return backslashes, Docker needs consistent format?
    # Usually Windows Docker Desktop handles "C:\Users\..." fine.

    workspace_path = str(PROJECT_ROOT)

    cmd = [
        "docker",
        "run",
        "--rm",
        "-v",
        f"{workspace_path}:/workspace",
        "-w",
        "/workspace",
        "rvolosatovs/protoc:latest",
        "--proto_path=proto",
        "--go_out=services/agent/pkg/api/v1",
        "--go_opt=paths=source_relative",
        "--go-grpc_out=services/agent/pkg/api/v1",
        "--go-grpc_opt=paths=source_relative",
        "agent.proto",
    ]
    run_command(cmd)


if __name__ == "__main__":
    print(f"Project Root: {PROJECT_ROOT}")
    os.makedirs(GO_OUT_DIR, exist_ok=True)
    os.makedirs(PYTHON_OUT_DIR, exist_ok=True)

    print("Generating Python code...")
    try:
        gen_python()
        print("Python code generated.")
    except Exception as e:
        print(f"Failed to generate Python code: {e}")
        sys.exit(1)

    print("Generating Go code (via Docker)...")
    try:
        gen_go_docker()
        print("Go code generated.")
    except Exception as e:
        print(f"Failed to generate Go code: {e}")
        print("Ensure Docker is running and you have internet access to pull the image.")
        sys.exit(1)
