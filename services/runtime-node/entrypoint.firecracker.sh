#!/bin/sh
# Where: services/runtime-node/entrypoint.firecracker.sh
# What: Firecracker runtime-node entrypoint.
# Why: Keep firecracker-specific startup separate from containerd-only mode.
set -eu

. /entrypoint.common.sh

CNI_GW_IP="${CNI_GW_IP:-10.88.0.1}"
CONTAINERD_BIN="${CONTAINERD_BIN:-containerd}"
CONTAINERD_CONFIG="${CONTAINERD_CONFIG:-/etc/firecracker-containerd/config.toml}"

FIRECRACKER_FIFO_READER="${FIRECRACKER_FIFO_READER:-1}"
VHOST_VSOCK_REQUIRED="${VHOST_VSOCK_REQUIRED:-0}"

DEVMAPPER_POOL="${DEVMAPPER_POOL:-}"
DEVMAPPER_DIR="${DEVMAPPER_DIR:-/var/lib/containerd/devmapper2}"
DEVMAPPER_DATA_SIZE="${DEVMAPPER_DATA_SIZE:-10G}"
DEVMAPPER_META_SIZE="${DEVMAPPER_META_SIZE:-2G}"
DEVMAPPER_UDEV="${DEVMAPPER_UDEV:-0}"

WG_CONTROL_NET="${WG_CONTROL_NET:-}"
WG_CONTROL_GW="${WG_CONTROL_GW:-}"
WG_CONTROL_GW_HOST="${WG_CONTROL_GW_HOST:-gateway}"

if [ "$DEVMAPPER_UDEV" != "1" ]; then
  export DM_UDEV_DISABLE=1
  export DM_UDEV_DISABLE_DISK_RULES_FLAG=1
fi

setup_cgroupv2_delegation

ensure_ip_forward
ensure_route_localnet
ensure_hv_network
ensure_wg_route
start_wg_route_watcher
ensure_vhost_vsock

mkdir -p /run/containerd /var/lib/containerd
mkdir -p /var/lib/firecracker-containerd/runtime /var/lib/firecracker-containerd/shim-base

start_udevd
start_firecracker_fifo_reader

apply_cni_nat

ensure_devmapper_ready
start_devmapper_watcher

if [ "$#" -gt 0 ]; then
  exec "$@"
fi

start_containerd
