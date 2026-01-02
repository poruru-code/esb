#!/bin/sh
# Where: services/runtime-node/entrypoint.sh
# What: Initialize runtime-node networking, DNAT, and containerd startup.
# Why: Keep firecracker/containerd runtime wiring consistent on compute hosts.
set -eu

CNI_GW_IP="${CNI_GW_IP:-10.88.0.1}"
DNAT_APPLY_OUTPUT="${DNAT_APPLY_OUTPUT:-1}"

DNAT_S3_IP="${DNAT_S3_IP:-}"
DNAT_DB_IP="${DNAT_DB_IP:-}"
DNAT_VL_IP="${DNAT_VL_IP:-}"

DNAT_DB_DPORT="${DNAT_DB_DPORT:-8001}"
DNAT_DB_PORT="${DNAT_DB_PORT:-8000}"

CONTAINERD_BIN="${CONTAINERD_BIN:-containerd}"
CONTAINERD_CONFIG="${CONTAINERD_CONFIG:-}"
FIRECRACKER_FIFO_READER="${FIRECRACKER_FIFO_READER:-1}"
VHOST_VSOCK_REQUIRED="${VHOST_VSOCK_REQUIRED:-0}"

DEVMAPPER_POOL="${DEVMAPPER_POOL:-}"
DEVMAPPER_DIR="${DEVMAPPER_DIR:-/var/lib/containerd/devmapper2}"
DEVMAPPER_DATA_SIZE="${DEVMAPPER_DATA_SIZE:-10G}"
DEVMAPPER_META_SIZE="${DEVMAPPER_META_SIZE:-2G}"
DEVMAPPER_UDEV="${DEVMAPPER_UDEV:-0}"
DEVMAPPER_NODE_READY_RETRIES="${DEVMAPPER_NODE_READY_RETRIES:-10}"
DEVMAPPER_NODE_READY_INTERVAL="${DEVMAPPER_NODE_READY_INTERVAL:-0.2}"
DEVMAPPER_WATCH_INTERVAL="${DEVMAPPER_WATCH_INTERVAL:-0.2}"
DEVMAPPER_UDEV_SETTLE_TIMEOUT="${DEVMAPPER_UDEV_SETTLE_TIMEOUT:-5}"
DEVMAPPER_ASYNC_REMOVE="${DEVMAPPER_ASYNC_REMOVE:-}"
DEVMAPPER_DISCARD_BLOCKS="${DEVMAPPER_DISCARD_BLOCKS:-}"
WG_CONTROL_NET="${WG_CONTROL_NET:-}"
WG_CONTROL_GW="${WG_CONTROL_GW:-}"
WG_CONTROL_GW_HOST="${WG_CONTROL_GW_HOST:-gateway}"

if [ "$DEVMAPPER_UDEV" != "1" ]; then
  export DM_UDEV_DISABLE=1
  export DM_UDEV_DISABLE_DISK_RULES_FLAG=1
fi

ensure_ip_forward() {
  if [ -w /proc/sys/net/ipv4/ip_forward ]; then
    echo 1 > /proc/sys/net/ipv4/ip_forward
  fi
}

ensure_route_localnet() {
  for path in \
    /proc/sys/net/ipv4/conf/all/route_localnet \
    /proc/sys/net/ipv4/conf/default/route_localnet \
    /proc/sys/net/ipv4/conf/lo/route_localnet; do
    if [ -w "$path" ]; then
      echo 1 > "$path"
    fi
  done
}

ensure_hv_network() {
  if ! command -v ethtool >/dev/null 2>&1; then
    return
  fi
  if ip link show eth0 >/dev/null 2>&1; then
    ethtool -K eth0 tx-checksumming off >/dev/null 2>&1 || true
  fi
}

ensure_ca_trust() {
  ca_path="/usr/local/share/ca-certificates/esb-rootCA.crt"
  if [ ! -f "$ca_path" ]; then
    return
  fi
  if ! command -v update-ca-certificates >/dev/null 2>&1; then
    echo "WARN: update-ca-certificates not found; skipping CA install"
    return
  fi
  update-ca-certificates >/dev/null 2>&1 || echo "WARN: failed to update CA certificates"
}

