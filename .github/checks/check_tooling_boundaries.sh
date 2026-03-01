#!/usr/bin/env bash
# Where: .github/checks/check_tooling_boundaries.sh
# What: Guard import/module/public-API boundaries for shared packages.
# Why: Keep architecture contracts enforceable in CI.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

has_rg() {
  command -v rg >/dev/null 2>&1
}

search_files() {
  local pattern="$1"
  shift
  if has_rg; then
    rg -n --glob 'go.mod' "${pattern}" "$@"
  else
    grep -nE "${pattern}" "$@"
  fi
}

search_go_tree() {
  local pattern="$1"
  shift
  if has_rg; then
    rg -n --glob '**/*.go' "${pattern}" "$@"
  else
    grep -RInE --include='*.go' "${pattern}" "$@"
  fi
}

search_go_tree_non_test() {
  local pattern="$1"
  shift
  if has_rg; then
    rg -n --glob '**/*.go' --glob '!**/*_test.go' "${pattern}" "$@"
  else
    find "$@" -type f -name '*.go' ! -name '*_test.go' -print0 | xargs -0 -r grep -nE "${pattern}"
  fi
}

echo "[check] validating runtime/tooling dependency direction"
if search_go_tree \
  '"github\.com/(poruru|poruru-code)/(edge-serverless-box|esb)/tools/' \
  services; then
  echo "[error] services must not import tools/* modules" >&2
  exit 1
fi

echo "[check] boundary checks passed"
