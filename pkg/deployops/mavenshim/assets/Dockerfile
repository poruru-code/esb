# Where: pkg/deployops/mavenshim/assets/Dockerfile
# What: Maven shim image that injects proxy-aware JVM properties at runtime.
# Why: Keep Maven proxy behavior deterministic without Dockerfile RUN rewriting.
ARG BASE_MAVEN_IMAGE=public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7
FROM ${BASE_MAVEN_IMAGE}

COPY mvn-wrapper.sh /tmp/mvn-wrapper.sh
RUN set -euo pipefail; \
    target_mvn="$(command -v mvn)"; \
    cp "${target_mvn}" "${target_mvn}.esb-real"; \
    install -m 0755 /tmp/mvn-wrapper.sh "${target_mvn}"; \
    install -m 0755 /tmp/mvn-wrapper.sh /usr/local/bin/mvn
