# Builder Stage
FROM cgr.dev/chainguard/go:latest AS builder

WORKDIR /app

# Copy go.mod and go.sum from the agent service directory
COPY services/agent/go.mod services/agent/go.sum ./
RUN go mod download

# Copy the entire agent service source code
COPY services/agent/ .

# Build the binary (main is in cmd/agent relative to services/agent/)
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/agent ./cmd/agent

# Production Stage (Standardized via service-base)
FROM esb-service-base:latest AS prod

WORKDIR /app

# Install containerd runtime tools (apt-get as base is Debian)
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && CNI_PLUGINS_VERSION="v1.3.0" \
    && ARCH=$(uname -m) \
    && if [ "$ARCH" = "x86_64" ]; then ARCH="amd64"; fi \
    && if [ "$ARCH" = "aarch64" ]; then ARCH="arm64"; fi \
    && curl -L -o cni-plugins.tgz "https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGINS_VERSION}/cni-plugins-linux-${ARCH}-${CNI_PLUGINS_VERSION}.tgz" \
    && mkdir -p /opt/cni/bin \
    && tar -C /opt/cni/bin -xzf cni-plugins.tgz \
    && rm cni-plugins.tgz \
    && apt-get purge -y --auto-remove curl \
    && rm -rf /var/lib/apt/lists/*

# Path is now relative to project root (context=.)
COPY services/agent/config/cni/ /etc/cni/net.d/

COPY --from=builder /app/agent /app/agent

# entrypoint.sh still needs to be copied as it's service-specific
COPY services/agent/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENV PORT=50051

EXPOSE ${PORT}

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/agent"]
