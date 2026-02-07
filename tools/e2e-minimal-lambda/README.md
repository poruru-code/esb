# e2e-minimal-lambda

Minimal AWS Lambda container image (Python 3.12) for ESB E2E `PackageType: Image` tests.

## Prerequisites

- Docker
- AWS CLI v2 (recommended via mise):
  - `mise install aws-cli`
- Authenticated AWS session for ECR Public login

## Build and push

```bash
ECR_PUBLIC_REPO_URI=public.ecr.aws/r9p4t4p0/poruru-code \
IMAGE_TAG=latest \
AWS_REGION=us-east-1 \
./tools/e2e-minimal-lambda/build_push.sh
```

Notes:

- ECR Public authentication API is only in `us-east-1`.
- To build only without pushing, set `NO_PUSH=1`.

## Local smoke run (optional)

```bash
docker run --rm -p 9000:8080 e2e-minimal-lambda:latest
curl -sS -XPOST localhost:9000/2015-03-31/functions/function/invocations \
  -d '{"message":"hello-image"}'
```
