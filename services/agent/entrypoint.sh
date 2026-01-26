#!/bin/sh
# Where: /app/entrypoint.sh
# What: Entrypoint for the agent with runtime guard before start.
# Why: Fail fast if the runtime or component does not match the image.
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
require_env "AGENT_RUNTIME"


case "$IMAGE_RUNTIME" in
  docker|containerd)
    ;;
  *)
    echo "ERROR: IMAGE_RUNTIME must be docker or containerd (got ${IMAGE_RUNTIME})" >&2
    exit 1
    ;;
esac

# Require exact match for agent/gateway
if [ -n "${AGENT_RUNTIME:-}" ] && [ "$AGENT_RUNTIME" != "$IMAGE_RUNTIME" ]; then
  echo "ERROR: AGENT_RUNTIME=${AGENT_RUNTIME} does not match IMAGE_RUNTIME=${IMAGE_RUNTIME}" >&2
  exit 1
fi

print_version_json

exec /app/agent "$@"
