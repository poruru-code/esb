#!/usr/bin/env bash
# Where: e2e/scripts/regenerate_artifacts.sh
# What: Regenerates E2E artifact fixtures using raw artifact producer output.
# Why: Keep E2E fixtures aligned with deploy output. Do not rewrite artifact.yml.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

TEMPLATE_PATH="${REPO_ROOT}/e2e/fixtures/template.e2e.yaml"

if [[ -n "${ARTIFACT_PRODUCER_CMD:-}" ]]; then
  # shellcheck disable=SC2206
  PRODUCER_CMD_ARR=(${ARTIFACT_PRODUCER_CMD})
elif [[ -n "${ESB_CMD:-}" ]]; then
  # shellcheck disable=SC2206
  PRODUCER_CMD_ARR=(${ESB_CMD})
else
  echo "artifact producer command is not configured." >&2
  echo "Set ARTIFACT_PRODUCER_CMD (example: ARTIFACT_PRODUCER_CMD='your-producer-cli')." >&2
  exit 1
fi

if ! command -v "${PRODUCER_CMD_ARR[0]}" >/dev/null 2>&1; then
  echo "artifact producer command not found: ${PRODUCER_CMD_ARR[0]}" >&2
  echo "Set ARTIFACT_PRODUCER_CMD to your producer invocation." >&2
  exit 1
fi

validate_manifest_contract() {
  local manifest_path="$1"
  local env_name="$2"

  python3 - "$manifest_path" "$env_name" <<'PY'
import re
import sys
import yaml

manifest_path, env_name = sys.argv[1], sys.argv[2]

with open(manifest_path, encoding="utf-8") as f:
    manifest = yaml.safe_load(f) or {}

artifacts = manifest.get("artifacts")
if not isinstance(artifacts, list) or not artifacts:
    raise SystemExit(f"{env_name}: artifacts must be a non-empty list ({manifest_path})")

for i, artifact in enumerate(artifacts):
    if not isinstance(artifact, dict):
        raise SystemExit(f"{env_name}: artifacts[{i}] must be an object ({manifest_path})")
    if "id" in artifact:
        raise SystemExit(f"{env_name}: artifacts[{i}].id must not exist ({manifest_path})")

    source_template = artifact.get("source_template")
    if not isinstance(source_template, dict):
        raise SystemExit(f"{env_name}: artifacts[{i}].source_template must exist ({manifest_path})")

    path = source_template.get("path")
    if not isinstance(path, str) or not path.strip():
        raise SystemExit(
            f"{env_name}: artifacts[{i}].source_template.path must be non-empty ({manifest_path})"
        )

    sha = source_template.get("sha256")
    if not isinstance(sha, str) or not re.fullmatch(r"[0-9a-f]{64}", sha):
        raise SystemExit(
            f"{env_name}: artifacts[{i}].source_template.sha256 must be 64 lowercase hex ({manifest_path})"
        )
PY
}

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
  local template_root="${output_dir}/${env_name}"

  rm -rf "${output_dir}"
  mkdir -p "${artifact_dir}"

  ENV_PREFIX=ESB ESB_TAG="${tag}" "${PRODUCER_CMD_ARR[@]}" artifact generate \
    --template "${TEMPLATE_PATH}" \
    --env "${env_name}" \
    --mode "${mode}" \
    --project "${project}" \
    --output "${output_dir}" \
    --manifest "${manifest_path}" \
    --image-uri "lambda-image=${image_uri}" \
    --image-runtime "lambda-image=${image_runtime}" \
    --force \
    --no-save-defaults

  local actual_mode
  actual_mode="$(awk '/^mode:/{print $2; exit}' "${manifest_path}" | tr -d '"')"
  if [[ "${actual_mode}" != "${mode}" ]]; then
    echo "artifact mode mismatch for ${env_name}: expected=${mode} actual=${actual_mode}" >&2
    exit 1
  fi

  validate_manifest_contract "${manifest_path}" "${env_name}"

  # runtime-base is out of deploy artifact contract scope.
  rm -rf "${template_root}/runtime-base"
}

cd "${REPO_ROOT}"

generate_fixture \
  "e2e-docker" \
  "docker" \
  "esb" \
  "127.0.0.1:5010/esb-e2e-lambda-python:latest" \
  "python" \
  "e2e-docker-latest"

generate_fixture \
  "e2e-containerd" \
  "containerd" \
  "esb" \
  "127.0.0.1:5010/esb-e2e-lambda-java:latest" \
  "java21" \
  "e2e-containerd-latest"
