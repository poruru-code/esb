FROM python:3.11-slim

WORKDIR /app

RUN pip install --no-cache-dir uv

COPY pyproject.toml /app/

COPY services/manager /app/services/manager

# Install dependencies for Manager
RUN uv pip install --system --break-system-packages "fastapi" "uvicorn[standard]" "docker" "pydantic" "apscheduler"

CMD ["uvicorn", "services.manager.main:app", "--host", "0.0.0.0", "--port", "8081"]
