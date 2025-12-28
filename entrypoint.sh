#!/bin/bash
set -e

echo "Starting Docker daemon..."
# Start the Docker daemon in the background.
dockerd-entrypoint.sh &

# Wait for the Docker daemon to start.
timeout=${DOCKER_DAEMON_TIMEOUT:-60}
while ! docker info > /dev/null 2>&1; do
    if [ $timeout -le 0 ]; then
        echo "ERROR: Docker daemon failed to start"
        exit 1
    fi
    echo "Waiting for Docker daemon to start... ($timeout seconds remaining)"
    sleep 1
    timeout=$((timeout - 1))
done

echo "Docker daemon started successfully"

# Prepare environment variables for the ESB CLI.
# Create tests/.env.test from the template if it doesn't exist.
if [ ! -f /app/tests/.env.test ]; then
    echo "Initializing environment variables from .env.example..."
    mkdir -p /app/tests
    cp /app/.env.example /app/tests/.env.test
fi

# Load prebuilt images (.tar) if present (to speed up startup).
if [ -d /app/build/lambda-images ]; then
    echo "Checking for pre-built images..."
    for tarfile in /app/build/lambda-images/*.tar; do
        if [ -f "$tarfile" ]; then
            echo "Loading pre-built image: $tarfile..."
            docker load -i "$tarfile"
        fi
    done
fi

# Start the environment using the ESB CLI.
# --build: generate configuration and build missing images.
# --detach: start services in the background.
echo "Starting Edge Serverless Box via CLI..."
cd /app
esb up --build --detach

# Tail logs to keep the container alive.
echo "All services started. Tailing logs..."
docker compose logs -f