ensure_wg_route() {
  if [ -z "$WG_CONTROL_NET" ]; then
    return
  fi
  gw="$WG_CONTROL_GW"
  if [ -z "$gw" ] && [ -n "$WG_CONTROL_GW_HOST" ]; then
    gw="$(getent hosts "$WG_CONTROL_GW_HOST" 2>/dev/null | awk '{print $1}' | head -n1)"
  fi
  if [ -z "$gw" ]; then
    gw="$(ip route show default 0.0.0.0/0 2>/dev/null | awk '{print $3}' | head -n1)"
  fi
  if [ -z "$gw" ]; then
    echo "WARN: default gateway not found; skipping WG route for $WG_CONTROL_NET"
    return
  fi
  ip route replace "$WG_CONTROL_NET" via "$gw" 2>/dev/null || true
}

start_wg_route_watcher() {
  if [ -z "$WG_CONTROL_NET" ] || [ -z "$WG_CONTROL_GW_HOST" ] || [ -n "$WG_CONTROL_GW" ]; then
    return
  fi
  (
    for _ in $(seq 1 30); do
      gw="$(getent hosts "$WG_CONTROL_GW_HOST" 2>/dev/null | awk '{print $1}' | head -n1)"
      if [ -n "$gw" ]; then
        ip route replace "$WG_CONTROL_NET" via "$gw" 2>/dev/null || true
        exit 0
      fi
      sleep 1
    done
  ) &
}

ensure_vhost_vsock() {
  if [ -e /dev/vhost-vsock ]; then
    return
  fi
  echo "WARN: /dev/vhost-vsock is missing. Load vhost_vsock on the host to enable vsock."
  if [ "$VHOST_VSOCK_REQUIRED" = "1" ]; then
    echo "ERROR: /dev/vhost-vsock is required but missing."
    exit 1
  fi
}

start_udevd() {
  if [ "$DEVMAPPER_UDEV" != "1" ]; then
    return
  fi
  if ! command -v udevadm >/dev/null 2>&1 && [ ! -x /lib/systemd/systemd-udevd ] && ! command -v udevd >/dev/null 2>&1; then
    echo "WARN: udev not available; devmapper may fail"
    return
  fi
  mkdir -p /run/udev
  if command -v udevd >/dev/null 2>&1; then
    udevd --daemon || true
  elif [ -x /lib/systemd/systemd-udevd ]; then
    /lib/systemd/systemd-udevd --daemon || true
  fi
  if command -v udevadm >/dev/null 2>&1; then
    udevadm trigger --action=add >/dev/null 2>&1 || true
    udevadm settle >/dev/null 2>&1 || true
  fi
}

start_firecracker_fifo_reader() {
  if [ "$FIRECRACKER_FIFO_READER" != "1" ]; then
    return
  fi
  fifo_root="/var/lib/firecracker-containerd/shim-base"
  state_dir="/run/firecracker-fifo-readers"
  mkdir -p "$state_dir"

  (
    while true; do
      if [ -d "$fifo_root" ]; then
        find "$fifo_root" -type p -name 'fc-*.fifo' 2>/dev/null | while read -r fifo; do
          key=$(echo "$fifo" | tr '/' '_')
          marker="$state_dir/$key"
          if [ ! -f "$marker" ]; then
            touch "$marker"
            (cat "$fifo" >/dev/null 2>&1 &) || true
          fi
        done
      fi
      sleep 0.2
    done
  ) &
}

add_dnat_rule() {
  chain="$1"
  dport="$2"
  dest="$3"
  if ! iptables -t nat -C "$chain" -d "${CNI_GW_IP}/32" -p tcp --dport "$dport" \
    -j DNAT --to-destination "$dest" 2>/dev/null; then
    iptables -t nat -A "$chain" -d "${CNI_GW_IP}/32" -p tcp --dport "$dport" \
      -j DNAT --to-destination "$dest"
  fi
}

add_snat_rule() {
  dest_ip="$1"
  if [ "$dest_ip" = "127.0.0.1" ]; then
    return
  fi
  if ! iptables -t nat -C POSTROUTING -s "${CNI_GW_IP}/32" -d "${dest_ip}/32" \
    -j MASQUERADE 2>/dev/null; then
    iptables -t nat -A POSTROUTING -s "${CNI_GW_IP}/32" -d "${dest_ip}/32" \
      -j MASQUERADE
  fi
}

