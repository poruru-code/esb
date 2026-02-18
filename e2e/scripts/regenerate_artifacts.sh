#!/usr/bin/env bash
# Where: e2e/scripts/regenerate_artifacts.sh
# What: Regenerates E2E artifact fixtures using raw `esb artifact generate` output.
# Why: Keep E2E fixtures aligned with CLI output without manual post-processing.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

TEMPLATE_PATH="${REPO_ROOT}/e2e/fixtures/template.e2e.yaml"

if [[ -n "${ESB_CMD:-}" ]]; then
  # shellcheck disable=SC2206
  ESB_CMD_ARR=(${ESB_CMD})
else
  ESB_CMD_ARR=(go -C cli run ./cmd/esb)
fi

generate_fixture() {
  local env_name="$1"
  local mode="$2"
  local project="$3"
  local image_uri="$4"
  local image_runtime="$5"
  local tag="$6"

  local artifact_dir="${REPO_ROOT}/e2e/artifacts/${env_name}"
  local output_dir="${artifact_dir}/template.e2e"
  local manifest_path="${artifact_dir}/artifact.yml"

  rm -rf "${output_dir}"
  mkdir -p "${artifact_dir}"

  ENV_PREFIX=ESB ESB_TAG="${tag}" "${ESB_CMD_ARR[@]}" artifact generate \
    --template "${TEMPLATE_PATH}" \
    --env "${env_name}" \
    --mode "${mode}" \
    --project "${project}" \
    --output "${output_dir}" \
    --manifest "${manifest_path}" \
    --image-uri "lambda-image=${image_uri}" \
    --image-runtime "lambda-image=${image_runtime}" \
    --image-prewarm all \
    --force \
    --no-save-defaults

  local actual_mode
  actual_mode="$(awk '/^mode:/{print $2; exit}' "${manifest_path}" | tr -d '"')"
  if [[ "${actual_mode}" != "${mode}" ]]; then
    echo "artifact mode mismatch for ${env_name}: expected=${mode} actual=${actual_mode}" >&2
    exit 1
  fi

  local runtime_base_dockerfile
  runtime_base_dockerfile="${output_dir}/${env_name}/runtime-base/runtime-hooks/python/docker/Dockerfile"
  if [[ ! -f "${runtime_base_dockerfile}" ]]; then
    echo "runtime-base dockerfile not found for ${env_name}: ${runtime_base_dockerfile}" >&2
    exit 1
  fi
}

cd "${REPO_ROOT}"

generate_fixture \
  "e2e-docker" \
  "docker" \
  "esb-e2e-docker" \
  "127.0.0.1:5010/esb-e2e-lambda-python:latest" \
  "python" \
  "e2e-docker-latest"

generate_fixture \
  "e2e-containerd" \
  "containerd" \
  "esb-e2e-containerd" \
  "127.0.0.1:5010/esb-e2e-lambda-java:latest" \
  "java21" \
  "e2e-containerd-latest"
