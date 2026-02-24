# Where: e2e/fixtures/images/lambda/java/Dockerfile
# What: Minimal AWS Lambda Java 21 container image for E2E image-function tests.
# Why: Validate ImageUri + Java agent injection flow with a deterministic fixture image.
ARG MAVEN_IMAGE=public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7
FROM ${MAVEN_IMAGE} AS builder

WORKDIR /src
COPY pom.xml ./
COPY src ./src

ARG HTTP_PROXY
ARG http_proxy
ARG HTTPS_PROXY
ARG https_proxy
ARG NO_PROXY
ARG no_proxy

RUN mvn -q -DskipTests package

FROM public.ecr.aws/lambda/java:21

RUN mkdir -p ${LAMBDA_TASK_ROOT}/lib
COPY --from=builder /src/target/app.jar ${LAMBDA_TASK_ROOT}/lib/app.jar

CMD ["com.fixtures.imageminimal.Handler::handleRequest"]
