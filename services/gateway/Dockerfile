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

# Copy virtual environment from builder
COPY --from=builder /app/.venv /app/.venv
ENV PATH="/app/.venv/bin:$PATH"
ENV PYTHONPATH=/app

# Copy services code
COPY services/common /app/services/common
COPY services/gateway /app/services/gateway
# Copy Config
COPY config /app/config

EXPOSE 443

CMD ["uvicorn", "services.gateway.main:app", "--host", "0.0.0.0", "--port", "443", "--ssl-keyfile", "/app/config/ssl/server.key", "--ssl-certfile", "/app/config/ssl/server.crt"]
