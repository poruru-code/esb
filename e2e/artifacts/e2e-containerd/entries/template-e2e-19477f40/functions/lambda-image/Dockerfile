# syntax=docker/dockerfile:1.7
# FunctionName: lambda-image
FROM 127.0.0.1:5010/esb-e2e-image-java:latest

# ESB Runtime
COPY functions/lambda-image/lambda-java-agent.jar /var/task/lib/lambda-java-agent.jar
ENV JAVA_AGENT_PRESENT=1
ENV JAVA_TOOL_OPTIONS="-javaagent:/var/task/lib/lambda-java-agent.jar ${JAVA_TOOL_OPTIONS}"
