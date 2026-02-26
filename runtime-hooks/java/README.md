# Java Runtime Hooks

This directory contains runtime-required Java hook assets and their Maven build project.

- `wrapper/`: handler wrapper jar used as the function entrypoint.
- `agent/`: Java agent jar used for AWS SDK behavior patching.
- `build/`: Maven aggregator project for building wrapper/agent.

Build with Docker (the deploy path uses the same pinned image and command):

```bash
cd <repo-root>
docker run --rm \
  -v "$(pwd)/runtime-hooks/java:/src/runtime-hooks/java:ro" \
  -v "$(pwd)/runtime-hooks/java:/out-hooks" \
  -v "/path/to/generated-settings.xml:/tmp/m2/settings.xml:ro" \
  -v "/path/to/repo/.esb/cache/m2/repository:/tmp/m2/repository" \
  -e MAVEN_CONFIG=/tmp/m2 -e HOME=/tmp \
  -e HTTP_PROXY= -e http_proxy= -e HTTPS_PROXY= -e https_proxy= -e NO_PROXY= -e no_proxy= \
  public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7 \
  bash -lc 'set -euo pipefail; \
    mkdir -p /tmp/work/runtime-hooks/java /out-hooks/wrapper /out-hooks/agent; \
    cp -a /src/runtime-hooks/java/. /tmp/work/runtime-hooks/java/; \
    cd /tmp/work/runtime-hooks/java/build; \
    mvn -s /tmp/m2/settings.xml -q -Dmaven.repo.local=/tmp/m2/repository -Dmaven.artifact.threads=1 -DskipTests \
      -pl ../wrapper,../agent -am package; \
    cp ../wrapper/target/lambda-java-wrapper.jar /out-hooks/wrapper/lambda-java-wrapper.jar; \
    cp ../agent/target/lambda-java-agent.jar /out-hooks/agent/lambda-java-agent.jar'
```

Build outputs:
- `wrapper/lambda-java-wrapper.jar`
- `agent/lambda-java-agent.jar`

Notes:
- The Java build image is digest-pinned for reproducibility.
- Maven always runs with `-s /tmp/m2/settings.xml`.
- Maven proxy source of truth is generated `settings.xml` (not runtime proxy env).
- Maven local repository cache path is `./.esb/cache/m2/repository`.
- Runtime hook JARs are configured for deterministic packaging via
  `project.build.outputTimestamp` in `build/pom.xml`.

Static contract check:

```bash
bash .github/checks/check_java_reproducible_jars.sh
```