apply_dnat() {
  chain="$1"
  if [ -n "$DNAT_S3_IP" ]; then
    add_dnat_rule "$chain" 9000 "${DNAT_S3_IP}:9000"
  fi
  if [ -n "$DNAT_DB_IP" ]; then
    add_dnat_rule "$chain" "$DNAT_DB_DPORT" "${DNAT_DB_IP}:${DNAT_DB_PORT}"
  fi
  if [ -n "$DNAT_VL_IP" ]; then
    add_dnat_rule "$chain" 9428 "${DNAT_VL_IP}:9428"
  fi
}

apply_snat() {
  if [ -n "$DNAT_S3_IP" ]; then
    add_snat_rule "$DNAT_S3_IP"
  fi
  if [ -n "$DNAT_DB_IP" ]; then
    add_snat_rule "$DNAT_DB_IP"
  fi
  if [ -n "$DNAT_VL_IP" ]; then
    add_snat_rule "$DNAT_VL_IP"
  fi
}

ensure_ip_forward
ensure_route_localnet
ensure_hv_network
ensure_ca_trust
ensure_wg_route
start_wg_route_watcher
ensure_vhost_vsock
mkdir -p /run/containerd /var/lib/containerd
mkdir -p /var/lib/firecracker-containerd/runtime /var/lib/firecracker-containerd/shim-base
start_udevd
start_firecracker_fifo_reader

normalize_bool() {
  value="$(echo "${1:-}" | tr '[:upper:]' '[:lower:]' | tr -d ' ')"
  case "$value" in
    1|true|yes|on) echo "true" ;;
    0|false|no|off) echo "false" ;;
    *) return 1 ;;
  esac
}

