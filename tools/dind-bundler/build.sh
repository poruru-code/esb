#!/usr/bin/env bash
# Where: tools/dind-bundler/build.sh
# What: Build a self-contained DinD bundle image from existing artifact directories.
# Why: Package prebuilt images/runtime-config without depending on ESB CLI at bundle time.

set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  ./tools/dind-bundler/build.sh -a <artifact-dir> [-a <artifact-dir> ...] -e <env-file> [-c <compose-file>] [--prepare-images] [output-image-tag]

Examples:
  ./tools/dind-bundler/build.sh -a e2e/artifacts/e2e-docker -e e2e/environments/e2e-docker/.env my-esb-bundle:latest
  ./tools/dind-bundler/build.sh -a artifacts/a -a artifacts/b -e environments/prod/.env -c environments/prod/docker-compose.yml --prepare-images my-esb-bundle:latest
USAGE
}

BUILD_DIR="tools/dind-bundler/build-context"
OUTPUT_TAG=""
ARTIFACT_DIRS=()
ENV_FILE=""
COMPOSE_FILE=""
PREPARE_IMAGES=false
POSITIONAL=()

RUNTIME_CONFIG_SOURCE_DIRS=()
FUNCTION_IMAGES=()
COMPOSE_IMAGES=()
ALL_IMAGES=()
MISSING_IMAGES=()

ARTIFACT_PROJECT=""
ARTIFACT_MODE=""
ARTIFACT_ENV=""

LOCAL_REGISTRY_CONTAINER_PREFIX="dind-bundler-local-registry"
LOCAL_REGISTRY_IMAGE="registry:2"

declare -A SEEN_RUNTIME
declare -A SEEN_FUNCTION_IMAGE
declare -A SEEN_IMAGE
declare -A FUNCTION_CONTEXT_BY_IMAGE
declare -A FUNCTION_DOCKERFILE_BY_IMAGE
declare -A PREPARED_BASE_IMAGE
declare -A PUSHED_LOCAL_REGISTRY_IMAGE
declare -A READY_LOCAL_REGISTRY

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: required command not found: $1"
    exit 1
  fi
}

