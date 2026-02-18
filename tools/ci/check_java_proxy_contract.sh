#!/usr/bin/env bash
# Where: tools/ci/check_java_proxy_contract.sh
# What: Static guard for Java Maven proxy contract invariants.
# Why: Detect plan drift in E2E warmup behavior before runtime.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

failures=0

require_pattern() {
  local pattern="$1"
  local file="$2"
  if ! rg -n --fixed-strings -- "$pattern" "$file" >/dev/null; then
    echo "[contract-check] MISSING: '$pattern' in $file" >&2
    failures=$((failures + 1))
  fi
}

forbid_pattern() {
  local pattern="$1"
  local file="$2"
  if rg -n --fixed-strings -- "$pattern" "$file" >/dev/null; then
    echo "[contract-check] FORBIDDEN: '$pattern' found in $file" >&2
    failures=$((failures + 1))
  fi
}

PY_FILE="$ROOT_DIR/e2e/runner/warmup.py"
PY_TEST_FILE="$ROOT_DIR/e2e/runner/tests/test_warmup_command.py"
CASE_FILE="$ROOT_DIR/runtime-hooks/java/testdata/maven_proxy_cases.json"
CONTRACT_FILE="$ROOT_DIR/docs/java-maven-proxy-contract.md"

forbid_pattern "if [ -f /tmp/m2/settings.xml ]" "$PY_FILE"
forbid_pattern "else mvn" "$PY_FILE"
forbid_pattern "build-java21:latest" "$PY_FILE"
forbid_pattern "HOST_M2_SETTINGS_PATH" "$PY_FILE"
forbid_pattern "hostM2SettingsPath" "$PY_FILE"

require_pattern "-Dmaven.repo.local={M2_REPOSITORY_PATH}" "$PY_FILE"
require_pattern "M2_REPOSITORY_PATH = \"/tmp/m2/repository\"" "$PY_FILE"
require_pattern ":{M2_REPOSITORY_PATH}" "$PY_FILE"
require_pattern "public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7" "$PY_FILE"
require_pattern "NamedTemporaryFile(" "$PY_FILE"
require_pattern "HTTP_PROXY" "$PY_FILE"

require_pattern "maven_proxy_cases.json" "$PY_TEST_FILE"

if [[ ! -s "$CASE_FILE" ]]; then
  echo "[contract-check] MISSING OR EMPTY: $CASE_FILE" >&2
  failures=$((failures + 1))
fi
if [[ ! -s "$CONTRACT_FILE" ]]; then
  echo "[contract-check] MISSING OR EMPTY: $CONTRACT_FILE" >&2
  failures=$((failures + 1))
fi

if (( failures > 0 )); then
  echo "[contract-check] FAILED ($failures issue(s))" >&2
  exit 1
fi

echo "[contract-check] OK"
