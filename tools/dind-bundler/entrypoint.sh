#!/bin/sh
# Where: tools/dind-bundler/entrypoint.sh
# What: DinD container entrypoint.
# Why: Start dockerd, load pre-baked images, and exec compose.

set -e

DOCKERD_LOG="${DOCKERD_LOG:-/var/log/dockerd.log}"
: > "$DOCKERD_LOG"

# Start Docker daemon in the background
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

# Load images if the tarball exists
if [ -f /images.tar ]; then
  echo "Loading pre-baked images from /images.tar..."
  docker load -i /images.tar
  echo "Images loaded successfully."

  # Remove the tarball to save space (though it's in the layer, it saves runtime space)
  rm -f /images.tar
  echo "Removed /images.tar to save runtime space."
fi

# Execute the passed command
echo "Executing command: $@"
exec "$@"
