#!/usr/bin/env bash
# Where: tools/dind-bundler/dind-e2e-equivalent/run_dind_e2e_equivalent_pytest.sh
# What: Reproduce E2E pytest run against a DinD container built from artifact input.
# Why: Validate DinD self-contained behavior, including restart tests that require docker compose control operations.

set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  ./tools/dind-bundler/dind-e2e-equivalent/run_dind_e2e_equivalent_pytest.sh [options] [-- <extra pytest args>]

Options:
  --artifact-dir <path>      Artifact directory (default: e2e/artifacts/e2e-docker)
  --env-file <path>          Env file (default: e2e/environments/e2e-docker/.env)
  --image-tag <tag>          DinD image tag (default: esb-e2e-dind-repro:latest)
  --container-name <name>    DinD container name (default: esb-e2e-dind-repro)
  --compose-project <name>   Compose project name inside DinD (auto-detect by default)
  --wait-seconds <n>         Wait timeout for gateway health (default: 300)
  --skip-clean               Skip host docker cleanup at start
  --no-prepare-images        Build DinD without --prepare-images
  --keep-container           Keep DinD container after script exits
  -h, --help                 Show this help

Examples:
  ./tools/dind-bundler/dind-e2e-equivalent/run_dind_e2e_equivalent_pytest.sh
  ./tools/dind-bundler/dind-e2e-equivalent/run_dind_e2e_equivalent_pytest.sh --skip-clean -- --maxfail=1 -k s3
USAGE
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: required command not found: $1"
    exit 1
  fi
}

to_abs_path() {
  local path="$1"
  if [ "${path#/}" != "$path" ]; then
    echo "$path"
  else
    echo "$(pwd)/$path"
  fi
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
cd "$REPO_ROOT"

ARTIFACT_DIR="e2e/artifacts/e2e-docker"
ENV_FILE="e2e/environments/e2e-docker/.env"
IMAGE_TAG="esb-e2e-dind-repro:latest"
CONTAINER_NAME="esb-e2e-dind-repro"
COMPOSE_PROJECT=""
WAIT_SECONDS=300

CLEAN_DOCKER=true
PREPARE_IMAGES=true
KEEP_CONTAINER=false

EXTRA_PYTEST_ARGS=()

while [ "$#" -gt 0 ]; do
  case "$1" in
    --artifact-dir)
      ARTIFACT_DIR="${2:-}"
      shift 2
      ;;
    --env-file)
      ENV_FILE="${2:-}"
      shift 2
      ;;
    --image-tag)
      IMAGE_TAG="${2:-}"
      shift 2
      ;;
    --container-name)
      CONTAINER_NAME="${2:-}"
      shift 2
      ;;
    --compose-project)
      COMPOSE_PROJECT="${2:-}"
      shift 2
      ;;
    --wait-seconds)
      WAIT_SECONDS="${2:-}"
      shift 2
      ;;
    --skip-clean)
      CLEAN_DOCKER=false
      shift
      ;;
    --no-prepare-images)
      PREPARE_IMAGES=false
      shift
      ;;
    --keep-container)
      KEEP_CONTAINER=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      EXTRA_PYTEST_ARGS=("$@")
      break
      ;;
    *)
      echo "Error: unknown argument: $1"
      usage
      exit 1
      ;;
  esac
done

require_cmd docker
require_cmd uv
require_cmd curl
require_cmd python3

if [ -z "$ARTIFACT_DIR" ] || [ -z "$ENV_FILE" ] || [ -z "$IMAGE_TAG" ] || [ -z "$CONTAINER_NAME" ]; then
  echo "Error: required option is empty."
  usage
  exit 1
fi

if ! [[ "$WAIT_SECONDS" =~ ^[0-9]+$ ]]; then
  echo "Error: --wait-seconds must be an integer."
  exit 1
fi

ARTIFACT_DIR_ABS="$(to_abs_path "$ARTIFACT_DIR")"
ENV_FILE_ABS="$(to_abs_path "$ENV_FILE")"
if [ ! -d "$ARTIFACT_DIR_ABS" ]; then
  echo "Error: artifact dir not found: $ARTIFACT_DIR_ABS"
  exit 1
fi
if [ ! -f "$ENV_FILE_ABS" ]; then
  echo "Error: env file not found: $ENV_FILE_ABS"
  exit 1
fi

PROXY_DIR=""

