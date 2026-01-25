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

require_env "COMPONENT"
require_env "IMAGE_RUNTIME"
require_env "AGENT_RUNTIME"

if [ "$COMPONENT" != "agent" ]; then
  echo "ERROR: COMPONENT must be agent (got ${COMPONENT})" >&2
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

if [ "$AGENT_RUNTIME" != "$IMAGE_RUNTIME" ]; then
  echo "ERROR: AGENT_RUNTIME=${AGENT_RUNTIME} does not match IMAGE_RUNTIME=${IMAGE_RUNTIME}" >&2
  exit 1
fi

exec /app/agent "$@"
