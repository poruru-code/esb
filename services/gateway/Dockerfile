FROM python:3.12-slim AS builder

WORKDIR /app

# Install uv
RUN pip install --no-cache-dir uv

# Create virtual environment
ENV VIRTUAL_ENV=/app/.venv
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy dependencies definition and source for package installation
COPY pyproject.toml README.md /app/
# tools package is required for "pip install ." as per pyproject.toml
COPY tools /app/tools

# Install dependencies (Cache layer)
# Generate requirements.txt from pyproject.toml before installing.
# This prevents missed dependency changes while maximizing cache use.
RUN uv pip compile pyproject.toml -o requirements.txt && \
    uv pip install -r requirements.txt

# Install the project itself to generate metadata (dist-info)
# This fixes: PROJECT_NAME = metadata("edge-serverless-box")["Name"]
RUN uv pip install .

# Final stage
FROM python:3.12-slim

WORKDIR /app

# WireGuard (userspace) + routing tools for gateway-side tunnel
RUN apt-get update && apt-get install -y --no-install-recommends \
    haproxy \
    iproute2 \
    wireguard-go \
    wireguard-tools \
  && rm -rf /var/lib/apt/lists/*

# Copy virtual environment from builder
COPY --from=builder /app/.venv /app/.venv
ENV PATH="/app/.venv/bin:$PATH"
ENV PYTHONPATH=/app

# Copy services code
COPY services/common /app/services/common
COPY services/gateway /app/services/gateway
# Copy Config (base: SSL certs, HAProxy, etc.)
COPY config /app/config

# Environment-specific config (functions.yml, routing.yml)
# Build arg: --build-arg ESB_CONFIG_DIR=e2e/fixtures/.esb/e2e-docker/config
# Default to services/gateway/config if not set (keeps build valid, but no baked config)
ARG ESB_CONFIG_DIR=""
COPY ${ESB_CONFIG_DIR:-services/gateway/config}/ /tmp/esb_config/
RUN if [ -n "$ESB_CONFIG_DIR" ] && [ -f /tmp/esb_config/functions.yml ]; then \
      cp /tmp/esb_config/functions.yml /app/config/functions.yml && \
      cp /tmp/esb_config/routing.yml /app/config/routing.yml; \
    else \
      echo "WARNING: Pre-baked configuration not found. Expecting runtime volume mounts."; \
    fi && \
    rm -rf /tmp/esb_config

COPY services/gateway/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

EXPOSE 443

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["uvicorn", "services.gateway.main:app", "--host", "0.0.0.0", "--port", "443", "--ssl-keyfile", "/app/config/ssl/server.key", "--ssl-certfile", "/app/config/ssl/server.crt"]
