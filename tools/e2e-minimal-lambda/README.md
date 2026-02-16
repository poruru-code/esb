# e2e-minimal-lambda

AWS Lambda container image (Python 3.12) for ESB E2E `PackageType: Image` tests.

## Repository usage

The E2E flow uses the image URI declared in `e2e/fixtures/template.image.yaml`:

- `public.ecr.aws/r9p4t4p0/poruru-code:latest`

At deploy time, ESB image prewarm pulls that source image and republishes it to the
internal registry.

Use this directory when you need to rebuild and publish the source image.

## Local build and publish (optional)

```bash
docker buildx build \
  --platform linux/amd64 \
  --load \
  --tag public.ecr.aws/r9p4t4p0/poruru-code:latest \
  ./tools/e2e-minimal-lambda
docker push public.ecr.aws/r9p4t4p0/poruru-code:latest
```

## Local smoke run (optional)

```bash
docker run --rm -p 9000:8080 public.ecr.aws/r9p4t4p0/poruru-code:latest
curl -sS -XPOST localhost:9000/2015-03-31/functions/function/invocations \
  -d '{"message":"hello-image"}'
```
