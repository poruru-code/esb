# Where: e2e/fixtures/images/lambda/java/Dockerfile
# What: Minimal AWS Lambda Java 21 container image for E2E image-function tests.
# Why: Validate ImageUri + Java agent injection flow with a deterministic fixture image.
FROM maven:3.9.11-eclipse-temurin-21 AS builder

WORKDIR /src
COPY pom.xml ./
COPY src ./src
RUN mvn -q -DskipTests package

FROM public.ecr.aws/lambda/java:21

RUN mkdir -p ${LAMBDA_TASK_ROOT}/lib
COPY --from=builder /src/target/app.jar ${LAMBDA_TASK_ROOT}/lib/app.jar

CMD ["com.fixtures.imageminimal.Handler::handleRequest"]
