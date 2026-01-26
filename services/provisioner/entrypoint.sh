#!/bin/sh
# Where: services/provisioner/entrypoint.sh
# What: Runtime guard for provisioner startup.
# Why: Fail fast on component/runtime mismatches.
set -eu

require_env() {
  name="$1"
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "ERROR: ${name} is required" >&2
    exit 1
  fi
}

print_version_json() {
  if [ -f /app/version.json ]; then
    echo "INFO: version.json"
    cat /app/version.json
  else
    echo "WARN: version.json not found"
  fi
}

require_env "IMAGE_RUNTIME"

case "$IMAGE_RUNTIME" in

  docker|containerd)
    ;;
  *)
    echo "ERROR: IMAGE_RUNTIME must be docker or containerd (got ${IMAGE_RUNTIME})" >&2
    exit 1
    ;;
esac

if [ -n "${AGENT_RUNTIME:-}" ] && [ "$AGENT_RUNTIME" != "$IMAGE_RUNTIME" ]; then
  echo "ERROR: AGENT_RUNTIME=${AGENT_RUNTIME} does not match IMAGE_RUNTIME=${IMAGE_RUNTIME}" >&2
  exit 1
fi

print_version_json

exec "$@"
