#!/usr/bin/env bash
set -euo pipefail

# Deploy artifact script
# Usage: ./artifacts/deploy.sh [path/to/artifact.yml]

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
DEFAULT_CTL_BIN="esb-ctl"

read_artifact_field() {
  local key="$1"
  local file="$2"
  awk -v k="$key" '
    $1 == k ":" {
      sub(/^[^:]*:[[:space:]]*/, "", $0)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", $0)
      gsub(/^["'\''"]|["'\''"]$/, "", $0)
      print
      exit
    }
  ' "$file"
}

wait_for_registry_ready() {
  local host_port="$1"
  local timeout="$2"
  local started
  local code

  if ! command -v curl >/dev/null 2>&1; then
    echo "curl is required to wait for registry readiness" >&2
    return 1
  fi

  started="$(date +%s)"
  while true; do
    code="$(
      curl -sS -o /dev/null \
        --noproxy '*' \
        --connect-timeout 1 \
        --max-time 2 \
        -w '%{http_code}' \
        "http://${host_port}/v2/" 2>/dev/null || true
    )"
    if [ "$code" = "200" ] || [ "$code" = "401" ]; then
      return 0
    fi

    if [ $(( $(date +%s) - started )) -ge "$timeout" ]; then
      echo "Registry not responding at http://${host_port}/v2/ after ${timeout}s (last_status='${code:-n/a}')" >&2
      docker compose -p "$PROJECT_NAME" \
        "${COMPOSE_ENV_FILE_ARGS[@]}" \
        -f "$COMPOSE_FILE" ps registry >&2 || true
      docker compose -p "$PROJECT_NAME" \
        "${COMPOSE_ENV_FILE_ARGS[@]}" \
        -f "$COMPOSE_FILE" logs --tail=50 registry >&2 || true
      return 1
    fi
    sleep 1
  done
}

ARTIFACT_ARG="${1:-}"
if [ -n "$ARTIFACT_ARG" ]; then
  ARTIFACT="$ARTIFACT_ARG"
