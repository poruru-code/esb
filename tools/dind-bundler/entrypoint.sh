#!/bin/sh
set -e

# Start Docker daemon in the background
echo "Starting Docker daemon..."
dockerd-entrypoint.sh &
DOCKER_PID=$!

# Wait for Docker to be ready
echo "Waiting for Docker daemon to be ready..."
until docker info >/dev/null 2>&1; do
    echo "Waiting for Docker daemon..."
    sleep 1
done
echo "Docker daemon is ready."

# Load images if the tarball exists
if [ -f /images.tar ]; then
    echo "Loading pre-baked images from /images.tar..."
    docker load -i /images.tar
    echo "Images loaded successfully."

    # Remove the tarball to save space (though it's in the layer, it saves runtime space)
    rm -f /images.tar
    echo "Removed /images.tar to save runtime space."
fi

# Execute the passed command
echo "Executing command: $@"
exec "$@"
