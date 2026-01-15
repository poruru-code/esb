#!/bin/sh
# Where: services/gateway/entrypoint.sh
# What: Bring up WireGuard and reconcile routes for AllowedIPs on startup.
# Why: Keep multi-node routing stable even when wg-quick misses or conflicts.
set -eu

WG_CONF_PATH="${WG_CONF_PATH:-/app/config/wireguard/wg0.conf}"
WG_INTERFACE="${WG_INTERFACE:-wg0}"
WORKER_ROUTE_VIA_HOST="${GATEWAY_WORKER_ROUTE_VIA_HOST:-}"
WORKER_ROUTE_VIA="${GATEWAY_WORKER_ROUTE_VIA:-}"
WORKER_ROUTE_CIDR="${GATEWAY_WORKER_ROUTE_CIDR:-10.88.0.0/16}"
HAPROXY_CFG="${HAPROXY_CFG:-/app/config/haproxy.gateway.cfg}"

apply_wg_routes() {
  if ! command -v python >/dev/null 2>&1; then
    echo "WARN: python not found; skipping WireGuard route correction"
    return
  fi
  python -m services.gateway.core.wg_routes --interface "$WG_INTERFACE" --conf "$WG_CONF_PATH" \
    || echo "WARN: WireGuard route correction failed"
}

apply_worker_routes_override() {
  if ! command -v python >/dev/null 2>&1; then
    echo "WARN: python not found; skipping worker route override"
    return
  fi
  if [ -z "$WORKER_ROUTE_VIA" ] && [ -n "$WORKER_ROUTE_VIA_HOST" ]; then
    WORKER_ROUTE_VIA="$(getent hosts "$WORKER_ROUTE_VIA_HOST" 2>/dev/null | awk '{print $1}' | head -n1)"
  fi
  if [ -z "$WORKER_ROUTE_VIA" ]; then
    return
  fi
  python -m services.gateway.core.wg_routes \
    --interface "$WG_INTERFACE" \
    --conf "$WG_CONF_PATH" \
    --via "$WORKER_ROUTE_VIA" \
    --include "$WORKER_ROUTE_CIDR" \
    || echo "WARN: worker route override failed"
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

if [ -f "$WG_CONF_PATH" ] && [ -c /dev/net/tun ]; then
  if command -v wireguard-go >/dev/null 2>&1; then
    export WG_QUICK_USERSPACE_IMPLEMENTATION=wireguard-go
    export WG_QUICK_USERSPACE_IMPLEMENTATION_FORCE=1
  fi
  if ! ip link show "$WG_INTERFACE" >/dev/null 2>&1; then
    if command -v wg-quick >/dev/null 2>&1; then
      wg-quick up "$WG_CONF_PATH" || echo "WARN: wg-quick failed; starting Gateway without tunnel"
    else
      echo "WARN: wg-quick not found; starting Gateway without tunnel"
    fi
  fi
  if ip link show "$WG_INTERFACE" >/dev/null 2>&1; then
    apply_wg_routes
  else
    echo "WARN: WireGuard interface missing; skipping route correction"
  fi
else
  echo "INFO: WireGuard config or /dev/net/tun missing; skipping tunnel setup"
fi

apply_worker_routes_override
start_registry_proxy

exec "$@"
