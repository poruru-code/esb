#!/usr/bin/env bash
# Where: tools/e2e-minimal-lambda/build_push.sh
# What: Build and push the minimal Lambda image to Amazon ECR Public.
# Why: Keep E2E image publishing reproducible from this repository.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

AWS_REGION="${AWS_REGION:-us-east-1}"
ECR_PUBLIC_REPO_URI="${ECR_PUBLIC_REPO_URI:-public.ecr.aws/r9p4t4p0/poruru-code}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
IMAGE_PLATFORM="${IMAGE_PLATFORM:-linux/amd64}"
LOCAL_IMAGE_NAME="${LOCAL_IMAGE_NAME:-e2e-minimal-lambda}"
NO_PUSH="${NO_PUSH:-0}"

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: docker command not found" >&2
  exit 1
fi

if [[ "${NO_PUSH}" != "1" ]] && ! command -v aws >/dev/null 2>&1; then
  echo "ERROR: aws command not found (install via 'mise install aws-cli')" >&2
  exit 1
fi

if [[ "${AWS_REGION}" != "us-east-1" ]]; then
  echo "ERROR: ECR Public authentication endpoint must use AWS_REGION=us-east-1" >&2
  exit 1
fi

LOCAL_REF="${LOCAL_IMAGE_NAME}:${IMAGE_TAG}"
REMOTE_REF="${ECR_PUBLIC_REPO_URI}:${IMAGE_TAG}"

echo "==> Building image"
echo "    local : ${LOCAL_REF}"
echo "    remote: ${REMOTE_REF}"
docker buildx build \
  --platform "${IMAGE_PLATFORM}" \
  --load \
  --tag "${LOCAL_REF}" \
  "${SCRIPT_DIR}"

if [[ "${NO_PUSH}" == "1" ]]; then
  echo "NO_PUSH=1 set. Build complete without push."
  exit 0
fi

echo "==> Logging in to ECR Public"
aws ecr-public get-login-password --region "${AWS_REGION}" \
  | docker login --username AWS --password-stdin public.ecr.aws

echo "==> Pushing image"
docker tag "${LOCAL_REF}" "${REMOTE_REF}"
docker push "${REMOTE_REF}"

echo "Done: ${REMOTE_REF}"
