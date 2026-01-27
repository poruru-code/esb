#!/usr/bin/env bash
# Where: tools/dind-bundler/build.sh
# What: Build a self-contained DinD bundle image.
# Why: Package prebuilt ESB images for offline/demo use.

set -euo pipefail

DEFAULTS_FILE="${DEFAULTS_FILE:-config/defaults.env}"
CLI_CMD=""
ENV_PREFIX=""
if [ -f "$DEFAULTS_FILE" ]; then
  CLI_CMD="$(awk -F= '/^CLI_CMD=/{print $2; exit}' "$DEFAULTS_FILE")"
  ENV_PREFIX="$(awk -F= '/^ENV_PREFIX=/{print $2; exit}' "$DEFAULTS_FILE")"
fi
CLI_CMD="${CLI_CMD:-esb}"
ENV_PREFIX="${ENV_PREFIX:-ESB}"
BRAND_SLUG="$(echo "$CLI_CMD" | tr '[:upper:]' '[:lower:]')"

# Usage: ./tools/dind-bundler/build.sh [TEMPLATE_PATH] [OUTPUT_TAG]
TEMPLATE_PATH="${1:-}"
if [ -z "$TEMPLATE_PATH" ]; then
  echo "Error: TEMPLATE_PATH is required."
  echo "Usage: ./tools/dind-bundler/build.sh <sam-template-path> <output-image-tag>"
  exit 1
fi
OUTPUT_TAG=${2:-"${BRAND_SLUG}-dind-bundle:latest"}
BUILD_DIR="tools/dind-bundler/build-context"
ENV_VAR="${ENV_PREFIX}_ENV"
ENV_NAME="${!ENV_VAR:-${ESB_ENV:-}}"
OUTPUT_VAR="${ENV_PREFIX}_OUTPUT_DIR"
OUTPUT_ROOT="${!OUTPUT_VAR:-${ESB_OUTPUT_DIR:-.${BRAND_SLUG}}}"

if [ -z "$ENV_NAME" ] && [ -f ".env" ]; then
  ENV_NAME="$(awk -F= '/^ENV=/{print $2; exit}' .env)"
fi
ENV_NAME="${ENV_NAME:-default}"
MANIFEST_PATH="${BUNDLE_MANIFEST_PATH:-${OUTPUT_ROOT}/${ENV_NAME}/bundle/manifest.json}"

CERT_DIR="${CERT_DIR:-$HOME/.${BRAND_SLUG}/certs}"

# Ensure we are in the project root
if [ ! -f "cli/go.mod" ]; then
  echo "Error: Please run this script from the project root."
  exit 1
fi

echo "Building DinD bundle from template: $TEMPLATE_PATH"
echo "Output tag: $OUTPUT_TAG"
echo "Env: $ENV_NAME"
echo "Brand: $CLI_CMD"

if [ "${DIND_BUNDLER_DRYRUN:-}" = "true" ]; then
  echo "CLI_CMD=$CLI_CMD"
  echo "ENV_PREFIX=$ENV_PREFIX"
  echo "BRAND_SLUG=$BRAND_SLUG"
  echo "OUTPUT_TAG=$OUTPUT_TAG"
  echo "ENV_NAME=$ENV_NAME"
  echo "OUTPUT_ROOT=$OUTPUT_ROOT"
  echo "MANIFEST_PATH=$MANIFEST_PATH"
  echo "CERT_DIR=$CERT_DIR"
  exit 0
fi

# Check Certs (no fallback)
if [ ! -f "$CERT_DIR/rootCA.crt" ]; then
  echo "Error: Root CA not found at $CERT_DIR/rootCA.crt"
  echo "Hint: run 'mise run setup:certs' or set CERT_DIR."
  exit 1
fi

# Cleanup and setup build context
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# 1. Build Base Services and Functions using esb build
if [ "${SKIP_ESB_BUILD:-}" = "true" ]; then
  echo "Skipping esb build (SKIP_ESB_BUILD=true)..."
else
  echo "Running esb build..."
  uv run "$CLI_CMD" build --template "$TEMPLATE_PATH" --env "$ENV_NAME" --mode docker --verbose --no-save-defaults --bundle-manifest
fi

# 2. Load manifest (source of truth)
if [ ! -f "$MANIFEST_PATH" ]; then
  echo "Error: Bundle manifest not found at $MANIFEST_PATH"
  echo "Hint: run the build with --bundle-manifest (or disable SKIP_ESB_BUILD)."
  exit 1
fi

echo "Loading bundle manifest: $MANIFEST_PATH"
IMAGES=$(python - <<'PY' "$MANIFEST_PATH"
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as f:
    data = json.load(f)

images = data.get("images", [])
if not images:
    print("", end="")
    sys.exit(0)

for item in images:
    name = item.get("name")
    if name:
        print(name)
PY
)

if [ -z "$IMAGES" ]; then
  echo "Error: Manifest contains no images."
  exit 1
fi

# 3. Save Images
echo "Saving images to tarball..."
docker save -o "$BUILD_DIR/images.tar" $IMAGES

# 4. Prepare Context
echo "Copying config files..."
cp docker-compose.docker.yml "$BUILD_DIR/"
cp -r config "$BUILD_DIR/"
cp tools/dind-bundler/Dockerfile "$BUILD_DIR/"
cp tools/dind-bundler/entrypoint.sh "$BUILD_DIR/"
mkdir -p "$BUILD_DIR/bundle"
cp "$MANIFEST_PATH" "$BUILD_DIR/bundle/manifest.json"

if [ -f ".env" ]; then
  cp ".env" "$BUILD_DIR/.env"
  if ! grep -q '^CERT_DIR=' "$BUILD_DIR/.env"; then
    echo "CERT_DIR=/root/.${BRAND_SLUG}/certs" >> "$BUILD_DIR/.env"
  fi
else
  {
    echo "ENV=$ENV_NAME"
    echo "RUSTFS_ACCESS_KEY=${RUSTFS_ACCESS_KEY:-esb}"
    echo "RUSTFS_SECRET_KEY=${RUSTFS_SECRET_KEY:-esbsecret}"
    echo "CERT_DIR=/root/.${BRAND_SLUG}/certs"
  } > "$BUILD_DIR/.env"
fi

# Copy certs
echo "Copying certificates..."
mkdir -p "$BUILD_DIR/certs"
cp "$CERT_DIR"/* "$BUILD_DIR/certs/"

# 5. Build DinD Image
echo "Building DinD image..."
docker build -t "$OUTPUT_TAG" --build-arg BRAND_HOME=".${BRAND_SLUG}" "$BUILD_DIR"

echo "Done! Image $OUTPUT_TAG created."
