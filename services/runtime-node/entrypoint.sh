#!/bin/sh
set -eu

CNI_GW_IP="${CNI_GW_IP:-10.88.0.1}"
DNAT_APPLY_OUTPUT="${DNAT_APPLY_OUTPUT:-1}"

DNAT_S3_IP="${DNAT_S3_IP:-}"
DNAT_DB_IP="${DNAT_DB_IP:-}"
DNAT_VL_IP="${DNAT_VL_IP:-}"

DNAT_DB_DPORT="${DNAT_DB_DPORT:-8001}"
DNAT_DB_PORT="${DNAT_DB_PORT:-8000}"

ensure_ip_forward() {
  if [ -w /proc/sys/net/ipv4/ip_forward ]; then
    echo 1 > /proc/sys/net/ipv4/ip_forward
  fi
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
mkdir -p /run/containerd /var/lib/containerd

apply_dnat PREROUTING
if [ "$DNAT_APPLY_OUTPUT" = "1" ]; then
  apply_dnat OUTPUT
  apply_snat
fi

exec containerd --address /run/containerd/containerd.sock
