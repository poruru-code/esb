# esb-e2e-lambda-java

AWS Lambda container image (Java 21) for ESB E2E `PackageType: Image` tests.

## Usage in E2E

This fixture is used through matrix override:

- `image_uri_overrides.lambda-image=127.0.0.1:5010/esb-e2e-lambda-java:latest`
- `image_runtime_overrides.lambda-image=java21`

`app.jar` is committed in this repository, and deploy automatically builds/pushes
this image to the local registry.

## Local build (optional)

```bash
mvn -q -DskipTests package
cp target/app.jar app.jar
docker buildx build --platform linux/amd64 --load \
  --tag 127.0.0.1:5010/esb-e2e-lambda-java:latest \
  ./tools/e2e-lambda-fixtures/java
docker push 127.0.0.1:5010/esb-e2e-lambda-java:latest
```
