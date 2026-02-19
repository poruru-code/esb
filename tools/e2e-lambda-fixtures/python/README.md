# esb-e2e-lambda-python

AWS Lambda container image (Python 3.12) for ESB E2E `PackageType: Image` tests.
Java counterpart: `tools/e2e-lambda-fixtures/java`.

## Repository usage

The E2E flow uses the image URI declared in `e2e/fixtures/template.e2e.yaml`:

- `public.ecr.aws/r9p4t4p0/esb-e2e-lambda-python:latest`

At deploy/apply time, `artifactctl prepare-images` builds function images from artifact
Dockerfiles and pushes them to the internal registry.

Use this directory when you need to rebuild and publish the source image.

## Local build and publish (optional)

```bash
docker buildx build \
  --platform linux/amd64 \
  --load \
  --tag public.ecr.aws/r9p4t4p0/esb-e2e-lambda-python:latest \
  ./tools/e2e-lambda-fixtures/python
docker push public.ecr.aws/r9p4t4p0/esb-e2e-lambda-python:latest
```

## Local smoke run (optional)

```bash
docker run --rm -p 9000:8080 public.ecr.aws/r9p4t4p0/esb-e2e-lambda-python:latest
curl -sS -XPOST localhost:9000/2015-03-31/functions/function/invocations \
  -d '{"message":"hello-image"}'
```
