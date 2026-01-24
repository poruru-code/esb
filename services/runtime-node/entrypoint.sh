#!/bin/sh
# Dispatch runtime-node entrypoints by containerd runtime.
set -eu

require_env() {
  name="$1"
  if [ -z "${!name:-}" ]; then
    echo "ERROR: ${name} is required" >&2
    exit 1
  fi
}

require_env "COMPONENT"
require_env "IMAGE_RUNTIME"

if [ "$COMPONENT" != "runtime-node" ]; then
  echo "ERROR: COMPONENT must be runtime-node (got ${COMPONENT})" >&2
  exit 1
fi

if [ "$IMAGE_RUNTIME" != "containerd" ]; then
  echo "ERROR: IMAGE_RUNTIME must be containerd (got ${IMAGE_RUNTIME})" >&2
  exit 1
fi

if [ -n "${AGENT_RUNTIME:-}" ] && [ "$AGENT_RUNTIME" != "containerd" ]; then
  echo "ERROR: AGENT_RUNTIME=${AGENT_RUNTIME} does not match IMAGE_RUNTIME=${IMAGE_RUNTIME}" >&2
  exit 1
fi

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
