# syntax=docker/dockerfile:1.7
# Where: services/provisioner/Dockerfile
# What: Provisioner image (Shared Runtime).
# Why: Provisioner is runtime-agnostic.

ARG PYTHON_BASE_IMAGE

FROM ${PYTHON_BASE_IMAGE} AS deps

ARG PROXY_CA_MOUNT_ID=proxy_ca

ENV UV_NATIVE_TLS=true

# Install uv (standardized via official image)
COPY --from=ghcr.io/astral-sh/uv:0.8.13@sha256:4de5495181a281bc744845b9579acf7b221d6791f99bcc211b9ec13f417c2853 /uv /uvx /usr/local/bin/

WORKDIR /app

# Copy dependencies
COPY pyproject.toml uv.lock /app/

# Create venv & install
ENV VIRTUAL_ENV=/app/.venv
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"
RUN --mount=type=cache,target=/root/.cache/uv \
    --mount=type=secret,id=${PROXY_CA_MOUNT_ID},required=false \
    if [ -s "/run/secrets/${PROXY_CA_MOUNT_ID}" ]; then \
      rm -rf /usr/local/share/ca-certificates/proxy-ca && \
      mkdir -p /usr/local/share/ca-certificates/proxy-ca && \
      awk 'BEGIN { n=0 } /-----BEGIN CERTIFICATE-----/ { n++ } { if (n > 0) print > ("/usr/local/share/ca-certificates/proxy-ca/proxy-ca-" sprintf("%03d", n) ".crt") }' /run/secrets/${PROXY_CA_MOUNT_ID} && \
      if [ ! -f /usr/local/share/ca-certificates/proxy-ca/proxy-ca-001.crt ]; then \
        install -m 0644 /run/secrets/${PROXY_CA_MOUNT_ID} /usr/local/share/ca-certificates/proxy-ca/proxy-ca-001.crt; \
      fi && \
      update-ca-certificates; \
    fi && \
    SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt \
    REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt \
    CURL_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt \
    uv sync --native-tls --frozen --no-install-project

FROM ${PYTHON_BASE_IMAGE}

ENV IMAGE_RUNTIME="shared"

WORKDIR /app

# Keep uv available in runtime for operational consistency.
COPY --from=deps /usr/local/bin/uv /usr/local/bin/uvx /usr/local/bin/
COPY --from=deps /app/.venv /app/.venv

ENV VIRTUAL_ENV=/app/.venv
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Copy Code & Assets
# sitecustomize.py is crucial for log forwarding and endpoint injection
COPY --from=generator_assets /sitecustomize.py /app/sitecustomize.py
COPY src /app/src

# Copy Config
COPY --from=config / /tmp/config/
RUN if [ -f /tmp/config/resources.yml ]; then \
      mkdir -p /app/runtime-config && \
      cp /tmp/config/resources.yml /app/runtime-config/resources.yml; \
    else \
      echo "WARNING: Pre-baked resources.yml not found. Expecting runtime volume mounts."; \
    fi && \
    rm -rf /tmp/config

COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh


ENV PYTHONPATH=/app

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["python", "/app/src/main.py"]
