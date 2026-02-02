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

require_env "IMAGE_RUNTIME"

case "$IMAGE_RUNTIME" in
  shared|docker|containerd)
    ;;
  *)
    echo "ERROR: IMAGE_RUNTIME must be shared, docker or containerd (got ${IMAGE_RUNTIME})" >&2
    exit 1
    ;;
esac

# Provisioner (shared) can run on any AGENT_RUNTIME, so no mismatch check is needed.

exec "$@"
