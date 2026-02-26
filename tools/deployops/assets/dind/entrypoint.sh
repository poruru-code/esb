#!/bin/sh
# Where: tools/deployops/assets/dind/entrypoint.sh
# What: DinD container entrypoint.
# Why: Start dockerd, load pre-baked images, and exec compose.

set -e

DOCKERD_LOG="${DOCKERD_LOG:-/var/log/dockerd.log}"
: > "$DOCKERD_LOG"

LOCAL_REGISTRY_IMAGE="${LOCAL_REGISTRY_IMAGE:-registry:2}"
LOCAL_REGISTRY_ADDR="${LOCAL_REGISTRY_ADDR:-}"
LOCAL_REGISTRY_CONTAINER="${LOCAL_REGISTRY_CONTAINER:-}"

BUNDLE_COMPOSE_FILE="${BUNDLE_COMPOSE_FILE:-/app/docker-compose.bundle.yml}"
BUNDLE_ENV_FILE="${BUNDLE_ENV_FILE:-/app/.env}"
AUTO_PROVISION_ON_BOOT="${AUTO_PROVISION_ON_BOOT:-1}"

is_truthy() {
  case "${1:-}" in
    1|true|TRUE|True|yes|YES|on|ON)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

read_env_file_value() {
  env_file="$1"
  key="$2"
  if [ ! -f "$env_file" ]; then
    return 0
  fi
  awk -F '=' -v target="$key" '
    $0 ~ /^[[:space:]]*#/ { next }
    $0 !~ /=/ { next }
    {
      k=$1
      sub(/^[[:space:]]+/, "", k)
      sub(/[[:space:]]+$/, "", k)
      if (k != target) { next }
      v=substr($0, index($0, "=") + 1)
      sub(/^[[:space:]]+/, "", v)
      sub(/[[:space:]]+$/, "", v)
      if ((v ~ /^".*"$/) || (v ~ /^\047.*\047$/)) {
        v=substr(v, 2, length(v)-2)
      }
      print v
      exit
    }
  ' "$env_file"
}

init_registry_defaults() {
  if [ -z "$LOCAL_REGISTRY_ADDR" ]; then
    port_registry="$(read_env_file_value "$BUNDLE_ENV_FILE" "PORT_REGISTRY")"
    if [ -z "$port_registry" ]; then
      port_registry="5010"
    fi
    LOCAL_REGISTRY_ADDR="127.0.0.1:${port_registry}"
  fi

  if [ -z "$LOCAL_REGISTRY_CONTAINER" ]; then
    registry_port="${LOCAL_REGISTRY_ADDR##*:}"
    LOCAL_REGISTRY_CONTAINER="bundle-registry-${registry_port}"
  fi
}

wait_for_local_registry() {
  i=0
  while [ "$i" -lt 30 ]; do
    if wget -q -O- "http://${LOCAL_REGISTRY_ADDR}/v2/" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}

setup_local_registry() {
  if docker ps --format '{{.Names}}' | grep -q "^${LOCAL_REGISTRY_CONTAINER}$"; then
    wait_for_local_registry
    return 0
  fi

  docker rm -f "${LOCAL_REGISTRY_CONTAINER}" >/dev/null 2>&1 || true
  local_registry_port="${LOCAL_REGISTRY_ADDR##*:}"
  if ! docker run -d --name "${LOCAL_REGISTRY_CONTAINER}" -p "${local_registry_port}:5000" "${LOCAL_REGISTRY_IMAGE}" >/dev/null 2>&1; then
    if wait_for_local_registry; then
      return 0
    fi
    echo "Failed to start local registry container ${LOCAL_REGISTRY_CONTAINER}."
    return 1
  fi

  if ! wait_for_local_registry; then
    echo "Local registry ${LOCAL_REGISTRY_ADDR} did not become ready."
    return 1
  fi
  return 0
}

