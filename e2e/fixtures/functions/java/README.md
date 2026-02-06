# Java E2E Fixtures

This directory contains Java Lambda fixtures used by the E2E templates.

## Build (javac)

Requirements: Java 21 (via mise).

```bash
mise install
cd e2e/fixtures/functions/java/echo
mkdir -p build/classes
mise exec -- javac --release 21 -d build/classes src/main/java/com/fixtures/echo/Handler.java
mise exec -- jar --create --file app.jar -C build/classes .
```

## Optional Maven build

If Maven is available and dependencies can be downloaded, you can also use `pom.xml`.

### Connectivity fixture (AWS SDK v2)

The connectivity fixture depends on AWS SDK v2 modules, so it should be built with Maven:

```bash
cd e2e/fixtures/functions/java/connectivity
mvn -q -DskipTests package
cp target/app.jar app.jar
```
