# syntax=docker/dockerfile:1.7
# FunctionName: lambda-chaos
FROM registry:5010/esb-lambda-base:e2e-containerd-latest

# ESB Runtime
COPY functions/lambda-chaos/sitecustomize.py /opt/python/sitecustomize.py
ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}
# Layer: common-lib-e2e
COPY functions/lambda-chaos/layers/common-lib-e2e/ /opt/

# Function code
COPY functions/lambda-chaos/src/ ${LAMBDA_TASK_ROOT}/

# Handler
CMD [ "lambda_function.lambda_handler" ]
