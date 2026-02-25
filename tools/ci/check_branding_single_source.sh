#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v rg >/dev/null 2>&1; then
  echo "[error] rg is required for branding single-source checks." >&2
  exit 1
fi

status=0

check_with_allowlist() {
  local description="$1"
  local allowlist_regex="$2"
  shift 2

  local tmp
  tmp="$(mktemp)"
  "$@" >"${tmp}" 2>/dev/null || true

  if [[ ! -s "${tmp}" ]]; then
    echo "[ok] ${description}"
    rm -f "${tmp}"
    return
  fi

  local unexpected
  unexpected="$(grep -Ev "${allowlist_regex}" "${tmp}" || true)"
  if [[ -n "${unexpected}" ]]; then
    echo "[error] ${description}" >&2
    printf '%s\n' "${unexpected}" >&2
    status=1
  else
    echo "[ok] ${description}"
  fi
  rm -f "${tmp}"
}

check_with_allowlist \
  "Go default brand slug literals are scoped to single sources" \
  '^(services/agent/internal/identity/stack_identity.go|pkg/deployops/branding_constants_gen.go):' \
  rg -n --glob '**/*.go' --glob '!**/*_test.go' '"esb"' services pkg tools

check_with_allowlist \
  "Python default brand slug literals are scoped to the branding contract" \
  '^e2e/runner/branding.py:' \
  rg -n --glob '*.py' 'DEFAULT_BRAND_SLUG\s*=\s*"esb"|BRAND_SLUG\s*=\s*"esb"|SLUG\s*=\s*"esb"' e2e tools

check_with_allowlist \
  "Go slug normalizers are scoped to identity package" \
  '^services/agent/internal/identity/stack_identity.go:' \
  rg -n --glob '**/*.go' --glob '!**/*_test.go' '^func\s+(NormalizeBrandSlug|normalizeBrandSlug)\s*\(' services pkg tools

check_with_allowlist \
  "ctl command default assignments are scoped to contract files" \
  '^(e2e/runner/ctl_contract.py|tools/artifactctl/cmd/artifactctl/main.go|tools/e2e_proxy/collect_failure_logs.sh):' \
  rg -n --glob '*.py' --glob '*.go' --glob '*.sh' --glob '!tools/ci/check_branding_single_source.sh' 'DEFAULT_CTL_BIN\s*=\s*"artifactctl"|DEFAULT_CTL_BIN="artifactctl"|ctlCommandName\s*=\s*"artifactctl"' e2e tools

if [[ ${status} -ne 0 ]]; then
  exit ${status}
fi

echo "[ok] branding single-source checks passed"
