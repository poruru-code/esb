#!/bin/sh
# Where: /app/entrypoint.sh
# What: Entrypoint for ESB Agent to handle CA trust and start the binary.
set -e

exec /app/agent "$@"
