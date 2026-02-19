#!/usr/bin/env bash
# Where: tools/ci/check_repo_layout.sh
# What: Static guard for artifact-first repository layout boundaries.
# Why: Prevent legacy path reintroduction that breaks future repo separation.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
failures=0

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

forbid_reference() {
  local pattern="$1"
  pushd "$ROOT_DIR" >/dev/null
  if rg -n --fixed-strings \
    --glob '!docs/repo-layout-contract.md' \
    --glob '!tools/ci/check_repo_layout.sh' \
    --glob '!.agent/**' \
    -- "$pattern" \
    cli services e2e tools docs docker-bake.hcl docker-compose.containerd.yml \
    docker-compose.docker.yml \
    .mise.toml >/dev/null; then
    echo "[layout-check] FORBIDDEN REFERENCE: '$pattern'" >&2
    failures=$((failures + 1))
  fi
  popd >/dev/null
}

forbid_regex_reference() {
  local pattern="$1"
  pushd "$ROOT_DIR" >/dev/null
  if rg -n \
    --glob '!docs/repo-layout-contract.md' \
    --glob '!tools/ci/check_repo_layout.sh' \
    --glob '!.agent/**' \
    -- "$pattern" \
    cli services e2e tools docs docker-bake.hcl docker-compose.containerd.yml \
    docker-compose.docker.yml \
    .mise.toml >/dev/null; then
    echo "[layout-check] FORBIDDEN REFERENCE (regex): '$pattern'" >&2
    failures=$((failures + 1))
  fi
  popd >/dev/null
}

require_path "cli/assets/runtime-templates/java/templates/dockerfile.tmpl"
require_path "cli/assets/runtime-templates/python/templates/dockerfile.tmpl"
require_path "runtime-hooks/java/build/pom.xml"
require_path "runtime-hooks/python/docker/Dockerfile"
require_path "services/contracts/proto/agent.proto"
require_path "tools/bootstrap/playbook.yml"
require_path "services/gateway/config/gateway_log.yaml"
require_path "services/gateway/config/haproxy.gateway.cfg"
require_path "services/runtime-node/config/Corefile"
require_path "tools/container-structure-test/os-base.yaml"
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
