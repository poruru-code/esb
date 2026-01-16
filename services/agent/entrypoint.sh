#!/bin/sh
# Where: /app/entrypoint.sh
# What: Entrypoint for the agent to handle CA trust and start the binary.
set -e

exec /app/agent "$@"
