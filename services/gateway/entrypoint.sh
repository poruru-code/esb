#!/bin/sh
# Where: services/gateway/entrypoint.sh
# What: Bring up WireGuard and reconcile routes for AllowedIPs on startup.
# Why: Keep multi-node routing stable even when wg-quick misses or conflicts.
set -eu

require_env() {
  name="$1"
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "ERROR: ${name} is required" >&2
    exit 1
  fi
}

require_env "IMAGE_RUNTIME"

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

# Phase 4: Initialize seed configuration for hot reload
# Copy seed config files to runtime-config directory if they don't exist
COPY_SEED_SCRIPT="/app/scripts/copy_seed_config.sh"
if [ -f "$COPY_SEED_SCRIPT" ]; then
  echo "INFO: Initializing seed configuration..."
  sh "$COPY_SEED_SCRIPT"
fi

WG_CONF_PATH="${WG_CONF_PATH:-/app/config/wireguard/wg0.conf}"
WG_INTERFACE="${WG_INTERFACE:-wg0}"
WORKER_ROUTE_VIA_HOST="${GATEWAY_WORKER_ROUTE_VIA_HOST:-}"
WORKER_ROUTE_VIA="${GATEWAY_WORKER_ROUTE_VIA:-}"
WORKER_ROUTE_CIDR="${GATEWAY_WORKER_ROUTE_CIDR:-}"
HAPROXY_CFG="${HAPROXY_CFG:-/app/config/haproxy.gateway.cfg}"

load_cni_identity_file() {
  cni_identity_file="/var/lib/cni/esb-cni.env"
  if [ ! -f "$cni_identity_file" ]; then
    return 1
  fi
  # shellcheck disable=SC1090
  . "$cni_identity_file"
  return 0
}

first_host_from_cidr() {
  cidr="$1"
  network_ip="${cidr%%/*}"
  if [ -z "$network_ip" ]; then
    return 1
  fi
  gateway_ip="$(printf '%s' "$network_ip" | awk -F. 'NF==4 {printf "%s.%s.%s.%d", $1, $2, $3, $4+1}')"
  if [ -z "$gateway_ip" ]; then
    return 1
  fi
  printf '%s\n' "$gateway_ip"
}

resolve_runtime_cni_defaults() {
  resolved_subnet="${CNI_SUBNET:-}"
  resolved_gw="${CNI_GW_IP:-}"
  wait_attempts=30
  wait_interval=0.2

  if [ -z "$resolved_subnet" ] || [ -z "$resolved_gw" ]; then
    while [ "$wait_attempts" -gt 0 ]; do
      load_cni_identity_file || true
      if [ -z "$resolved_subnet" ]; then
        resolved_subnet="${CNI_SUBNET:-}"
      fi
      if [ -z "$resolved_gw" ]; then
        resolved_gw="${CNI_GW_IP:-}"
      fi
      # route default needs subnet; keep waiting until subnet is known.
      if [ -n "$resolved_subnet" ]; then
        break
      fi
      wait_attempts=$((wait_attempts - 1))
      sleep "$wait_interval"
    done
  fi

  if [ -z "$resolved_gw" ] && [ -n "$resolved_subnet" ]; then
    resolved_gw="$(first_host_from_cidr "$resolved_subnet" || true)"
  fi

  if [ -z "${DATA_PLANE_HOST:-}" ] && [ -n "$resolved_gw" ]; then
    DATA_PLANE_HOST="$resolved_gw"
    export DATA_PLANE_HOST
  fi

  if [ -z "${GATEWAY_INTERNAL_URL:-}" ] && [ -n "${DATA_PLANE_HOST:-}" ]; then
    GATEWAY_INTERNAL_URL="https://${DATA_PLANE_HOST}:8443"
    export GATEWAY_INTERNAL_URL
  fi

  if [ -z "${DATA_PLANE_HOST:-}" ]; then
    echo "WARN: DATA_PLANE_HOST unresolved (set DATA_PLANE_HOST or CNI identity inputs)"
  fi
  if [ -z "${GATEWAY_INTERNAL_URL:-}" ]; then
    echo "WARN: GATEWAY_INTERNAL_URL unresolved (set GATEWAY_INTERNAL_URL or DATA_PLANE_HOST)"
  fi

  if [ -z "${GATEWAY_WORKER_ROUTE_CIDR:-}" ] && [ -n "$resolved_subnet" ]; then
    WORKER_ROUTE_CIDR="$resolved_subnet"
  else
    WORKER_ROUTE_CIDR="${GATEWAY_WORKER_ROUTE_CIDR:-}"
  fi
}

apply_wg_routes() {
  if ! command -v python >/dev/null 2>&1; then
    echo "WARN: WG python not found; skipping route correction"
    return
  fi
  python -m services.gateway.core.wg_routes --interface "$WG_INTERFACE" --conf "$WG_CONF_PATH" \
    || echo "WARN: WG route correction failed"
}

apply_worker_routes_override() {
  if ! command -v python >/dev/null 2>&1; then
    echo "WARN: WG python not found; skipping worker route override"
    return
  fi
  if [ -z "$WORKER_ROUTE_VIA" ] && [ -n "$WORKER_ROUTE_VIA_HOST" ]; then
    WORKER_ROUTE_VIA="$(getent hosts "$WORKER_ROUTE_VIA_HOST" 2>/dev/null | awk '{print $1}' | head -n1)"
  fi
  if [ -z "$WORKER_ROUTE_VIA" ]; then
    return
  fi
  if [ -z "$WORKER_ROUTE_CIDR" ]; then
    echo "WARN: Worker route CIDR unresolved; skipping worker route override"
    return
  fi
  python -m services.gateway.core.wg_routes \
    --interface "$WG_INTERFACE" \
    --conf "$WG_CONF_PATH" \
    --via "$WORKER_ROUTE_VIA" \
    --include "$WORKER_ROUTE_CIDR" \
    || echo "WARN: WG worker route override failed"
}

start_registry_proxy() {
  if [ ! -f "$HAPROXY_CFG" ]; then
    return
  fi
  if ! command -v haproxy >/dev/null 2>&1; then
    echo "WARN: haproxy not found; registry proxy disabled"
    return
  fi
  haproxy -f "$HAPROXY_CFG" >/dev/null 2>&1 &
}

resolve_runtime_cni_defaults

if [ -f "$WG_CONF_PATH" ] && [ -c /dev/net/tun ]; then
  if command -v wireguard-go >/dev/null 2>&1; then
    export WG_QUICK_USERSPACE_IMPLEMENTATION=wireguard-go
    export WG_QUICK_USERSPACE_IMPLEMENTATION_FORCE=1
  fi
  if ! ip link show "$WG_INTERFACE" >/dev/null 2>&1; then
    if command -v wg-quick >/dev/null 2>&1; then
      wg-quick up "$WG_CONF_PATH" || echo "WARN: WG wg-quick failed; starting Gateway without tunnel"
    else
      echo "WARN: WG wg-quick not found; starting Gateway without tunnel"
    fi
  fi
  if ip link show "$WG_INTERFACE" >/dev/null 2>&1; then
    apply_wg_routes
  else
    echo "WARN: WG interface missing; skipping route correction"
  fi
else
  echo "INFO: WG config or /dev/net/tun missing; skipping tunnel setup"
fi

apply_worker_routes_override
start_registry_proxy

exec "$@"