cleanup() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    echo
    echo "[ERROR] Reproduction failed. Tail logs from DinD container:"
    docker logs --tail 200 "$CONTAINER_NAME" 2>/dev/null || true
  fi

  if [ -n "$PROXY_DIR" ] && [ -d "$PROXY_DIR" ]; then
    rm -rf "$PROXY_DIR"
  fi

  if ! $KEEP_CONTAINER; then
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "[1/6] Preparing host docker state..."
if $CLEAN_DOCKER; then
  echo "  - Running destructive cleanup: docker system prune -af --volumes"
  docker rm -f $(docker ps -aq) >/dev/null 2>&1 || true
  docker system prune -af --volumes >/dev/null
else
  echo "  - Skipped cleanup (--skip-clean)"
fi

echo "[2/6] Building DinD image from artifact..."
BUILD_CMD=(./tools/dind-bundler/build.sh -a "$ARTIFACT_DIR_ABS" -e "$ENV_FILE_ABS")
if $PREPARE_IMAGES; then
  BUILD_CMD+=(--prepare-images)
fi
BUILD_CMD+=("$IMAGE_TAG")
"${BUILD_CMD[@]}"

echo "[3/6] Starting DinD container..."
docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
docker run --privileged --name "$CONTAINER_NAME" \
  -p 8443:8443 \
  -p 9428:9428 \
  -p 9091:9091 \
  -p 9000:9000 \
  -p 9001:9001 \
  -p 8000:8000 \
  -d "$IMAGE_TAG" >/dev/null

echo "[4/6] Waiting for DinD gateway health..."
if ! timeout "$WAIT_SECONDS" bash -lc 'until curl -skf https://localhost:8443/health >/dev/null; do sleep 2; done'; then
  echo "Error: DinD gateway did not become healthy within ${WAIT_SECONDS}s."
  exit 1
fi

if [ -z "$COMPOSE_PROJECT" ]; then
  COMPOSE_PROJECT="$(docker exec "$CONTAINER_NAME" sh -lc "docker ps --filter label=com.docker.compose.service=gateway --format '{{.Label \"com.docker.compose.project\"}}' | head -n1")"
fi
if [ -z "$COMPOSE_PROJECT" ]; then
  echo "Error: failed to detect compose project in DinD. Pass --compose-project explicitly."
  exit 1
fi
echo "  - Compose project: $COMPOSE_PROJECT"

PROXY_DIR="$(mktemp -d)"
printf '#!/usr/bin/env bash\nset -euo pipefail\nexec /usr/bin/docker exec %q docker "$@"\n' "$CONTAINER_NAME" > "$PROXY_DIR/docker"
chmod +x "$PROXY_DIR/docker"

echo "[5/6] Preparing pytest environment..."
set -a
source "$ENV_FILE_ABS"
set +a

export AUTH_USER="${AUTH_USER:-test-admin}"
export AUTH_PASS="${AUTH_PASS:-test-secure-password}"
export X_API_KEY="${X_API_KEY:-test-api-key}"
export RUSTFS_ACCESS_KEY="${RUSTFS_ACCESS_KEY:-rustfsadmin}"
export RUSTFS_SECRET_KEY="${RUSTFS_SECRET_KEY:-rustfsadmin}"

export MODE=docker
export PROJECT_NAME="$COMPOSE_PROJECT"

export PORT_GATEWAY_HTTPS=8443
export PORT_VICTORIALOGS=9428
export PORT_AGENT_METRICS=9091
export PORT_S3=9000
export PORT_S3_MGMT=9001
export PORT_DATABASE=8000

export GATEWAY_PORT=8443
export GATEWAY_URL=https://localhost:8443
export VICTORIALOGS_PORT=9428
export VICTORIALOGS_URL=http://localhost:9428
export AGENT_METRICS_PORT=9091
export AGENT_METRICS_URL=http://localhost:9091

TEST_TARGETS=(
  e2e/scenarios/smoke/test_connectivity.py
  e2e/scenarios/smoke/test_containerd_network_identity.py
  e2e/scenarios/smoke/test_smoke.py
  e2e/scenarios/autoscaling
  e2e/scenarios/standard
  e2e/scenarios/runtime/java
  e2e/scenarios/runtime/python
  e2e/scenarios/restart
)

echo "[6/6] Running pytest against DinD..."
PATH="$PROXY_DIR:$PATH" uv run python -m pytest \
  --compose-file /app/docker-compose.bundle.yml \
  --import-mode=importlib \
  "${TEST_TARGETS[@]}" \
  -v \
  "${EXTRA_PYTEST_ARGS[@]}"

echo "Done: DinD pytest reproduction finished successfully."