expand_home() {
  case "$1" in
    "~")
      echo "$HOME"
      ;;
    "~"/*)
      echo "$HOME/${1#~/}"
      ;;
    *)
      echo "$1"
      ;;
  esac
}

to_abs_path() {
  local value="$1"
  local expanded
  expanded="$(expand_home "$value")"
  if [ "${expanded#/}" != "$expanded" ]; then
    echo "$expanded"
  else
    echo "$(pwd)/$expanded"
  fi
}

read_env_file_value() {
  local env_file="$1"
  local key="$2"
  python3 - <<'PY' "$env_file" "$key"
import sys

env_file = sys.argv[1]
target = sys.argv[2]

with open(env_file, "r", encoding="utf-8") as handle:
    for raw in handle:
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        name, value = line.split("=", 1)
        if name.strip() != target:
            continue
        value = value.strip()
        if len(value) >= 2 and ((value[0] == value[-1] == '"') or (value[0] == value[-1] == "'")):
            value = value[1:-1]
        print(value)
        break
PY
}

add_unique_function_image() {
  local image="$1"
  if [ -z "$image" ]; then
    return
  fi
  if [ -z "${SEEN_FUNCTION_IMAGE[$image]:-}" ]; then
    FUNCTION_IMAGES+=("$image")
    SEEN_FUNCTION_IMAGE["$image"]=1
  fi
}

register_function_build() {
  local image="$1"
  local context_dir="$2"
  local dockerfile_rel="$3"
  if [ -z "$image" ] || [ -z "$context_dir" ] || [ -z "$dockerfile_rel" ]; then
    return
  fi
  if [ -z "${FUNCTION_CONTEXT_BY_IMAGE[$image]:-}" ]; then
    FUNCTION_CONTEXT_BY_IMAGE["$image"]="$context_dir"
    FUNCTION_DOCKERFILE_BY_IMAGE["$image"]="$dockerfile_rel"
  fi
}

add_unique_image() {
  local image="$1"
  if [ -z "$image" ]; then
    return
  fi
  if [ -z "${SEEN_IMAGE[$image]:-}" ]; then
    ALL_IMAGES+=("$image")
    SEEN_IMAGE["$image"]=1
  fi
}

local_registry_host_port_from_ref() {
  local ref="$1"
  case "$ref" in
    127.0.0.1:[0-9]*/*)
      echo "${ref%%/*}"
      ;;
    localhost:[0-9]*/*)
      echo "${ref%%/*}"
      ;;
    *)
      return 1
      ;;
  esac
}

registry_ping() {
  local host_port="$1"
  python3 - <<'PY' "$host_port"
import sys
import urllib.request

host_port = sys.argv[1]
url = f"http://{host_port}/v2/"

try:
    with urllib.request.urlopen(url, timeout=1) as response:
        status = int(response.status)
        if status in (200, 401):
            sys.exit(0)
except Exception:
    pass

sys.exit(1)
PY
}

wait_for_local_registry() {
  local host_port="$1"
  local i
  for i in $(seq 1 30); do
    if registry_ping "$host_port"; then
      return
    fi
    sleep 1
  done
  echo "Error: local registry ${host_port} did not become ready in time."
  exit 1
}

ensure_local_registry() {
  local host_port="$1"
  if [ -n "${READY_LOCAL_REGISTRY[$host_port]:-}" ]; then
    return
  fi

  if registry_ping "$host_port"; then
    READY_LOCAL_REGISTRY["$host_port"]=1
    return
  fi

  local port="${host_port##*:}"
  local container_name="${LOCAL_REGISTRY_CONTAINER_PREFIX}-${port}"

  if docker ps -a --format '{{.Names}}' | grep -qx "$container_name"; then
    docker start "$container_name" >/dev/null
    wait_for_local_registry "$host_port"
    READY_LOCAL_REGISTRY["$host_port"]=1
    return
  fi

  if ! docker run -d --name "$container_name" -p "${port}:5000" "$LOCAL_REGISTRY_IMAGE" >/dev/null; then
    if registry_ping "$host_port"; then
      READY_LOCAL_REGISTRY["$host_port"]=1
      return
    fi
    echo "Error: failed to start local registry container '${container_name}'."
    exit 1
  fi

  wait_for_local_registry "$host_port"
  READY_LOCAL_REGISTRY["$host_port"]=1
}

push_to_local_registry_if_needed() {
  local ref="$1"
  local host_port=""
  if ! host_port="$(local_registry_host_port_from_ref "$ref")"; then
    return
  fi
  if [ -n "${PUSHED_LOCAL_REGISTRY_IMAGE[$ref]:-}" ]; then
    return
  fi
  if ! docker image inspect "$ref" >/dev/null 2>&1; then
    echo "Error: image not found for local registry push: $ref"
    exit 1
  fi

  ensure_local_registry "$host_port"
  echo "Publishing local-registry image for buildx compatibility: $ref"
  docker push "$ref" >/dev/null
  PUSHED_LOCAL_REGISTRY_IMAGE["$ref"]=1
}

collect_missing_images() {
  MISSING_IMAGES=()
  local image
  for image in "${ALL_IMAGES[@]}"; do
    if ! docker image inspect "$image" >/dev/null 2>&1; then
      MISSING_IMAGES+=("$image")
    fi
  done
}

build_with_buildx() {
  local tag="$1"
  local dockerfile="$2"
  local context_dir="$3"
  echo "Preparing image via buildx: $tag"
  docker buildx build --platform linux/amd64 --load --pull --tag "$tag" --file "$dockerfile" "$context_dir"
}

prepare_fixture_image_if_known() {
  local ref="$1"
  local repo="${ref##*/}"
  repo="${repo%%:*}"
  local fixture_dir=""
  case "$repo" in
    esb-e2e-lambda-python)
      fixture_dir="$(pwd)/e2e/fixtures/images/lambda/python"
      ;;
    esb-e2e-lambda-java)
      fixture_dir="$(pwd)/e2e/fixtures/images/lambda/java"
      ;;
    *)
      return 0
      ;;
  esac
  if [ ! -d "$fixture_dir" ]; then
    return 0
  fi
  build_with_buildx "$ref" "$fixture_dir/Dockerfile" "$fixture_dir"
}

