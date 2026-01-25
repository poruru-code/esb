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

require_env "COMPONENT"
require_env "IMAGE_RUNTIME"

if [ "$COMPONENT" != "provisioner" ]; then
  echo "ERROR: COMPONENT must be provisioner (got ${COMPONENT})" >&2
  exit 1
fi

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

exec "$@"
