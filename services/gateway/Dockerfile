FROM python:3.11-slim

WORKDIR /app

# Install uv
RUN pip install --no-cache-dir uv

# Copy dependencies
COPY pyproject.toml /app/

# Install dependencies (Cache layer)
# pyproject.toml から requirements.txt を生成してインストールすることで、
# 依存関係の変更漏れを防ぎつつ、キャッシュを有効活用する
# 注意: requirements.txt 生成のために一時ファイルが必要な場合があるため
RUN uv pip compile pyproject.toml -o requirements.txt && \
    uv pip install --system --break-system-packages -r requirements.txt

# Copy services code
COPY services/common /app/services/common
COPY services/gateway /app/services/gateway
# Copy Config
COPY config /app/config

EXPOSE 443

CMD ["uvicorn", "services.gateway.main:app", "--host", "0.0.0.0", "--port", "443", "--ssl-keyfile", "/app/config/ssl/server.key", "--ssl-certfile", "/app/config/ssl/server.crt", "--log-config", "/app/config/gateway_log.yaml"]
