# e2e-minimal-lambda

Minimal AWS Lambda container image (Python 3.12) for ESB E2E `PackageType: Image` tests.

## Repository usage

This repository consumes the published image from:

- `public.ecr.aws/r9p4t4p0/poruru-code:latest`

E2E image tests reference that URI directly via `e2e/fixtures/template.image.yaml`.

## Local build (optional)

```bash
docker buildx build \
  --platform linux/amd64 \
  --load \
  --tag e2e-minimal-lambda:latest \
  ./tools/e2e-minimal-lambda
```

## Local smoke run (optional)

```bash
docker run --rm -p 9000:8080 e2e-minimal-lambda:latest
curl -sS -XPOST localhost:9000/2015-03-31/functions/function/invocations \
  -d '{"message":"hello-image"}'
```
