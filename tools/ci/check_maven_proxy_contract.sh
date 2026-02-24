#!/usr/bin/env bash
# Where: tools/ci/check_maven_proxy_contract.sh
# What: Static guard that blocks uncontracted Maven usage in compose-managed service Dockerfiles.
# Why: Keep docker compose control-plane builds outside Maven proxy drift unless explicitly integrated.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
failures=0

compose_build_dirs=(
  "services/common"
  "services/provisioner"
  "services/runtime-node"
  "services/gateway"
  "services/agent"
)

while IFS= read -r -d '' dockerfile; do
  if grep -E -n -- '\bmvn\b|\./mvnw\b|(^|[[:space:]])maven(:|@)' "$dockerfile" >/dev/null; then
    echo "[maven-proxy-contract] FOUND uncontracted Maven usage in compose-managed Dockerfile: ${dockerfile#$ROOT_DIR/}" >&2
    grep -E -n -- '\bmvn\b|\./mvnw\b|(^|[[:space:]])maven(:|@)' "$dockerfile" >&2 || true
    failures=$((failures + 1))
  fi
done < <(find "${compose_build_dirs[@]/#/$ROOT_DIR/}" -type f -name 'Dockerfile*' -print0)

if (( failures > 0 )); then
  echo "[maven-proxy-contract] FAILED (${failures} issue(s))" >&2
  echo "[maven-proxy-contract] If Maven is intentionally introduced in compose builds, integrate deploy-core maven shim ownership (pkg/deployops/mavenshim) and update this guard." >&2
  exit 1
fi

echo "[maven-proxy-contract] OK"
