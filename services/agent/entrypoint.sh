#!/bin/sh
# Where: /app/entrypoint.sh
# What: Entrypoint for ESB Agent to handle CA trust and start the binary.
set -e

# Trust Root CA using shared utility
if [ -f /usr/local/bin/ensure_ca_trust.sh ]; then
    . /usr/local/bin/ensure_ca_trust.sh
    ensure_ca_trust
fi

exec /app/agent "$@"
