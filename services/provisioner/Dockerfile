# syntax=docker/dockerfile:1.7
# Where: services/provisioner/Dockerfile
# What: Provisioner image (Shared Runtime).
# Why: Provisioner is runtime-agnostic.

ARG PYTHON_BASE_IMAGE

# Metadata Stage - Generate version.json from git context.
FROM python:3.12-alpine AS build-meta

RUN apk add --no-cache git

WORKDIR /work

RUN --mount=type=bind,from=trace_tools,source=.,target=/trace_tools \
    --mount=type=bind,from=git_dir,source=.,target=/gitdir \
    --mount=type=bind,from=git_common,source=.,target=/gitcommon \
    python3 /trace_tools/generate_version_json.py \
      --output /out/version.json \
      --git-dir /gitdir \
      --git-common-dir /gitcommon \
      --image-runtime "shared"

FROM ${PYTHON_BASE_IMAGE}

ENV IMAGE_RUNTIME="shared"

# Install uv (standardized via official image)
COPY --from=ghcr.io/astral-sh/uv:0.8.13@sha256:4de5495181a281bc744845b9579acf7b221d6791f99bcc211b9ec13f417c2853 /uv /uvx /usr/local/bin/

WORKDIR /app

COPY --from=build-meta /out/version.json /app/version.json

# Copy dependencies
COPY services/provisioner/pyproject.toml .

# Create venv & install
ENV VIRTUAL_ENV=/app/.venv
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"
RUN uv pip install -r pyproject.toml

# Copy Code & Assets
# sitecustomize.py is crucial for log forwarding and endpoint injection
COPY cli/internal/generator/assets/site-packages/sitecustomize.py /app/sitecustomize.py
COPY services/provisioner/src /app/src

# Copy Config
COPY --from=config / /tmp/config/
RUN if [ -f /tmp/config/resources.yml ]; then \
      mkdir -p /app/config && \
      cp /tmp/config/resources.yml /app/config/resources.yml; \
    else \
      echo "WARNING: Pre-baked resources.yml not found. Expecting runtime volume mounts."; \
    fi && \
    rm -rf /tmp/config

COPY services/provisioner/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENV PYTHONPATH=/app

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["python", "/app/src/main.py"]