else
  mapfile -t ART_FILES < <(find "$REPO_ROOT/artifacts" -type f -name artifact.yml 2>/dev/null | sort)
  if [ ${#ART_FILES[@]} -eq 0 ]; then
    echo "No artifact.yml found under $REPO_ROOT/artifacts. Provide as argument." >&2
    exit 1
  fi
  ARTIFACT="${ART_FILES[0]}"
  if [ ${#ART_FILES[@]} -gt 1 ]; then
    echo "Multiple artifact.yml files found; choose one:"
    for i in "${!ART_FILES[@]}"; do
      idx=$((i + 1))
      printf "%2d) %s\n" "$idx" "${ART_FILES[$i]}"
    done
    while true; do
      read -r -p "Select number (1-${#ART_FILES[@]}) [1]: " sel
      sel=${sel:-1}
      if [[ "$sel" =~ ^[0-9]+$ ]] && [ "$sel" -ge 1 ] && [ "$sel" -le "${#ART_FILES[@]}" ]; then
        ARTIFACT="${ART_FILES[$((sel - 1))]}"
        break
      fi
      echo "Invalid selection"
    done
  fi
fi

if [ ! -f "$ARTIFACT" ]; then
  echo "artifact.yml not found: $ARTIFACT" >&2
  exit 1
fi

echo "Using artifact: $ARTIFACT"

# Parse artifact after final selection.
ARTIFACT_PROJECT="$(read_artifact_field project "$ARTIFACT")"
ARTIFACT_ENV="$(read_artifact_field env "$ARTIFACT")"

# Use root compose only; it includes mode-specific overlays via `include`.
COMPOSE_FILE="$REPO_ROOT/docker-compose.yml"

if [ -f "$REPO_ROOT/.env" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$REPO_ROOT/.env"
  set +a
  COMPOSE_ENV_FILE_ARGS=(--env-file "$REPO_ROOT/.env")
  PROVISION_ENV_FILE_ARGS=(--env-file "$REPO_ROOT/.env")
else
  COMPOSE_ENV_FILE_ARGS=()
  PROVISION_ENV_FILE_ARGS=()
fi

# Artifact values override .env.
BASE_PROJECT="${ARTIFACT_PROJECT:-${PROJECT_NAME:-}}"
RESOLVED_ENV="${ARTIFACT_ENV:-${ENV:-}}"

if [ -z "$BASE_PROJECT" ]; then
  echo "project is empty (artifact project / PROJECT_NAME)" >&2
  exit 1
fi

CTL_BIN="${CTL_BIN:-$DEFAULT_CTL_BIN}"
if ! command -v "$CTL_BIN" >/dev/null 2>&1; then
  echo "ctl command not found: $CTL_BIN" >&2
  echo "Install via 'mise run setup' (or 'mise run build-ctl')," >&2
  echo "or set CTL_BIN to override (example: CTL_BIN=esb-ctl)." >&2
  exit 1
fi

if [ -n "$RESOLVED_ENV" ]; then
  PROJECT_NAME="${BASE_PROJECT}-${RESOLVED_ENV}"
else
  PROJECT_NAME="$BASE_PROJECT"
fi

# Compose project names should be lowercase-safe.
PROJECT_NAME="$(printf '%s' "$PROJECT_NAME" \
  | tr '[:upper:]' '[:lower:]' \
  | sed -E 's/[^a-z0-9_.-]+/-/g; s/^[^a-z0-9]+//; s/[^a-z0-9]+$//')"

if [ -z "$PROJECT_NAME" ]; then
  echo "resolved PROJECT_NAME is empty after normalization" >&2
  exit 1
fi

export PROJECT_NAME
export ENV="$RESOLVED_ENV"

if [ -z "${JWT_SECRET_KEY:-}" ] || [ ${#JWT_SECRET_KEY} -lt 32 ]; then
  echo "JWT_SECRET_KEY must be set and >= 32 chars" >&2
  exit 1
fi

echo "Bringing up stack for project '$PROJECT_NAME' (ENV='${ENV:-}')"

docker compose -p "$PROJECT_NAME" \
  "${COMPOSE_ENV_FILE_ARGS[@]}" \
  -f "$COMPOSE_FILE" up -d

PORT_REGISTRY="${PORT_REGISTRY:-5010}"
if ! [[ "$PORT_REGISTRY" =~ ^[0-9]+$ ]]; then
  echo "PORT_REGISTRY must be numeric, got: '$PORT_REGISTRY'" >&2
  exit 1
fi

if [ "$PORT_REGISTRY" -eq 0 ]; then
  echo "PORT_REGISTRY=0 detected; resolving published host port for registry:5010"
  RESOLVED_PORT="$(
    docker compose -p "$PROJECT_NAME" \
      "${COMPOSE_ENV_FILE_ARGS[@]}" \
      -f "$COMPOSE_FILE" port registry 5010 2>/dev/null \
      | tail -n1
  )"
  PORT_REGISTRY="${RESOLVED_PORT##*:}"
  if ! [[ "$PORT_REGISTRY" =~ ^[0-9]+$ ]] || [ "$PORT_REGISTRY" -eq 0 ]; then
    echo "Failed to resolve published registry port (raw='${RESOLVED_PORT:-}')" >&2
    exit 1
  fi
  echo "Resolved registry host port: $PORT_REGISTRY"
fi

REGISTRY_WAIT_TIMEOUT="${REGISTRY_WAIT_TIMEOUT:-60}"
if ! [[ "$REGISTRY_WAIT_TIMEOUT" =~ ^[0-9]+$ ]] || [ "$REGISTRY_WAIT_TIMEOUT" -le 0 ]; then
  echo "REGISTRY_WAIT_TIMEOUT must be a positive integer, got: '$REGISTRY_WAIT_TIMEOUT'" >&2
  exit 1
fi

echo "Waiting for registry on http://127.0.0.1:$PORT_REGISTRY/v2/"
wait_for_registry_ready "127.0.0.1:${PORT_REGISTRY}" "$REGISTRY_WAIT_TIMEOUT"

echo "Deploying artifact via ${CTL_BIN}"
"$CTL_BIN" deploy --artifact "$ARTIFACT"

echo "Running explicit provision (recommended for restores)"
"$CTL_BIN" provision \
  --project "$PROJECT_NAME" \
  --compose-file "$COMPOSE_FILE" \
  "${PROVISION_ENV_FILE_ARGS[@]}" \
  --project-dir "$REPO_ROOT"

echo "Showing compose ps"
docker compose -p "$PROJECT_NAME" \
  "${COMPOSE_ENV_FILE_ARGS[@]}" \
  -f "$COMPOSE_FILE" ps

echo "Listing runtime config volume contents"
docker run --rm -v "${PROJECT_NAME}_esb-runtime-config:/runtime-config" alpine ls -1 /runtime-config || true

echo "Done."
