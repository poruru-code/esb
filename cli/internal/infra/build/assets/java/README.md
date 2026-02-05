# Java Wrapper Jar

This folder contains the Java handler wrapper source used by the generator.
The wrapper jar is built on demand during deploy if it does not exist.

Build with Docker (default at deploy time):

```
cd cli/internal/infra/build/assets/java
docker run --rm \
  -v "$(pwd):/work" -w /work \
  -v "${HOME}/.m2:/root/.m2" \
  maven:3.9.6-eclipse-temurin-21 \
  mvn -q -DskipTests package
```

The build produces a shaded jar that includes Jackson and the Lambda runtime
interfaces.
