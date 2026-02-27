#!/usr/bin/env bash
# Where: .github/checks/check_repo_layout.sh
# What: Static guard for artifact-first repository layout boundaries.
# Why: Prevent legacy path reintroduction that breaks future repo separation.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
failures=0

has_rg() {
  command -v rg >/dev/null 2>&1
}

SEARCH_TARGET_CANDIDATES=(
  services
  e2e
  tools
  .github
  docs
  cli
  docker-bake.hcl
  docker-compose.containerd.yml
  docker-compose.docker.yml
  .mise.toml
)
SEARCH_TARGETS=()
for candidate in "${SEARCH_TARGET_CANDIDATES[@]}"; do
  if [[ -e "$ROOT_DIR/$candidate" ]]; then
    SEARCH_TARGETS+=("$candidate")
  fi
done

require_path() {
  local path="$1"
  if [[ ! -e "$ROOT_DIR/$path" ]]; then
    echo "[layout-check] MISSING: $path" >&2
    failures=$((failures + 1))
  fi
}

forbid_path() {
  local path="$1"
  if [[ -e "$ROOT_DIR/$path" ]]; then
    echo "[layout-check] FORBIDDEN PATH EXISTS: $path" >&2
    failures=$((failures + 1))
  fi
}

require_any_path() {
  local found=0
  local path
  for path in "$@"; do
    if [[ -e "$ROOT_DIR/$path" ]]; then
      found=1
      break
    fi
  done
  if [[ $found -eq 0 ]]; then
    echo "[layout-check] MISSING (all candidates): $*" >&2
    failures=$((failures + 1))
  fi
}

forbid_reference() {
  local pattern="$1"
  pushd "$ROOT_DIR" >/dev/null
  if has_rg; then
    if rg -n --fixed-strings \
      --glob '!docs/repo-layout-contract.md' \
      --glob '!.github/checks/check_repo_layout.sh' \
      --glob '!.agent/**' \
      -- "$pattern" \
      "${SEARCH_TARGETS[@]}" >/dev/null; then
      echo "[layout-check] FORBIDDEN REFERENCE: '$pattern'" >&2
      failures=$((failures + 1))
    fi
  elif find "${SEARCH_TARGETS[@]}" -type f \
      ! -path 'docs/repo-layout-contract.md' \
      ! -path '.github/checks/check_repo_layout.sh' \
      ! -path '.agent/*' \
      -print0 | xargs -0 -r grep -nF -- "$pattern" >/dev/null; then
    echo "[layout-check] FORBIDDEN REFERENCE: '$pattern'" >&2
    failures=$((failures + 1))
  fi
  popd >/dev/null
}

forbid_regex_reference() {
  local pattern="$1"
  pushd "$ROOT_DIR" >/dev/null
  if has_rg; then
    if rg -n \
      --glob '!docs/repo-layout-contract.md' \
      --glob '!.github/checks/check_repo_layout.sh' \
      --glob '!.agent/**' \
      -- "$pattern" \
      "${SEARCH_TARGETS[@]}" >/dev/null; then
      echo "[layout-check] FORBIDDEN REFERENCE (regex): '$pattern'" >&2
      failures=$((failures + 1))
    fi
  elif find "${SEARCH_TARGETS[@]}" -type f \
      ! -path 'docs/repo-layout-contract.md' \
      ! -path '.github/checks/check_repo_layout.sh' \
      ! -path '.agent/*' \
      -print0 | xargs -0 -r grep -nE -- "$pattern" >/dev/null; then
    echo "[layout-check] FORBIDDEN REFERENCE (regex): '$pattern'" >&2
    failures=$((failures + 1))
  fi
  popd >/dev/null
}

require_path "runtime-hooks/java/build/pom.xml"
require_path "runtime-hooks/python/docker/Dockerfile"
require_path "services/contracts/proto/agent.proto"
require_path "tools/bootstrap/playbook.yml"
require_path "services/gateway/config/gateway_log.yaml"
require_path "services/gateway/config/haproxy.gateway.cfg"
require_path "services/runtime-node/config/Corefile"
require_path ".github/cst/os-base.yaml"
require_path "tools/bootstrap/wireguard/examples/gateway/wg0.conf.example"
require_path "tools/bootstrap/wireguard/examples/compute/wg0.conf.example"

forbid_path "runtime"
forbid_path "contracts"
forbid_path "proto"
forbid_path "bootstrap"
forbid_path "config"

forbid_reference "runtime/java/templates/"
forbid_reference "runtime/python/templates/"
forbid_reference "runtime/java/extensions/"
forbid_reference "runtime/python/extensions/"
forbid_reference "runtime/java/testdata/maven_proxy_cases.json"
forbid_reference "runtime/python/docker/Dockerfile"
forbid_reference "./config/Corefile"
forbid_reference "./config/haproxy.gateway.cfg"
forbid_regex_reference '`config/defaults\.env`'
forbid_regex_reference '`config/haproxy\.cfg`'
forbid_regex_reference '`config/Corefile`'
forbid_regex_reference '`config/haproxy\.gateway\.cfg`'
forbid_regex_reference '`config/gateway_log\.yaml`'
forbid_regex_reference '`config/container-structure-test/[^`]+`'
forbid_regex_reference '`config/wireguard/[^`]+`'
forbid_regex_reference '`contracts/proto/agent\.proto`'
forbid_regex_reference '`proto/agent\.proto`'
forbid_regex_reference '`bootstrap/playbook\.yml`'
forbid_regex_reference '`bootstrap/vars\.yml`'

if (( failures > 0 )); then
  echo "[layout-check] FAILED ($failures issue(s))" >&2
  exit 1
fi

echo "[layout-check] OK"
