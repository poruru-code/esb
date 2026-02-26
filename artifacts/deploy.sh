#!/usr/bin/env bash
set -euo pipefail

# Deploy artifact script
# Usage: ./artifacts/deploy.sh [path/to/artifact.yml]

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

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

ARTIFACT_ARG="${1:-}"
if [ -n "$ARTIFACT_ARG" ]; then
  ARTIFACT="$ARTIFACT_ARG"
else
  ARTIFACT="$(find "$REPO_ROOT/artifacts" -maxdepth 3 -type f -name artifact.yml 2>/dev/null | head -n1 || true)"
  if [ -z "$ARTIFACT" ]; then
    echo "No artifact.yml found under $REPO_ROOT/artifacts. Provide as argument." >&2
    exit 1
  fi
fi

if [ ! -f "$ARTIFACT" ]; then
  echo "artifact.yml not found: $ARTIFACT" >&2
  exit 1
fi

if [ -z "${ARTIFACT_ARG:-}" ]; then
  mapfile -t ART_FILES < <(find "$REPO_ROOT/artifacts" -type f -name artifact.yml 2>/dev/null | sort)
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

echo "Using artifact: $ARTIFACT"

# Parse artifact after final selection.
ARTIFACT_PROJECT="$(read_artifact_field project "$ARTIFACT")"
ARTIFACT_ENV="$(read_artifact_field env "$ARTIFACT")"

# Always use docker-mode compose overlay.
COMPOSE_FILE="$REPO_ROOT/docker-compose.docker.yml"

if [ -f "$REPO_ROOT/.env" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$REPO_ROOT/.env"
  set +a
fi

# Artifact values override .env.
BASE_PROJECT="${ARTIFACT_PROJECT:-${PROJECT_NAME:-}}"
RESOLVED_ENV="${ARTIFACT_ENV:-${ENV:-}}"

if [ -z "$BASE_PROJECT" ]; then
  echo "project is empty (artifact project / PROJECT_NAME)" >&2
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
  --env-file "$REPO_ROOT/.env" \
  -f "$REPO_ROOT/docker-compose.yml" \
  -f "$COMPOSE_FILE" up -d

PORT_REGISTRY="${PORT_REGISTRY:-5010}"
echo "Waiting for registry on http://127.0.0.1:$PORT_REGISTRY/v2/"
until curl -fsS "http://127.0.0.1:${PORT_REGISTRY}/v2/" >/dev/null 2>&1; do sleep 1; done

echo "Deploying artifact via esb-ctl"
esb-ctl deploy --artifact "$ARTIFACT"

echo "Running explicit provision (recommended for restores)"
esb-ctl provision \
  --project "$PROJECT_NAME" \
  --compose-file "$REPO_ROOT/docker-compose.yml,$COMPOSE_FILE" \
  --env-file "$REPO_ROOT/.env" \
  --project-dir "$REPO_ROOT"

echo "Showing compose ps"
docker compose -p "$PROJECT_NAME" \
  --env-file "$REPO_ROOT/.env" \
  -f "$REPO_ROOT/docker-compose.yml" \
  -f "$COMPOSE_FILE" ps

echo "Listing runtime config volume contents"
docker run --rm -v "${PROJECT_NAME}_esb-runtime-config:/runtime-config" alpine ls -1 /runtime-config || true

echo "Done."
