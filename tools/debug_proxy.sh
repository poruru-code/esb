#!/bin/bash
# tools/debug_proxy.sh
# Diagnostic script to verify proxy configuration and connectivity.
# Outputs to diagnostics/proxy_debug.log

set -e

# Setup logging
mkdir -p diagnostics
LOG_FILE="diagnostics/proxy_debug.log"

# Function to log and echo
log() {
    echo "$1" | tee -a "$LOG_FILE"
}

# Start Logging
echo "=== Proxy Debug Log Started at $(date) ===" > "$LOG_FILE"

log "=========================================="
log " 1. Host Environment Variables"
log "=========================================="




# Load e2e-docker profile env
if [ -f "tests/environments/.env.docker" ]; then
  log "Loading tests/environments/.env.docker..."
  export $(grep -v '^#' tests/environments/.env.docker | xargs)
fi

# Set defaults for required vars IF missing (Safety net)
export RUSTFS_ACCESS_KEY=${RUSTFS_ACCESS_KEY:-rustfsadmin}
export RUSTFS_SECRET_KEY=${RUSTFS_SECRET_KEY:-rustfsadmin}

for var in HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy; do
    log "$var=$(printenv $var || echo '<unset>')"
done

log ""
log "=========================================="
log " 2. Docker Compose Config (Effective)"
log "=========================================="
# Check s3-storage as a representative service
docker compose -f docker-compose.yml config s3-storage | grep -i -A 10 "environment" >> "$LOG_FILE" 2>&1 || log "No environment section found for s3-storage"

log ""
log "=========================================="
log " 3. Container Runtime Environment"
log "=========================================="
log "Inspecting env vars of RUNNING s3-storage container..."
# Using exec to see the actual environment of the container started by run_tests.py
if [ -n "$(docker compose -f docker-compose.yml ps -q s3-storage)" ]; then
    docker compose -f docker-compose.yml exec s3-storage env > /tmp/esb_container_env 2>&1
    grep -i proxy /tmp/esb_container_env | tee -a "$LOG_FILE" || log "No proxy variables found inside running container!"
else
    log "WARNING: s3-storage container is NOT running. Cannot inspect exact test state."
    log "Attempting to run a fresh container (Note: This may not match run_tests.py environment)..."
    docker compose -f docker-compose.yml run --rm --entrypoint env s3-storage > /tmp/esb_container_env 2>&1
    grep -i proxy /tmp/esb_container_env | tee -a "$LOG_FILE" || log "No proxy variables found inside fresh container!"
fi

log ""
log "=========================================="
log " 4. Connectivity Check (Internal)"
log "=========================================="
# Testing if s3-storage can reach gateway (should be NO_PROXY)
# Note: we need gateway running for this, checking if it's up, otherwise start it.
if [ -z "$(docker compose -f docker-compose.yml ps -q gateway)" ]; then
    log "Gateway is not running. Starting minimal stack..."
    docker compose -f docker-compose.yml up -d gateway database s3-storage >> "$LOG_FILE" 2>&1
    sleep 5
fi

log "Testing connection from s3-storage to gateway (internal)..."
# Gateway listens on 443 (HTTPS) inside. Using -k for self-signed certs.
log "curl -k -v https://gateway:443/health"
docker compose -f docker-compose.yml exec s3-storage curl -k -v https://gateway:443/health >> "$LOG_FILE" 2>&1 && log "SUCCESS: Internal connection to Gateway established" || log "FAILURE: Could not connect to internal Gateway"

log ""
log "=========================================="
log " 5. Connectivity Check (External)"
log "=========================================="
# Testing if s3-storage can reach google.com (should be PROXY)
log "Testing connection from s3-storage to google.com (external)..."
docker compose -f docker-compose.yml exec s3-storage curl -I https://www.google.com >> "$LOG_FILE" 2>&1 && log "SUCCESS: External connection to Google established" || log "FAILURE: Could not reach external internet"

log ""
log "Diagnostic complete. Log saved to $LOG_FILE"