ensure_base_image_available() {
  local ref="$1"
  if [ -z "$ref" ]; then
    return
  fi
  if docker image inspect "$ref" >/dev/null 2>&1; then
    push_to_local_registry_if_needed "$ref"
    return
  fi
  if [ -n "${PREPARED_BASE_IMAGE[$ref]:-}" ]; then
    push_to_local_registry_if_needed "$ref"
    return
  fi

  PREPARED_BASE_IMAGE["$ref"]=1
  local runtime_hooks_dockerfile="$(pwd)/runtime-hooks/python/docker/Dockerfile"
  if [[ "$ref" == *"/esb-lambda-base:"* || "$ref" == esb-lambda-base:* ]]; then
    if [ ! -f "$runtime_hooks_dockerfile" ]; then
      echo "Error: runtime hooks dockerfile not found: $runtime_hooks_dockerfile"
      exit 1
    fi
    build_with_buildx "$ref" "$runtime_hooks_dockerfile" "$(pwd)"
  else
    prepare_fixture_image_if_known "$ref" || true
  fi

  if ! docker image inspect "$ref" >/dev/null 2>&1; then
    docker pull "$ref" >/dev/null 2>&1 || true
  fi
  if ! docker image inspect "$ref" >/dev/null 2>&1; then
    echo "Error: required base image not available: $ref"
    exit 1
  fi
  push_to_local_registry_if_needed "$ref"
}

ensure_dockerfile_bases() {
  local dockerfile_path="$1"
  local ref
  while IFS= read -r ref; do
    if [ -n "$ref" ]; then
      ensure_base_image_available "$ref"
    fi
  done < <(
    awk '
      tolower($1) == "from" {
        for (i = 2; i <= NF; i++) {
          if ($i ~ /^--/) { continue }
          print $i
          break
        }
      }
    ' "$dockerfile_path"
  )
}

prepare_missing_function_images() {
  local image
  for image in "${FUNCTION_IMAGES[@]}"; do
    if docker image inspect "$image" >/dev/null 2>&1; then
      continue
    fi

    local context_dir="${FUNCTION_CONTEXT_BY_IMAGE[$image]:-}"
    local dockerfile_rel="${FUNCTION_DOCKERFILE_BY_IMAGE[$image]:-}"
    if [ -z "$context_dir" ] || [ -z "$dockerfile_rel" ]; then
      continue
    fi

    local dockerfile_path="$context_dir/$dockerfile_rel"
    if [ ! -f "$dockerfile_path" ]; then
      continue
    fi

    ensure_dockerfile_bases "$dockerfile_path"
    build_with_buildx "$image" "$dockerfile_path" "$context_dir"
  done
}

prepare_images() {
  echo "Preparing missing images (--prepare-images)..."
  docker compose --env-file "$ENV_FILE_ABS" -f "$COMPOSE_FILE_ABS" --profile deploy build
  docker compose --env-file "$ENV_FILE_ABS" -f "$COMPOSE_FILE_ABS" --profile deploy pull --ignore-pull-failures || true
  docker pull "$LOCAL_REGISTRY_IMAGE" >/dev/null 2>&1 || true
  prepare_missing_function_images
}

resolve_compose_file() {
  if [ -n "$COMPOSE_FILE" ]; then
    COMPOSE_FILE_ABS="$(to_abs_path "$COMPOSE_FILE")"
  else
    local env_dir
    env_dir="$(dirname "$ENV_FILE_ABS")"
    local env_compose="${env_dir}/docker-compose.yml"
    if [ -f "$env_compose" ]; then
      COMPOSE_FILE_ABS="$env_compose"
    else
      case "$ARTIFACT_MODE" in
        docker)
          COMPOSE_FILE_ABS="$(pwd)/docker-compose.docker.yml"
          ;;
        containerd)
          COMPOSE_FILE_ABS="$(pwd)/docker-compose.containerd.yml"
          ;;
        *)
          echo "Error: unsupported artifact mode for compose fallback: ${ARTIFACT_MODE}"
          exit 1
          ;;
      esac
    fi
  fi

  if [ ! -f "$COMPOSE_FILE_ABS" ]; then
    echo "Error: compose file not found: $COMPOSE_FILE_ABS"
    exit 1
  fi
}

