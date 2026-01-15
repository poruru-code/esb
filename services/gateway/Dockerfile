# Builder Stage - Wolfi with Python 3.12
FROM cgr.dev/chainguard/wolfi-base:latest AS builder
WORKDIR /app

# Install build dependencies, Python 3.12 and uv
RUN apk update && apk add --no-cache \
    python-3.12 \
    python-3.12-dev \
    uv \
    build-base \
    ca-certificates

# Create virtual environment
ENV VIRTUAL_ENV=/app/.venv
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy dependencies definition and source for package installation
COPY pyproject.toml README.md /app/
COPY tools /app/tools

# Install dependencies (Cache layer)
RUN uv pip compile pyproject.toml -o requirements.txt && \
    uv pip install -r requirements.txt

# Install the project itself to generate metadata
RUN uv pip install .

# Final stage - Wolfi with Python 3.12
FROM cgr.dev/chainguard/wolfi-base:latest

WORKDIR /app

# Install Python 3.12 runtime and network tools
RUN apk update && apk add --no-cache \
    python-3.12 \
    haproxy \
    iproute2 \
    wireguard-go \
    wireguard-tools \
    ca-certificates && \
    ln -sf /usr/bin/python3 /usr/bin/python

# Copy virtual environment from builder
COPY --from=builder /app/.venv /app/.venv
ENV PATH="/app/.venv/bin:$PATH"
ENV PYTHONPATH=/app

# Copy services code
COPY services/common /app/services/common
COPY services/gateway /app/services/gateway
# Copy Config (base: SSL certs, HAProxy, etc.)
COPY config /app/config

# Link the shared CA trust utility for the entrypoint
RUN mkdir -p /usr/local/bin && ln -sf /app/services/common/ensure_ca_trust.sh /usr/local/bin/ensure_ca_trust.sh

# Environment-specific config (functions.yml, routing.yml)
ARG ESB_CONFIG_DIR=""
COPY ${ESB_CONFIG_DIR:-services/gateway/config}/ /tmp/esb_config/
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
