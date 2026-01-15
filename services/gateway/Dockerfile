# Builder Stage - Standardized via service-base (Debian 12 + Python 3.12)
FROM esb-service-base:latest AS builder

# Install uv (standardized via official image in standard path)
COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /usr/local/bin/

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    python3-dev \
    && rm -rf /var/lib/apt/lists/*

# Create virtual environment
ENV VIRTUAL_ENV=/app/.venv
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy dependencies definition for better cache efficiency
COPY pyproject.toml README.md /app/

# Install dependencies (Cache layer)
RUN uv pip compile pyproject.toml -o requirements.txt && \
    uv pip install -r requirements.txt

# Copy source code for final installation (ensures build-backend sees everything)
COPY tools /app/tools
COPY services /app/services
RUN uv pip install .

# Final stage - Standardized via service-base
FROM esb-service-base:latest AS prod

WORKDIR /app

# Install service-specific runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    haproxy \
    wireguard-tools \
    && rm -rf /var/lib/apt/lists/*

# Fix python symlink safely (python formula might put it in /usr/local/bin)
RUN if ! command -v python >/dev/null 2>&1; then \
        ln -sf "$(command -v python3)" /usr/local/bin/python; \
    fi

# Copy uv for operational use/debugging from standardized path
COPY --from=builder /usr/local/bin/uv /usr/local/bin/uvx /usr/local/bin/

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
# Use ARG with default to improve portability of the COPY command
ARG ESB_CONFIG_DIR=services/gateway/config
COPY ${ESB_CONFIG_DIR}/ /tmp/esb_config/
RUN if [ -f /tmp/esb_config/functions.yml ]; then \
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
