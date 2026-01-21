# syntax=docker/dockerfile:1.4
FROM esb-python-base:latest

# Install uv (standardized via official image)
COPY --from=ghcr.io/astral-sh/uv:0.8.13@sha256:4de5495181a281bc744845b9579acf7b221d6791f99bcc211b9ec13f417c2853 /uv /uvx /usr/local/bin/

WORKDIR /app

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

ENV PYTHONPATH=/app
CMD ["python", "/app/src/main.py"]
