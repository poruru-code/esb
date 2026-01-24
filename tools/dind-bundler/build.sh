#!/bin/bash
set -e

# Usage: ./tools/dind-bundler/build.sh [TEMPLATE_PATH] [OUTPUT_TAG]
TEMPLATE_PATH=${1:-"e2e/fixtures/template.yaml"}
OUTPUT_TAG=${2:-"esb-dind-bundle:latest"}
BUILD_DIR="tools/dind-bundler/build-context"

# Ensure we are in the project root
if [ ! -f "cli/go.mod" ]; then
    echo "Error: Please run this script from the project root."
    exit 1
fi

echo "Building DinD bundle from template: $TEMPLATE_PATH"
echo "Output tag: $OUTPUT_TAG"

# Check/Generate Certs
if [ ! -f "$HOME/.esb/certs/rootCA.crt" ]; then
    echo "Root CA not found. Generating dummy certs..."
    ./tools/dind-bundler/generate_dummy_certs.sh
fi

# Cleanup and setup build context
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# 1. Build Base Services and Functions using esb build
if [ "$SKIP_ESB_BUILD" = "true" ]; then
    echo "Skipping esb build (SKIP_ESB_BUILD=true)..."
else
    echo "Building esb CLI..."
    (cd cli && go build -o ../bin/esb cmd/esb/main.go)
    ESB_CMD="$(pwd)/bin/esb"

    echo "Running esb build..."
    # esb build will build base images (gateway, agent, etc.) and function images
    $ESB_CMD build --template "$TEMPLATE_PATH" --env "default" --mode docker --verbose
fi

# 2. Identify and Pull/Tag Images
IMAGES=()

# External images (must match docker-compose.docker.yml)
# Note: alpine is used without tag in compose file, defaulting to latest
EXTERNAL_IMAGES=(
    "scylladb/scylla:latest"
    "rustfs/rustfs:latest"
    "victorialogs/victoria-logs:latest"
    "alpine:latest"
)

echo "Pulling external images..."
for img in "${EXTERNAL_IMAGES[@]}"; do
    echo "Pulling $img..."
    docker pull "$img"
    IMAGES+=("$img")
done

# Internal Base Images (built by esb build)
INTERNAL_IMAGES=(
    "esb-os-base:latest"
    "esb-python-base:latest"
    "esb-gateway:docker"
    "esb-agent:docker"
    "esb-provisioner:docker"
)

# Verify they exist
for img in "${INTERNAL_IMAGES[@]}"; do
    if ! docker image inspect "$img" >/dev/null 2>&1; then
        echo "Error: Image $img was not found. Did esb build fail?"
        exit 1
    fi
    IMAGES+=("$img")
done

# Function Images
# Filter by label com.esb.kind=function
echo "Finding function images..."
FUNC_IMAGES=$(docker image ls --filter "label=com.esb.kind=function" --format "{{.Repository}}:{{.Tag}}")

if [ -z "$FUNC_IMAGES" ]; then
    echo "Warning: No function images found with label com.esb.kind=function"
else
    for img in $FUNC_IMAGES; do
        echo "Found function image: $img"
        IMAGES+=("$img")
    done
fi

# 3. Save Images
echo "Saving images to tarball..."
docker save -o "$BUILD_DIR/images.tar" "${IMAGES[@]}"

# 4. Prepare Context
echo "Copying config files..."
cp docker-compose.docker.yml "$BUILD_DIR/"
cp -r config "$BUILD_DIR/"
cp tools/dind-bundler/Dockerfile "$BUILD_DIR/"
cp tools/dind-bundler/entrypoint.sh "$BUILD_DIR/"

# Copy certs
echo "Copying certificates..."
mkdir -p "$BUILD_DIR/certs"
cp "$HOME/.esb/certs/"* "$BUILD_DIR/certs/"

# 5. Build DinD Image
echo "Building DinD image..."
docker build -t "$OUTPUT_TAG" "$BUILD_DIR"

echo "Done! Image $OUTPUT_TAG created."
