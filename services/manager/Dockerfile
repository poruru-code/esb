FROM python:3.11-slim

WORKDIR /app

RUN pip install --no-cache-dir uv

COPY pyproject.toml /app/

# Install dependencies for Manager
# 全体の依存関係が入るが、変更反映漏れを防ぐため pyproject.toml を正とする
RUN uv pip compile pyproject.toml -o requirements.txt && \
    uv pip install --system --break-system-packages -r requirements.txt

COPY services/common /app/services/common
COPY services/manager /app/services/manager
COPY config /app/config

CMD ["uvicorn", "services.manager.main:app", "--host", "0.0.0.0", "--port", "8081", "--log-config", "/app/config/manager_log.yaml"]