merge_runtime_configs() {
  RUNTIME_CONFIG_DIR="$BUILD_DIR/runtime-config"
  mkdir -p "$RUNTIME_CONFIG_DIR"

  local runtime_source
  for runtime_source in "${RUNTIME_CONFIG_SOURCE_DIRS[@]}"; do
    if [ ! -d "$runtime_source" ]; then
      echo "Error: runtime config source not found: $runtime_source"
      exit 1
    fi
    echo "Merging runtime config from: $runtime_source"

    local src rel dest
    while IFS= read -r src; do
      rel="${src#${runtime_source}/}"
      dest="$RUNTIME_CONFIG_DIR/$rel"
      mkdir -p "$(dirname "$dest")"

      if [ -f "$dest" ]; then
        if ! cmp -s "$src" "$dest"; then
          echo "Error: runtime-config merge conflict for path '$rel'"
          echo "  existing: $dest"
          echo "  incoming: $src"
          exit 1
        fi
        continue
      fi

      cp -a "$src" "$dest"
    done < <(find "$runtime_source" -type f | sort)
  done

  if [ ! -f "$RUNTIME_CONFIG_DIR/functions.yml" ]; then
    echo "Error: runtime-config/functions.yml not found after materialization."
    exit 1
  fi
  if [ ! -f "$RUNTIME_CONFIG_DIR/routing.yml" ]; then
    echo "Error: runtime-config/routing.yml not found after materialization."
    exit 1
  fi
}

materialize_runtime_compose() {
  local output_path="$1"
  local env_file_path="$2"
  docker compose --env-file "$env_file_path" -f "$COMPOSE_FILE_ABS" --profile deploy config > "$output_path"

  python3 - <<'PY' "$output_path"
import sys
from pathlib import Path

import yaml

path = Path(sys.argv[1])
data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}

services = data.get("services")
if isinstance(services, dict):
    for service in services.values():
        if isinstance(service, dict):
            service.pop("build", None)

path.write_text(yaml.safe_dump(data, sort_keys=False), encoding="utf-8")
PY
}

normalize_bundled_env() {
  local env_path="$1"
  local brand_home="$2"
  python3 - <<'PY' "$env_path" "$brand_home"
import sys
from pathlib import Path

path = Path(sys.argv[1])
brand_home = sys.argv[2]

credential_defaults = {
    "AUTH_USER": "test-admin",
    "AUTH_PASS": "test-secure-password",
    "JWT_SECRET_KEY": "test-secret-key-must-be-at-least-32-chars",
    "X_API_KEY": "test-api-key",
    "RUSTFS_ACCESS_KEY": "rustfsadmin",
    "RUSTFS_SECRET_KEY": "rustfsadmin",
}

port_defaults = {
    "PORT_GATEWAY_HTTPS": "8443",
    "PORT_VICTORIALOGS": "9428",
    "PORT_AGENT_METRICS": "9091",
    "PORT_S3": "9000",
    "PORT_S3_MGMT": "9001",
    "PORT_DATABASE": "8000",
    "PORT_REGISTRY": "5010",
}

forced_values = {
    "CERT_DIR": f"/root/{brand_home}/certs",
    "CONFIG_DIR": "/app/runtime-config",
}

raw_lines = path.read_text(encoding="utf-8").splitlines()
lines = list(raw_lines)
index_by_key: dict[str, int] = {}
value_by_key: dict[str, str] = {}

for idx, line in enumerate(lines):
    stripped = line.strip()
    if not stripped or stripped.startswith("#") or "=" not in line:
        continue
    key, value = line.split("=", 1)
    key = key.strip()
    if key == "":
        continue
    index_by_key[key] = idx
    value_by_key[key] = value.strip()

for key, forced in forced_values.items():
    if key in index_by_key:
        lines[index_by_key[key]] = f"{key}={forced}"
    else:
        lines.append(f"{key}={forced}")

for key, default in port_defaults.items():
    current = value_by_key.get(key, "")
    if key in index_by_key and current not in ("", "0"):
        continue
    if key in index_by_key:
        lines[index_by_key[key]] = f"{key}={default}"
    else:
        lines.append(f"{key}={default}")

for key, default in credential_defaults.items():
    current = value_by_key.get(key, "")
    if key in index_by_key and current != "":
        continue
    if key in index_by_key:
        lines[index_by_key[key]] = f"{key}={default}"
    else:
        lines.append(f"{key}={default}")

path.write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    -a|--artifact-dir)
      if [ -z "${2:-}" ]; then
        echo "Error: --artifact-dir requires a value."
        usage
        exit 1
      fi
      ARTIFACT_DIRS+=("$2")
      shift 2
      ;;
    --artifact-dir=*)
      ARTIFACT_DIRS+=("${1#*=}")
      shift
      ;;
    -e|--env-file)
      if [ -z "${2:-}" ]; then
        echo "Error: --env-file requires a value."
        usage
        exit 1
      fi
      ENV_FILE="$2"
      shift 2
      ;;
    --env-file=*)
      ENV_FILE="${1#*=}"
      shift
      ;;
    -c|--compose-file)
      if [ -z "${2:-}" ]; then
        echo "Error: --compose-file requires a value."
        usage
        exit 1
      fi
      COMPOSE_FILE="$2"
      shift 2
      ;;
    --compose-file=*)
      COMPOSE_FILE="${1#*=}"
      shift
      ;;
    --prepare-images)
      PREPARE_IMAGES=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      POSITIONAL+=("$@")
      break
      ;;
    *)
      POSITIONAL+=("$1")
      shift
      ;;
  esac
