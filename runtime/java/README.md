# Java Runtime Hooks

This directory contains the Java runtime artifacts used by the generator.
It includes the Lambda handler wrapper and the Java agent under extensions.

- `extensions/wrapper/`: Handler wrapper jar used as the function entrypoint.
- `extensions/agent/`: Java agent jar that patches AWS SDK v2 at runtime.
- `build/`: Maven aggregator project for building wrapper/agent.

Build with Docker (default at deploy time):

```
cd runtime/java
docker run --rm \
  -v "$(pwd):/src:ro" -v "$(pwd):/out" \
  -v "${HOME}/.m2:/tmp/m2" -e MAVEN_CONFIG=/tmp/m2 -e HOME=/tmp \
  maven:3.9.6-eclipse-temurin-21 \
  bash -lc 'set -euo pipefail; \
    mkdir -p /tmp/work; \
    cp -a /src/. /tmp/work; \
    cd /tmp/work/build; \
    mvn -q -DskipTests -pl ../extensions/wrapper,../extensions/agent -am package; \
    cp ../extensions/wrapper/target/lambda-java-wrapper.jar /out/extensions/wrapper/lambda-java-wrapper.jar; \
    cp ../extensions/agent/target/lambda-java-agent.jar /out/extensions/agent/lambda-java-agent.jar'
```

The build produces shaded jars:
- `extensions/wrapper/lambda-java-wrapper.jar`
- `extensions/agent/lambda-java-agent.jar`
