# syntax=docker/dockerfile:1
# Runtime node image for containerd-only mode.
# Why: Avoid shipping Firecracker binaries when not required.

ARG IMAGE_PREFIX=esb
ARG DEBIAN_VERSION=bookworm
ARG OS_BASE_IMAGE=${IMAGE_PREFIX}-os-base:latest

FROM ${OS_BASE_IMAGE}

# System dependencies for containerd networking/devmapper
RUN apt-get update && apt-get install -y --no-install-recommends \
    containerd \
    dmsetup \
    iproute2 \
    iptables \
    runc \
    udev \
    util-linux \
  && rm -rf /var/lib/apt/lists/*

# -- Entrypoints --
COPY entrypoint.common.sh /entrypoint.common.sh
COPY entrypoint.containerd.sh /entrypoint.containerd.sh
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /entrypoint.containerd.sh

ENTRYPOINT ["/entrypoint.sh"]
