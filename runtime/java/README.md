# Java Runtime Hooks

This directory contains the Java runtime artifacts used by the generator.
It includes the Lambda handler wrapper and the Java agent.

- `wrapper/`: Handler wrapper jar used as the function entrypoint.
- `agent/`: Java agent jar that patches AWS SDK v2 at runtime.

Build with Docker (default at deploy time):

```
cd runtime/java
docker run --rm \
  -v "$(pwd):/work" -w /work \
  -v "${HOME}/.m2:/root/.m2" \
  maven:3.9.6-eclipse-temurin-21 \
  mvn -q -DskipTests -pl wrapper,agent -am package
```

The build produces shaded jars:
- `wrapper/target/lambda-java-wrapper.jar`
- `agent/target/lambda-java-agent.jar`