done

if [ "${#ARTIFACT_DIRS[@]}" -eq 0 ]; then
  echo "Error: --artifact-dir is required."
  usage
  exit 1
fi
if [ -z "$ENV_FILE" ]; then
  echo "Error: --env-file is required."
  usage
  exit 1
fi

if [ "${#POSITIONAL[@]}" -eq 1 ]; then
  OUTPUT_TAG="${POSITIONAL[0]}"
elif [ "${#POSITIONAL[@]}" -gt 1 ]; then
  echo "Error: too many positional arguments."
  usage
  exit 1
fi

require_command python3
require_command docker

ENV_FILE_ABS="$(to_abs_path "$ENV_FILE")"
if [ ! -f "$ENV_FILE_ABS" ]; then
  echo "Error: env file not found: $ENV_FILE_ABS"
  exit 1
fi

# Ensure we are in the project root.
if [ ! -d "tools/dind-bundler" ] || [ ! -f "tools/dind-bundler/Dockerfile" ]; then
  echo "Error: Please run this script from the project root."
  exit 1
fi

for artifact_dir in "${ARTIFACT_DIRS[@]}"; do
  artifact_dir_abs="$(to_abs_path "$artifact_dir")"
  artifact_manifest_path="${artifact_dir_abs}/artifact.yml"
  if [ ! -f "$artifact_manifest_path" ]; then
    echo "Error: artifact manifest not found: $artifact_manifest_path"
    exit 1
  fi

  while IFS= read -r line; do
    case "$line" in
      PROJECT=*)
        value="${line#PROJECT=}"
        if [ -n "$value" ]; then
          if [ -z "$ARTIFACT_PROJECT" ]; then
            ARTIFACT_PROJECT="$value"
          elif [ "$ARTIFACT_PROJECT" != "$value" ]; then
            echo "Error: artifact project mismatch. expected '$ARTIFACT_PROJECT', got '$value' ($artifact_manifest_path)"
            exit 1
          fi
        fi
        ;;
      MODE=*)
        value="${line#MODE=}"
        if [ -n "$value" ]; then
          if [ -z "$ARTIFACT_MODE" ]; then
            ARTIFACT_MODE="$value"
          elif [ "$ARTIFACT_MODE" != "$value" ]; then
            echo "Error: artifact mode mismatch. expected '$ARTIFACT_MODE', got '$value' ($artifact_manifest_path)"
            exit 1
          fi
        fi
        ;;
      ENV=*)
        value="${line#ENV=}"
        if [ -n "$value" ]; then
          if [ -z "$ARTIFACT_ENV" ]; then
            ARTIFACT_ENV="$value"
          elif [ "$ARTIFACT_ENV" != "$value" ]; then
            echo "Error: artifact env mismatch. expected '$ARTIFACT_ENV', got '$value' ($artifact_manifest_path)"
            exit 1
          fi
        fi
        ;;
      RUNTIME=*)
        runtime_path="${line#RUNTIME=}"
        if [ -n "$runtime_path" ] && [ -z "${SEEN_RUNTIME[$runtime_path]:-}" ]; then
          RUNTIME_CONFIG_SOURCE_DIRS+=("$runtime_path")
          SEEN_RUNTIME["$runtime_path"]=1
        fi
        ;;
      FUNCTION_IMAGE=*)
        add_unique_function_image "${line#FUNCTION_IMAGE=}"
        ;;
      FUNCTION_BUILD=*)
        IFS='|' read -r build_image build_context build_dockerfile <<< "${line#FUNCTION_BUILD=}"
        register_function_build "$build_image" "$build_context" "$build_dockerfile"
        ;;
    esac
  done < <(python3 - <<'PY' "$artifact_manifest_path"
import os
import sys

import yaml

manifest_path = os.path.abspath(sys.argv[1])
manifest_dir = os.path.dirname(manifest_path)

with open(manifest_path, "r", encoding="utf-8") as handle:
    data = yaml.safe_load(handle) or {}

project = str(data.get("project", "")).strip()
mode = str(data.get("mode", "")).strip()
env = str(data.get("env", "")).strip()

if project:
    print(f"PROJECT={project}")
if mode:
    print(f"MODE={mode}")
if env:
    print(f"ENV={env}")

for entry in data.get("artifacts") or []:
    artifact_root = str(entry.get("artifact_root", "")).strip()
    if not artifact_root:
        continue
    artifact_root_abs = artifact_root if os.path.isabs(artifact_root) else os.path.abspath(os.path.join(manifest_dir, artifact_root))

    runtime_dir = str(entry.get("runtime_config_dir", "")).strip()
    if not runtime_dir:
        continue

    runtime_abs = runtime_dir if os.path.isabs(runtime_dir) else os.path.abspath(os.path.join(artifact_root_abs, runtime_dir))
    print(f"RUNTIME={runtime_abs}")

    functions_path = os.path.join(runtime_abs, "functions.yml")
    if not os.path.isfile(functions_path):
        continue

    with open(functions_path, "r", encoding="utf-8") as functions_handle:
        functions_data = yaml.safe_load(functions_handle) or {}

    for name, spec in (functions_data.get("functions") or {}).items():
        if not isinstance(spec, dict):
            continue
        image_name = str(spec.get("image", "")).strip()
        if not image_name:
            continue
        print(f"FUNCTION_IMAGE={image_name}")
        dockerfile_rel = f"functions/{name}/Dockerfile"
        print(f"FUNCTION_BUILD={image_name}|{artifact_root_abs}|{dockerfile_rel}")
PY
)
done

