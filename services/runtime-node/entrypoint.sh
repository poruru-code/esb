#!/bin/sh
# Dispatch runtime-node entrypoints by containerd runtime.
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

if [ "$IMAGE_RUNTIME" != "containerd" ]; then

  echo "ERROR: IMAGE_RUNTIME must be containerd (got ${IMAGE_RUNTIME})" >&2
  exit 1
fi

if [ -n "${AGENT_RUNTIME:-}" ] && [ "$AGENT_RUNTIME" != "containerd" ]; then
  echo "ERROR: AGENT_RUNTIME=${AGENT_RUNTIME} does not match IMAGE_RUNTIME=${IMAGE_RUNTIME}" >&2
  exit 1
fi

print_version_json

runtime="${CONTAINERD_RUNTIME:-}"
case "$runtime" in
  ""|containerd)
    exec /entrypoint.containerd.sh "$@"
    ;;
  aws.firecracker)
    exec /entrypoint.firecracker.sh "$@"
    ;;
  *)
    echo "ERROR: CONTAINERD_RUNTIME must be aws.firecracker or empty (got ${runtime})" >&2
    exit 1
    ;;
esac
