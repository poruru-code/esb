#!/usr/bin/env bash
# Where: tools/e2e_proxy/collect_failure_logs.sh
# What: Collects minimal, high-signal logs for proxy-related E2E failures.
# Why: Keep shared diagnostics small while preserving Maven/proxy root-cause evidence.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

DEFAULT_PRIMARY_LOG="${ROOT_DIR}/e2e/.parallel-e2e-containerd.log"
DEFAULT_SECONDARY_LOG="${ROOT_DIR}/e2e/.parallel-e2e-docker.log"
DEFAULT_OUT="${ROOT_DIR}/artifacts/proxy-failure-summary-$(date +%Y%m%d-%H%M%S).log"
DEFAULT_CTL_BIN="artifactctl"

PRIMARY_LOG="${1:-$DEFAULT_PRIMARY_LOG}"
OUT_FILE="${2:-$DEFAULT_OUT}"
CTL_BIN="${CTL_BIN:-$DEFAULT_CTL_BIN}"

mkdir -p "$(dirname "$OUT_FILE")"

redact_proxy() {
  local value="$1"
  if [[ -z "$value" ]]; then
    printf '(unset)'
    return 0
  fi
  printf '%s' "$value" | sed -E 's#(https?://)[^/@[:space:]]+(:[^@[:space:]]*)?@#\1***:***@#g'
}

append_section_header() {
  local title="$1"
  {
    echo
    echo "===== ${title} ====="
  } >>"$OUT_FILE"
}

append_command() {
  local label="$1"
  shift
  append_section_header "$label"
  {
    echo "\$ $*"
    "$@" 2>&1 || true
  } >>"$OUT_FILE"
}

append_context_matches() {
  local file="$1"
  local pattern="$2"
  local max_matches="$3"
  local context_lines="$4"
  local label="$5"

  append_section_header "${label} (${file})"

  if [[ ! -f "$file" ]]; then
    echo "(log file not found)" >>"$OUT_FILE"
    return 0
  fi

  mapfile -t matches < <(rg -n -m "$max_matches" "$pattern" "$file" | cut -d: -f1 || true)
  if [[ "${#matches[@]}" -eq 0 ]]; then
    echo "(no matches)" >>"$OUT_FILE"
    return 0
  fi

  local line
  for line in "${matches[@]}"; do
    local start=$((line - context_lines))
    local end=$((line + context_lines))
    if ((start < 1)); then
      start=1
    fi
    {
      echo "--- line ${line} (±${context_lines}) ---"
      sed -n "${start},${end}p" "$file"
    } >>"$OUT_FILE"
  done
}

escape_regex() {
  printf '%s' "$1" | sed -E 's/[][(){}.^$*+?|\\]/\\&/g'
}

CTL_BIN_REGEX="$(escape_regex "$CTL_BIN")"

{
  echo "# Proxy Failure Summary"
  echo "generated_at=$(date -Iseconds)"
  echo "cwd=${ROOT_DIR}"
  echo "primary_log=${PRIMARY_LOG}"
  echo "secondary_log=${DEFAULT_SECONDARY_LOG}"
  echo "HTTP_PROXY=$(redact_proxy "${HTTP_PROXY:-}")"
  echo "http_proxy=$(redact_proxy "${http_proxy:-}")"
  echo "HTTPS_PROXY=$(redact_proxy "${HTTPS_PROXY:-}")"
  echo "https_proxy=$(redact_proxy "${https_proxy:-}")"
  echo "NO_PROXY=${NO_PROXY:-${no_proxy:-}}"
} >"$OUT_FILE"

append_command "Git" git -C "$ROOT_DIR" rev-parse --abbrev-ref HEAD
append_command "Git Commit" git -C "$ROOT_DIR" rev-parse HEAD
append_command "Ctl maven-shim help (${CTL_BIN})" "$CTL_BIN" internal maven-shim ensure --help
append_command "Docker Buildx ls" docker buildx ls

append_context_matches "$PRIMARY_LOG" "Preparing local image fixture: .*esb-e2e-image-java" 2 6 "fixture prep"
append_context_matches "$PRIMARY_LOG" "${CTL_BIN_REGEX} internal maven-shim ensure|shim_image" 3 8 "maven shim ensure"
append_context_matches "$PRIMARY_LOG" "docker buildx build .*esb-e2e-image-java|MAVEN_IMAGE=" 3 8 "java fixture build command"
append_context_matches "$PRIMARY_LOG" "RUN mvn -q -DskipTests package|Non-resolvable import POM|Network is unreachable|UnknownIssuer|PKIX|invalid peer certificate|Could not transfer artifact" 6 10 "maven execution / failure"
append_context_matches "$PRIMARY_LOG" "ERROR: process \"/bin/sh -c mvn -q -DskipTests package\"|UnresolvableModelException|ProjectBuildingException" 6 12 "maven terminal errors"
append_context_matches "$PRIMARY_LOG" "\\[e2e-containerd\\] ❌ deploy|\\[e2e-containerd\\] 🏁 done" 4 6 "containerd result"

if [[ -f "$DEFAULT_SECONDARY_LOG" ]]; then
  append_context_matches "$DEFAULT_SECONDARY_LOG" "Preparing local image fixture: .*esb-e2e-image-java|${CTL_BIN_REGEX} internal maven-shim ensure|esb-e2e-image-java|\\[e2e-docker\\] ❌|\\[e2e-docker\\] done" 6 6 "docker-side key lines"
fi

append_section_header "Primary Log Tail"
if [[ -f "$PRIMARY_LOG" ]]; then
  tail -n 80 "$PRIMARY_LOG" >>"$OUT_FILE"
else
  echo "(log file not found)" >>"$OUT_FILE"
fi

echo "Wrote ${OUT_FILE}"
