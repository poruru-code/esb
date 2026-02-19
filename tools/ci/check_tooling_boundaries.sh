#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

echo "[check] validating artifactcore dependency contract"
if rg -n --glob 'go.mod' '^\s*replace\s+github\.com/(poruru|poruru-code)/edge-serverless-box/pkg/artifactcore\b' \
  cli/go.mod tools/artifactctl/go.mod; then
  echo "[error] do not add artifactcore replace directives to cli/tools go.mod; use go.work only" >&2
  exit 1
fi

echo "[check] validating runtime/tooling dependency direction"
if rg -n --glob '**/*.go' \
  '"github\.com/(poruru|poruru-code)/edge-serverless-box/(tools/|pkg/artifactcore)' \
  services; then
  echo "[error] services must not import tools/* or pkg/artifactcore" >&2
  exit 1
fi

echo "[check] boundary checks passed"
