# esb-e2e-lambda-java

AWS Lambda container image (Java 21) for ESB E2E `PackageType: Image` tests.

## Usage in E2E

This fixture is used through matrix override:

- `image_uri_overrides.lambda-image=127.0.0.1:5010/esb-e2e-lambda-java:latest`
- `image_runtime_overrides.lambda-image=java21`

The image builds `app.jar` from source during Docker build, and deploy
automatically builds/pushes this image to the local registry.

## Local build (optional)

```bash
docker buildx build --platform linux/amd64 --load \
  --tag 127.0.0.1:5010/esb-e2e-lambda-java:latest \
  ./e2e/fixtures/images/lambda/java
docker push 127.0.0.1:5010/esb-e2e-lambda-java:latest
```
