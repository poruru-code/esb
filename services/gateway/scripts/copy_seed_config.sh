#!/bin/sh
# Where: services/gateway/scripts/copy_seed_config.sh
# What: Initialize runtime config from seed-config.
# Why: Ensure gateway starts with valid config files before deploy.

set -eu

SEED_CONFIG_DIR="/app/seed-config"
RUNTIME_CONFIG_DIR="/app/runtime-config"

# Create runtime config directory if it doesn't exist
mkdir -p "$RUNTIME_CONFIG_DIR"

# Copy seed config files if they don't exist in runtime
for file in functions.yml routing.yml resources.yml; do
    if [ ! -f "$RUNTIME_CONFIG_DIR/$file" ]; then
        cp "$SEED_CONFIG_DIR/$file" "$RUNTIME_CONFIG_DIR/$file"
        echo "Copied seed $file to $RUNTIME_CONFIG_DIR/$file"
    fi
done

echo "Seed configuration initialization complete"
