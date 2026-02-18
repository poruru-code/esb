#!/usr/bin/env bash
# Where: tools/ci/check_java_reproducible_jars.sh
# What: Static guard for reproducible Java JAR build settings.
# Why: Prevent drift that breaks byte-for-byte reproducibility.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
failures=0

require_pattern() {
  local pattern="$1"
  local file="$2"
  if ! rg -n --fixed-strings -- "$pattern" "$file" >/dev/null; then
    echo "[java-repro] MISSING: '$pattern' in $file" >&2
    failures=$((failures + 1))
  fi
}

RUNTIME_BUILD_POM="$ROOT_DIR/runtime-hooks/java/build/pom.xml"
RUNTIME_WRAPPER_POM="$ROOT_DIR/runtime-hooks/java/wrapper/pom.xml"
RUNTIME_AGENT_POM="$ROOT_DIR/runtime-hooks/java/agent/pom.xml"
E2E_ECHO_POM="$ROOT_DIR/e2e/fixtures/functions/java/echo/pom.xml"
E2E_CONNECTIVITY_POM="$ROOT_DIR/e2e/fixtures/functions/java/connectivity/pom.xml"

for pom in \
  "$RUNTIME_BUILD_POM" \
  "$E2E_ECHO_POM" \
  "$E2E_CONNECTIVITY_POM"; do
  require_pattern "<project.build.outputTimestamp>" "$pom"
done

require_pattern "<maven.compiler.plugin.version>" "$RUNTIME_BUILD_POM"
require_pattern "<maven.shade.plugin.version>" "$RUNTIME_BUILD_POM"
require_pattern "<maven.jar.plugin.version>" "$RUNTIME_BUILD_POM"

require_pattern '<version>${maven.shade.plugin.version}</version>' "$RUNTIME_WRAPPER_POM"
require_pattern '<outputTimestamp>${project.build.outputTimestamp}</outputTimestamp>' "$RUNTIME_WRAPPER_POM"
require_pattern '<version>${maven.shade.plugin.version}</version>' "$RUNTIME_AGENT_POM"
require_pattern '<outputTimestamp>${project.build.outputTimestamp}</outputTimestamp>' "$RUNTIME_AGENT_POM"

require_pattern "<artifactId>maven-jar-plugin</artifactId>" "$E2E_ECHO_POM"
require_pattern '<version>${maven.jar.plugin.version}</version>' "$E2E_ECHO_POM"
require_pattern '<outputTimestamp>${project.build.outputTimestamp}</outputTimestamp>' "$E2E_ECHO_POM"

require_pattern '<version>${maven.shade.plugin.version}</version>' "$E2E_CONNECTIVITY_POM"
require_pattern '<outputTimestamp>${project.build.outputTimestamp}</outputTimestamp>' "$E2E_CONNECTIVITY_POM"

if (( failures > 0 )); then
  echo "[java-repro] FAILED ($failures issue(s))" >&2
  exit 1
fi

echo "[java-repro] OK"
