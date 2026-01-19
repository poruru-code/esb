#!/bin/sh
# Dispatch runtime-node entrypoints by mode.
set -eu

mode="${RUNTIME_MODE:-containerd}"

case "$mode" in
  firecracker|fc)
    exec /entrypoint.firecracker.sh "$@"
    ;;
  containerd|"")
    exec /entrypoint.containerd.sh "$@"
    ;;
  *)
    echo "ERROR: Unknown RUNTIME_MODE=$mode" >&2
    exit 1
    ;;
esac
