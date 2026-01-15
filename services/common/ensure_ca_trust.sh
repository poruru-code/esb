#!/bin/sh
# Where: /usr/local/bin/ensure_ca_trust.sh
# What: Shared utility to discover and trust Root CA from mounted directory.
# Why: Avoid duplication across different services (agent, gateway, runtime-node).

ensure_ca_trust() {
  target_ca_path="/usr/local/share/ca-certificates/esb-rootCA.crt"
  mounted_certs_dir="${SSL_CERT_DIR:-/app/config/ssl}"
  
  # Try to discover Root CA from mounted directory if target doesn't exist
  if [ ! -f "$target_ca_path" ] && [ -d "$mounted_certs_dir" ]; then
    mkdir -p /usr/local/share/ca-certificates
    if [ -f "$mounted_certs_dir/rootCA.crt" ]; then
      cp "$mounted_certs_dir/rootCA.crt" "$target_ca_path"
    elif [ -f "$mounted_certs_dir/rootCA.pem" ]; then
      cp "$mounted_certs_dir/rootCA.pem" "$target_ca_path"
    fi
  fi

  if [ -f "$target_ca_path" ]; then
    if command -v update-ca-certificates >/dev/null 2>&1; then
      # Redirect to stderr to avoid polluting stdout of command execution
      if ! update-ca-certificates >/dev/null 2>&1; then
        echo "ERROR: failed to update CA certificates. Root CA may be invalid." >&2
        exit 1
      fi
    fi
  fi
}