seed_local_registry_images() {
  registry_port="${LOCAL_REGISTRY_ADDR##*:}"
  registry_addr_prefix="${LOCAL_REGISTRY_ADDR}/"
  registry_internal_prefix="registry:${registry_port}/"
  registry_localhost_prefix="localhost:${registry_port}/"
  registry_loopback_prefix="127.0.0.1:${registry_port}/"
  images="$(
    docker images --format '{{.Repository}}:{{.Tag}}' | awk \
      -v a="$registry_addr_prefix" \
      -v b="$registry_internal_prefix" \
      -v c="$registry_localhost_prefix" \
      -v d="$registry_loopback_prefix" \
      '
        index($0, a) == 1 || index($0, b) == 1 || index($0, c) == 1 || index($0, d) == 1 {
          if (!seen[$0]++) {
            print $0
          }
        }
      '
  )"
  if [ -z "$images" ]; then
    echo "No local-registry-tagged images found for ${LOCAL_REGISTRY_ADDR}; skipping registry seed."
    return 0
  fi

  echo "Seeding local registry with bundled images..."
  while IFS= read -r image; do
    [ -n "$image" ] || continue
    normalized_image="$image"
    case "$image" in
      "${registry_internal_prefix}"*)
        normalized_image="${registry_addr_prefix}${image#${registry_internal_prefix}}"
        ;;
      "${registry_localhost_prefix}"*)
        normalized_image="${registry_addr_prefix}${image#${registry_localhost_prefix}}"
        ;;
      "${registry_loopback_prefix}"*)
        normalized_image="${registry_addr_prefix}${image#${registry_loopback_prefix}}"
        ;;
    esac

    if [ "$normalized_image" != "$image" ]; then
      docker tag "$image" "$normalized_image"
    fi

    echo "  push ${normalized_image}"
    docker push "$normalized_image" >/dev/null
  done <<EOF_IMAGES
$images
EOF_IMAGES
}

run_bootstrap_provisioner() {
  if ! is_truthy "$AUTO_PROVISION_ON_BOOT"; then
    echo "Skipping bootstrap provisioner (AUTO_PROVISION_ON_BOOT=${AUTO_PROVISION_ON_BOOT})."
    return 0
  fi

  if [ ! -f "$BUNDLE_COMPOSE_FILE" ]; then
    echo "Skipping bootstrap provisioner: compose file not found (${BUNDLE_COMPOSE_FILE})."
    return 0
  fi

  if [ ! -f "$BUNDLE_ENV_FILE" ]; then
    echo "Skipping bootstrap provisioner: env file not found (${BUNDLE_ENV_FILE})."
    return 0
  fi

  if ! docker compose -f "$BUNDLE_COMPOSE_FILE" --env-file "$BUNDLE_ENV_FILE" --profile deploy \
    config --services 2>/dev/null | grep -qx "provisioner"; then
    echo "Skipping bootstrap provisioner: service 'provisioner' is not defined."
    return 0
  fi

  echo "Running bootstrap provisioner..."
  docker compose -f "$BUNDLE_COMPOSE_FILE" --env-file "$BUNDLE_ENV_FILE" --profile deploy up \
    --no-build \
    --abort-on-container-exit \
    --exit-code-from provisioner \
    provisioner
  docker compose -f "$BUNDLE_COMPOSE_FILE" --env-file "$BUNDLE_ENV_FILE" --profile deploy \
    rm -f -s provisioner >/dev/null 2>&1 || true
}

# Start Docker daemon in the background.
echo "Starting Docker daemon..."
dockerd-entrypoint.sh dockerd >>"$DOCKERD_LOG" 2>&1 &
DOCKER_PID=$!

# Wait for Docker socket to appear first (avoids early CLI calls during bootstrap).
echo "Waiting for Docker daemon to be ready..."
START_TS=$(date +%s)
while [ ! -S /var/run/docker.sock ]; do
  if ! kill -0 "$DOCKER_PID" >/dev/null 2>&1; then
    echo "Docker daemon failed to start. Last 200 log lines:"
    tail -n 200 "$DOCKERD_LOG" || true
    exit 1
  fi
  if [ -n "${DOCKERD_START_TIMEOUT:-}" ]; then
    NOW_TS=$(date +%s)
    if [ $((NOW_TS - START_TS)) -gt "${DOCKERD_START_TIMEOUT}" ]; then
      echo "Docker daemon did not become ready within ${DOCKERD_START_TIMEOUT}s."
      echo "Last 200 log lines:"
      tail -n 200 "$DOCKERD_LOG" || true
      exit 1
    fi
  fi
  echo "Waiting for Docker daemon..."
  sleep 1
done

# Validate daemon responsiveness after socket is ready.
if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon did not respond to docker info."
  echo "Last 200 log lines:"
  tail -n 200 "$DOCKERD_LOG" || true
  exit 1
fi
echo "Docker daemon is ready."

# Load images if the tarball exists.
if [ -f /images.tar ]; then
  echo "Loading pre-baked images from /images.tar..."
  docker load -i /images.tar
  echo "Images loaded successfully."

  # Remove the tarball to save runtime space.
  rm -f /images.tar
  echo "Removed /images.tar to save runtime space."
fi

init_registry_defaults
if ! setup_local_registry; then
  echo "Fatal: local registry setup failed."
  exit 1
fi
if ! seed_local_registry_images; then
  echo "Fatal: local registry seed failed."
  exit 1
fi

run_bootstrap_provisioner

# Execute the passed command.
echo "Executing command: $@"
exec "$@"
