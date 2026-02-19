#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

echo "[check] validating artifactcore dependency contract"
if rg -n --glob 'go.mod' '^\s*replace\s+github\.com/(poruru|poruru-code)/edge-serverless-box/pkg/(artifactcore|composeprovision)\b' \
  cli/go.mod tools/artifactctl/go.mod; then
  echo "[error] do not add pkg/* replace directives to cli/tools go.mod; use go.work only" >&2
  exit 1
fi

echo "[check] validating runtime/tooling dependency direction"
if rg -n --glob '**/*.go' \
  '"github\.com/(poruru|poruru-code)/edge-serverless-box/(tools/|pkg/artifactcore|pkg/composeprovision)' \
  services; then
  echo "[error] services must not import tools/* or pkg/artifactcore|pkg/composeprovision" >&2
  exit 1
fi

echo "[check] validating pure-core package restrictions"
if rg -n --glob '**/*.go' --glob '!**/*_test.go' '"os/exec"|exec\.Command\(' \
  pkg/artifactcore pkg/yamlshape; then
  echo "[error] pkg/artifactcore and pkg/yamlshape must not execute external commands" >&2
  exit 1
fi

if rg -n --glob '**/*.go' --glob '!**/*_test.go' 'CONTAINER_REGISTRY|HOST_REGISTRY_ADDR' \
  pkg/artifactcore pkg/yamlshape; then
  echo "[error] pkg/artifactcore and pkg/yamlshape must not depend on runtime registry env vars" >&2
  exit 1
fi

echo "[check] validating artifactcore public API surface"
allowlist_file="tools/ci/artifactcore_exports_allowlist.txt"
if [[ ! -f "${allowlist_file}" ]]; then
  echo "[error] missing allowlist file: ${allowlist_file}" >&2
  exit 1
fi
actual_exports="$(
  rg --no-filename -n --glob '*.go' --glob '!**/*_test.go' \
    '^(type|func|var|const) [A-Z][A-Za-z0-9_]*' pkg/artifactcore \
  | sed -E 's/^[0-9]+:(type|func|var|const) ([A-Z][A-Za-z0-9_]*).*/\2/' \
  | sort -u
)"
expected_exports="$(sort -u "${allowlist_file}")"
if ! diff -u <(printf "%s\n" "${expected_exports}") <(printf "%s\n" "${actual_exports}"); then
  echo "[error] artifactcore public API changed; update allowlist with design rationale" >&2
  exit 1
fi

echo "[check] boundary checks passed"
