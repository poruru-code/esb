# Where: tools/maven-shim/Dockerfile
# What: Maven shim image that forces proxy-aware settings.xml at runtime.
# Why: Keep Maven proxy behavior deterministic without Dockerfile RUN rewriting.
ARG BASE_MAVEN_IMAGE=public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7
FROM ${BASE_MAVEN_IMAGE}

COPY mvn-wrapper.sh /usr/local/bin/mvn
RUN chmod 0755 /usr/local/bin/mvn