maybe_patch_devmapper_config() {
  config_path="$CONTAINERD_CONFIG"
  if [ -z "$config_path" ] || [ ! -f "$config_path" ]; then
    return
  fi
  if ! grep -q 'io.containerd.snapshotter.v1.devmapper' "$config_path"; then
    return
  fi

  pool_value=""
  if [ -n "$DEVMAPPER_POOL" ]; then
    pool_value="$DEVMAPPER_POOL"
  fi
  async_value=""
  discard_value=""
  if [ -n "$DEVMAPPER_ASYNC_REMOVE" ]; then
    if async_value="$(normalize_bool "$DEVMAPPER_ASYNC_REMOVE")"; then
      if grep -q '^[[:space:]]*async_remove' "$config_path"; then
        async_value=""
      fi
    else
      echo "WARN: invalid DEVMAPPER_ASYNC_REMOVE=$DEVMAPPER_ASYNC_REMOVE; skipping"
    fi
  fi
  if [ -n "$DEVMAPPER_DISCARD_BLOCKS" ]; then
    if discard_value="$(normalize_bool "$DEVMAPPER_DISCARD_BLOCKS")"; then
      if grep -q '^[[:space:]]*discard_blocks' "$config_path"; then
        discard_value=""
      fi
    else
      echo "WARN: invalid DEVMAPPER_DISCARD_BLOCKS=$DEVMAPPER_DISCARD_BLOCKS; skipping"
    fi
  fi
  if [ -z "$async_value" ] && [ -z "$discard_value" ] && [ -z "$pool_value" ]; then
    return
  fi

  tmp="${config_path}.tmp"
  if ! awk -v pool="$pool_value" -v async="$async_value" -v discard="$discard_value" '
    BEGIN { in_dev = 0; pool_done = 0; async_done = 0; discard_done = 0 }
    $0 ~ /^[[:space:]]*\[plugins\."io\.containerd\.snapshotter\.v1\.devmapper"\]/ {
      in_dev = 1
      pool_done = 0
      async_done = 0
      discard_done = 0
      print
      next
    }
    in_dev && $0 ~ /^[[:space:]]*\[/ {
      if (pool != "" && pool_done == 0) { print "    pool_name = \"" pool "\"" }
      if (async != "" && async_done == 0) { print "    async_remove = " async }
      if (discard != "" && discard_done == 0) { print "    discard_blocks = " discard }
      in_dev = 0
      print
      next
    }
    in_dev && $0 ~ /^[[:space:]]*pool_name[[:space:]]*=/ {
      if (pool != "") {
        print "    pool_name = \"" pool "\""
        pool_done = 1
        next
      }
    }
    in_dev && $0 ~ /^[[:space:]]*async_remove[[:space:]]*=/ {
      if (async != "") {
        print "    async_remove = " async
        async_done = 1
        next
      }
    }
    in_dev && $0 ~ /^[[:space:]]*discard_blocks[[:space:]]*=/ {
      if (discard != "") {
        print "    discard_blocks = " discard
        discard_done = 1
        next
      }
    }
    { print }
    END {
      if (in_dev) {
        if (pool != "" && pool_done == 0) { print "    pool_name = \"" pool "\"" }
        if (async != "" && async_done == 0) { print "    async_remove = " async }
        if (discard != "" && discard_done == 0) { print "    discard_blocks = " discard }
      }
    }
  ' "$config_path" > "$tmp"; then
    echo "WARN: failed to render devmapper config patch"
    rm -f "$tmp"
    return
  fi
  if ! mv "$tmp" "$config_path"; then
    echo "WARN: failed to update devmapper config at $config_path"
    rm -f "$tmp"
  fi
}

maybe_udev_settle() {
  if [ "$DEVMAPPER_UDEV" != "1" ]; then
    return
  fi
  if ! command -v udevadm >/dev/null 2>&1; then
    return
  fi
  udevadm settle --timeout="$DEVMAPPER_UDEV_SETTLE_TIMEOUT" >/dev/null 2>&1 || true
}

wait_for_devmapper_node() {
  node_path="$1"
  tries="$DEVMAPPER_NODE_READY_RETRIES"
  interval="$DEVMAPPER_NODE_READY_INTERVAL"
  while [ "$tries" -gt 0 ] && [ ! -e "$node_path" ]; do
    sleep "$interval"
    tries=$((tries - 1))
  done
  if [ ! -e "$node_path" ]; then
    echo "WARN: devmapper node still missing: $node_path"
  fi
}

ensure_devmapper_ready() {
  if [ -z "$DEVMAPPER_POOL" ]; then
    return
  fi
  if ! command -v dmsetup >/dev/null 2>&1; then
    echo "WARN: dmsetup not found; skipping devmapper check"
    return
  fi

  dm_env="DM_UDEV_DISABLE=1 DM_UDEV_DISABLE_DISK_RULES_FLAG=1"
  if [ "$DEVMAPPER_UDEV" = "1" ]; then
    dm_env=""
  fi

  if ! env $dm_env dmsetup status "$DEVMAPPER_POOL" >/dev/null 2>&1; then
    echo "ERROR: Devmapper pool ${DEVMAPPER_POOL} is missing. Run esb node provision."
    exit 1
  fi

  env $dm_env dmsetup mknodes "$DEVMAPPER_POOL" >/dev/null 2>&1 || true
  maybe_udev_settle
  if [ ! -e "/dev/mapper/$DEVMAPPER_POOL" ]; then
    env $dm_env dmsetup mknodes "$DEVMAPPER_POOL" >/dev/null 2>&1 || true
    maybe_udev_settle
    wait_for_devmapper_node "/dev/mapper/$DEVMAPPER_POOL"
  fi
}

start_devmapper_watcher() {
  if [ -z "$DEVMAPPER_POOL" ]; then
    return
  fi
  if [ "$DEVMAPPER_UDEV" = "1" ]; then
    return
  fi
  if ! command -v dmsetup >/dev/null 2>&1; then
    return
  fi

  (
    while true; do
      for dev in $(dmsetup ls --noheadings 2>/dev/null | awk '{print $1}'); do
        case "$dev" in
          "${DEVMAPPER_POOL}-snap-"*)
            if [ ! -e "/dev/mapper/$dev" ]; then
              dmsetup mknodes "$dev" >/dev/null 2>&1 || true
              wait_for_devmapper_node "/dev/mapper/$dev"
            fi
            ;;
        esac
      done
      sleep "$DEVMAPPER_WATCH_INTERVAL"
    done
  ) &
}

apply_dnat PREROUTING
if [ "$DNAT_APPLY_OUTPUT" = "1" ]; then
  apply_dnat OUTPUT
  apply_snat
fi

ensure_devmapper_ready
start_devmapper_watcher
maybe_patch_devmapper_config

if [ -n "$CONTAINERD_CONFIG" ]; then
  exec "$CONTAINERD_BIN" --config "$CONTAINERD_CONFIG" --address /run/containerd/containerd.sock
fi
exec "$CONTAINERD_BIN" --address /run/containerd/containerd.sock
