# syntax=docker/dockerfile:1.7
# Where: services/provisioner/Dockerfile
# What: Provisioner image (Shared Runtime).
# Why: Provisioner is runtime-agnostic.

ARG PYTHON_BASE_IMAGE

FROM ${PYTHON_BASE_IMAGE}

ENV IMAGE_RUNTIME="shared"

# Install uv (standardized via official image)
COPY --from=ghcr.io/astral-sh/uv:0.8.13@sha256:4de5495181a281bc744845b9579acf7b221d6791f99bcc211b9ec13f417c2853 /uv /uvx /usr/local/bin/

WORKDIR /app

# Copy dependencies
COPY pyproject.toml uv.lock .

# Create venv & install
ENV VIRTUAL_ENV=/app/.venv
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-install-project

# Copy Code & Assets
# sitecustomize.py is crucial for log forwarding and endpoint injection
COPY --from=generator_assets /sitecustomize.py /app/sitecustomize.py
COPY src /app/src

# Copy Config
COPY --from=config / /tmp/config/
RUN if [ -f /tmp/config/resources.yml ]; then \
      mkdir -p /app/config && \
      cp /tmp/config/resources.yml /app/config/resources.yml; \
    else \
      echo "WARNING: Pre-baked resources.yml not found. Expecting runtime volume mounts."; \
    fi && \
    rm -rf /tmp/config

COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh


ENV PYTHONPATH=/app

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["python", "/app/src/main.py"]
