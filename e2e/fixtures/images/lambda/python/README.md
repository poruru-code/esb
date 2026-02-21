# esb-e2e-lambda-python

AWS Lambda container image (Python 3.12) for ESB E2E `PackageType: Image` tests.
Java counterpart: `e2e/fixtures/images/lambda/java`.

## Repository usage

The default image URI in `e2e/fixtures/template.e2e.yaml` is:

- `127.0.0.1:5010/esb-e2e-lambda-python:latest`

During E2E deploy, runner prepares this fixture image first
by scanning artifact Dockerfiles (`FROM ...`) and then running
`docker buildx build` + `docker push`, followed by
`artifactctl deploy` / `artifactctl provision`.

Use this directory when you need to rebuild and publish the source image.

## Local build and publish (optional)

```bash
docker buildx build \
  --platform linux/amd64 \
  --load \
  --tag 127.0.0.1:5010/esb-e2e-lambda-python:latest \
  ./e2e/fixtures/images/lambda/python
docker push 127.0.0.1:5010/esb-e2e-lambda-python:latest
```

## Local smoke run (optional)

```bash
docker run --rm -p 9000:8080 127.0.0.1:5010/esb-e2e-lambda-python:latest
curl -sS -XPOST localhost:9000/2015-03-31/functions/function/invocations \
  -d '{"message":"hello-image"}'
```
