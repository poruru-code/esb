# Builder Stage
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/agent ./cmd/agent

# Production Stage
FROM alpine:3.19 AS prod

WORKDIR /app

# Install runtime dependencies: ca-certificates, iptables, and containerd (for ctr)
RUN apk --no-cache add ca-certificates iptables containerd-ctr curl

# Install CNI plugins
RUN CNI_PLUGINS_VERSION="v1.3.0" && \
    ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then ARCH="amd64"; fi && \
    if [ "$ARCH" = "aarch64" ]; then ARCH="arm64"; fi && \
    curl -L -o cni-plugins.tgz "https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGINS_VERSION}/cni-plugins-linux-${ARCH}-${CNI_PLUGINS_VERSION}.tgz" && \
    mkdir -p /opt/cni/bin && \
    tar -C /opt/cni/bin -xzf cni-plugins.tgz && \
    rm cni-plugins.tgz

COPY config/cni/ /etc/cni/net.d/

COPY --from=builder /app/agent /app/agent

ENV PORT=50051

EXPOSE ${PORT}

CMD ["/app/agent"]
