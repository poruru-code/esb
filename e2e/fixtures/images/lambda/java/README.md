# esb-e2e-image-java

AWS Lambda container image (Java 21) for ESB E2E `PackageType: Image` tests.

## Usage in E2E

This fixture is referenced from generated artifact Dockerfiles using:

- `FROM 127.0.0.1:5010/esb-e2e-image-java:latest`

For artifact fixture generation, `e2e/scripts/regenerate_artifacts.sh` sets
`--image-runtime "lambda-image=java21"` for the containerd artifact.

The image builds `app.jar` from source during Docker build, and deploy
automatically builds/pushes this image to the local registry.
When proxy environment variables are present, E2E deploy swaps this fixture's
`MAVEN_IMAGE` build arg to a `tools/maven-shim` image that is pushed to the
local registry first. This keeps `buildx` stages deterministic and ensures
`mvn` always runs with proxy-aware `settings.xml` generated at runtime.
Default Maven base is `public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7`.

## Local build (optional)

```bash
docker buildx build --platform linux/amd64 --load \
  --tag 127.0.0.1:5010/esb-e2e-image-java:latest \
  ./e2e/fixtures/images/lambda/java
docker push 127.0.0.1:5010/esb-e2e-image-java:latest
```