if [ -z "$ARTIFACT_PROJECT" ]; then
  echo "Error: artifact project not found in manifest metadata."
  exit 1
fi
if [ -z "$ARTIFACT_MODE" ]; then
  echo "Error: artifact mode not found in manifest metadata."
  exit 1
fi
if [ "${#RUNTIME_CONFIG_SOURCE_DIRS[@]}" -eq 0 ]; then
  echo "Error: runtime config directories not found from artifact manifest."
  exit 1
fi

resolve_compose_file

BRAND_HOME=".${ARTIFACT_PROJECT}"
OUTPUT_TAG="${OUTPUT_TAG:-${ARTIFACT_PROJECT}-dind-bundle:latest}"
ENV_NAME="${ESB_ENV:-}"
if [ -z "$ENV_NAME" ]; then
  ENV_NAME="$(read_env_file_value "$ENV_FILE_ABS" "ENV" || true)"
fi
ENV_NAME="${ENV_NAME:-${ARTIFACT_ENV:-default}}"

CERT_DIR="${CERT_DIR:-$(pwd)/${BRAND_HOME}/certs}"
RUN_UID="${RUN_UID:-$(read_env_file_value "$ENV_FILE_ABS" "RUN_UID" || true)}"
RUN_GID="${RUN_GID:-$(read_env_file_value "$ENV_FILE_ABS" "RUN_GID" || true)}"
RUN_UID="${RUN_UID:-1000}"
RUN_GID="${RUN_GID:-1000}"

echo "Building DinD bundle from artifact dirs: ${ARTIFACT_DIRS[*]}"
echo "Output tag: $OUTPUT_TAG"
echo "Project: $ARTIFACT_PROJECT"
echo "Mode: $ARTIFACT_MODE"
echo "Env: $ENV_NAME"
echo "Env file: $ENV_FILE_ABS"
echo "Compose file: $COMPOSE_FILE_ABS"
echo "Prepare images: $PREPARE_IMAGES"

while IFS= read -r image; do
  if [ -n "$image" ]; then
    COMPOSE_IMAGES+=("$image")
  fi
done < <(docker compose --env-file "$ENV_FILE_ABS" -f "$COMPOSE_FILE_ABS" --profile deploy config --images | awk 'NF{print}')

for image in "${COMPOSE_IMAGES[@]}"; do
  add_unique_image "$image"
