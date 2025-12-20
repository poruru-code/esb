FROM python:3.11-slim

WORKDIR /app

# Install uv
RUN pip install --no-cache-dir uv

# Copy dependencies
COPY pyproject.toml /app/

# Copy services code
COPY services/gateway /app/services/gateway
# We need namespace packages? services/__init__.py might need to exist if we import services...
# But imports inside services/gateway use relative imports from ., so it's a package.
# If we run uvicorn services.gateway.main:app, we need services folder in pythonpath.
# COPY puts it in /app/services/gateway. So /app is pythonpath.

# Copy Config
COPY gateway/config /app/config

# Install dependencies
RUN uv pip install --system --break-system-packages "fastapi" "uvicorn[standard]" "requests" "httpx" "pyjwt" "respx" "pyyaml" "pydantic" 

EXPOSE 443

CMD ["uvicorn", "services.gateway.main:app", "--host", "0.0.0.0", "--port", "443", "--ssl-keyfile", "/app/config/ssl/server.key", "--ssl-certfile", "/app/config/ssl/server.crt"]
