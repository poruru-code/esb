# syntax=docker/dockerfile:1

ARG GO_VERSION=1.24
ARG DEBIAN_VERSION=bookworm
# Specific commit for firecracker-containerd compatibility
ARG FIRECRACKER_CONTAINERD_REF=d6ffdaa615b8da95dea35e611d0290447fcf8fd1
ARG FIRECRACKER_VERSION=1.14.0

# -----------------------------------------------------------------------------
# Stage 1: Firecracker Builder
# Compiles firecracker-containerd, shim, and tools from source
# -----------------------------------------------------------------------------
FROM golang:${GO_VERSION}-${DEBIAN_VERSION} AS firecracker-builder

ARG FIRECRACKER_CONTAINERD_REF

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    git \
    make \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Fetch specific version of firecracker-containerd
RUN git init . \
  && git remote add origin https://github.com/firecracker-microvm/firecracker-containerd.git \
  && git fetch --depth 1 origin "${FIRECRACKER_CONTAINERD_REF}" \
  && git checkout FETCH_HEAD

# Apply patches
COPY patches/firecracker-fifo-reader.patch /tmp/firecracker-fifo-reader.patch
RUN git apply /tmp/firecracker-fifo-reader.patch

# Build components
RUN make -C runtime containerd-shim-aws-firecracker
RUN make -C firecracker-control/cmd/containerd firecracker-containerd firecracker-ctr

# -----------------------------------------------------------------------------
# Stage 2: Firecracker Binaries
# Downloads pre-built Firecracker and Jailer binaries
# -----------------------------------------------------------------------------
FROM debian:${DEBIAN_VERSION}-slim AS firecracker-bins

ARG FIRECRACKER_VERSION
ARG FIRECRACKER_ARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    tar \
  && rm -rf /var/lib/apt/lists/*

RUN set -eu; \
  arch="${FIRECRACKER_ARCH:-$(uname -m)}"; \
  case "$arch" in \
    amd64|x86_64) arch="x86_64" ;; \
    arm64|aarch64) arch="aarch64" ;; \
    *) echo "unsupported architecture: $arch" >&2; exit 1 ;; \
  esac; \
  url="https://github.com/firecracker-microvm/firecracker/releases/download/v${FIRECRACKER_VERSION}/firecracker-v${FIRECRACKER_VERSION}-${arch}.tgz"; \
  curl --retry 5 --retry-connrefused --retry-max-time 60 -fsSL -o /tmp/firecracker.tgz "$url"; \
  tar -xzf /tmp/firecracker.tgz -C /tmp; \
  dir="release-v${FIRECRACKER_VERSION}-${arch}"; \
  install -m 0755 "/tmp/${dir}/firecracker-v${FIRECRACKER_VERSION}-${arch}" /usr/local/bin/firecracker; \
  install -m 0755 "/tmp/${dir}/jailer-v${FIRECRACKER_VERSION}-${arch}" /usr/local/bin/jailer; \
  rm -rf "/tmp/${dir}" /tmp/firecracker.tgz

# -----------------------------------------------------------------------------
# Stage 3: Final Runtime Image
# Assembles the runtime with containerd and firecracker support
# -----------------------------------------------------------------------------
FROM esb-service-base

# System dependencies for containerd and firecracker networking/devmapper
RUN apt-get update && apt-get install -y --no-install-recommends \
    containerd \
    dmsetup \
    runc \
    udev \
    util-linux \
  && rm -rf /var/lib/apt/lists/*

# -- Firecracker Components --
# Comment out these COPY instructions to remove Firecracker support
COPY --from=firecracker-builder /src/runtime/containerd-shim-aws-firecracker /usr/local/bin/
COPY --from=firecracker-builder /src/firecracker-control/cmd/containerd/firecracker-containerd /usr/local/bin/
COPY --from=firecracker-builder /src/firecracker-control/cmd/containerd/firecracker-ctr /usr/local/bin/
COPY --from=firecracker-bins /usr/local/bin/firecracker /usr/local/bin/
COPY --from=firecracker-bins /usr/local/bin/jailer /usr/local/bin/

# -- Configuration --
COPY firecracker-containerd.toml /etc/firecracker-containerd/config.toml
COPY firecracker-runtime.json /etc/containerd/firecracker-runtime.json
COPY firecracker-runc-config.json /etc/containerd/firecracker-runc-config.json

# -- Entrypoints --
COPY entrypoint.common.sh /entrypoint.common.sh
COPY entrypoint.containerd.sh /entrypoint.containerd.sh
COPY entrypoint.firecracker.sh /entrypoint.firecracker.sh
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /entrypoint.containerd.sh /entrypoint.firecracker.sh

ENTRYPOINT ["/entrypoint.sh"]