done
for image in "${FUNCTION_IMAGES[@]}"; do
  add_unique_image "$image"
done
add_unique_image "$LOCAL_REGISTRY_IMAGE"

if [ "${#ALL_IMAGES[@]}" -eq 0 ]; then
  echo "Error: no images resolved from compose/artifact."
  exit 1
fi

if [ "${DIND_BUNDLER_DRYRUN:-}" = "true" ]; then
  join_by() { local IFS="$1"; shift; echo "$*"; }
  echo "ARTIFACT_DIRS=$(join_by "," "${ARTIFACT_DIRS[@]}")"
  echo "ENV_FILE=$ENV_FILE_ABS"
  echo "COMPOSE_FILE=$COMPOSE_FILE_ABS"
  echo "OUTPUT_TAG=$OUTPUT_TAG"
  echo "PROJECT=$ARTIFACT_PROJECT"
  echo "MODE=$ARTIFACT_MODE"
  echo "ENV_NAME=$ENV_NAME"
  echo "RUNTIME_CONFIG_SOURCE_DIRS=$(join_by "," "${RUNTIME_CONFIG_SOURCE_DIRS[@]}")"
  echo "FUNCTION_IMAGES=$(join_by "," "${FUNCTION_IMAGES[@]}")"
  echo "COMPOSE_IMAGES=$(join_by "," "${COMPOSE_IMAGES[@]}")"
  echo "ALL_IMAGES=$(join_by "," "${ALL_IMAGES[@]}")"
  echo "PREPARE_IMAGES=$PREPARE_IMAGES"
  echo "LOCAL_REGISTRY_IMAGE=$LOCAL_REGISTRY_IMAGE"
  echo "CERT_DIR=$CERT_DIR"
  echo "RUN_UID=$RUN_UID"
  echo "RUN_GID=$RUN_GID"
  exit 0
fi

# Check certs (no fallback).
required_certs=(
  "rootCA.crt"
  "server.crt"
  "server.key"
  "client.crt"
  "client.key"
)
for cert_file in "${required_certs[@]}"; do
  if [ ! -f "$CERT_DIR/$cert_file" ]; then
    echo "Error: required cert not found: $CERT_DIR/$cert_file"
    echo "Hint: run 'mise run setup:certs' or set CERT_DIR."
    exit 1
  fi
done

collect_missing_images
if [ "${#MISSING_IMAGES[@]}" -gt 0 ] && [ "$PREPARE_IMAGES" = "true" ]; then
  prepare_images
  collect_missing_images
fi
if [ "${#MISSING_IMAGES[@]}" -gt 0 ]; then
  echo "Error: missing local images required for bundle:"
  printf '  - %s\n' "${MISSING_IMAGES[@]}"
  echo "Hint: build/pull required images before running dind-bundler (or pass --prepare-images)."
  exit 1
fi

# Cleanup and setup build context.
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# 1. Save Images.
echo "Saving images to tarball..."
docker save -o "$BUILD_DIR/images.tar" "${ALL_IMAGES[@]}"

# 2. Materialize runtime-config.
merge_runtime_configs

# 3. Prepare context files.
echo "Copying runtime files..."
cp tools/dind-bundler/Dockerfile "$BUILD_DIR/"
cp tools/dind-bundler/entrypoint.sh "$BUILD_DIR/"
cp "$ENV_FILE_ABS" "$BUILD_DIR/.env"

# 4. Normalize bundled env first, then materialize compose from the normalized values.
normalize_bundled_env "$BUILD_DIR/.env" "$BRAND_HOME"

# 5. Generate runtime compose file.
echo "Generating bundle compose file..."
materialize_runtime_compose "$BUILD_DIR/docker-compose.bundle.yml" "$BUILD_DIR/.env"

# 6. Copy certs.
echo "Copying certificates..."
mkdir -p "$BUILD_DIR/certs"
cp "$CERT_DIR"/* "$BUILD_DIR/certs/"

# 7. Build DinD image.
echo "Building DinD image..."
docker build -t "$OUTPUT_TAG" \
  --build-arg BRAND_HOME="$BRAND_HOME" \
  --build-arg CERT_UID="$RUN_UID" \
  --build-arg CERT_GID="$RUN_GID" \
  "$BUILD_DIR"

echo "Done! Image $OUTPUT_TAG created."
