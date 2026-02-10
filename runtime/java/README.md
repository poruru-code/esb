# Java Runtime Hooks

This directory contains the Java runtime artifacts used by the generator.
It includes the Lambda handler wrapper and the Java agent under extensions.

- `extensions/wrapper/`: Handler wrapper jar used as the function entrypoint.
- `extensions/agent/`: Java agent jar that patches AWS SDK v2 at runtime.
- `build/`: Maven aggregator project for building wrapper/agent.

Build with Docker (default at deploy time, fixed AWS SAM build image digest).
ESB always generates a temporary Maven `settings.xml` per run and mounts it read-only.
Proxy configuration is rendered into this file from proxy env inputs:

```
cd runtime/java
docker run --rm \
  -v "$(pwd):/src:ro" -v "$(pwd):/out" \
  -v "/path/to/generated-settings.xml:/tmp/m2/settings.xml:ro" \
  -v "/path/to/repo/.esb/cache/m2/repository:/tmp/m2/repository" \
  -e MAVEN_CONFIG=/tmp/m2 -e HOME=/tmp \
  -e HTTP_PROXY= -e http_proxy= -e HTTPS_PROXY= -e https_proxy= -e NO_PROXY= -e no_proxy= \
  public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7 \
  bash -lc 'set -euo pipefail; \
    mkdir -p /tmp/work; \
    cp -a /src/. /tmp/work; \
    cd /tmp/work/build; \
    mvn -s /tmp/m2/settings.xml -q -Dmaven.repo.local=/tmp/m2/repository -Dmaven.artifact.threads=1 -DskipTests \
      -pl ../extensions/wrapper,../extensions/agent -am package; \
    cp ../extensions/wrapper/target/lambda-java-wrapper.jar /out/extensions/wrapper/lambda-java-wrapper.jar; \
    cp ../extensions/agent/target/lambda-java-agent.jar /out/extensions/agent/lambda-java-agent.jar'
```

The build produces shaded jars:
- `extensions/wrapper/lambda-java-wrapper.jar`
- `extensions/agent/lambda-java-agent.jar`

Notes:
- The Java build image is pinned to a fixed digest for reproducibility.
- No runtime override is provided; all Java builds use the same pinned image.
- `~/.m2/settings.xml` is not used as a runtime dependency.
- Maven execution without `-s /tmp/m2/settings.xml` is not supported.
- Maven proxy source of truth is generated `settings.xml` (not container proxy env).
- Maven dependency resolution is serialized with `-Dmaven.artifact.threads=1`.
- Maven local repository cache path is `./.esb/cache/m2/repository`.
- Contract reference: `docs/java-maven-proxy-contract.md`

Reset Maven cache:

```bash
rm -rf .esb/cache/m2/repository
```

Static contract check:

```bash
bash tools/ci/check_java_proxy_contract.sh
```
