# syntax=docker/dockerfile:1.7
# FunctionName: lambda-scheduled
FROM 127.0.0.1:5010/esb-lambda-base:latest

# ESB Runtime
COPY functions/lambda-scheduled/sitecustomize.py /opt/python/sitecustomize.py
ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}
# Layer: common-lib-e2e
COPY functions/lambda-scheduled/layers/common-lib-e2e/ /opt/

# Function code
COPY functions/lambda-scheduled/src/ ${LAMBDA_TASK_ROOT}/

# Handler
CMD [ "lambda_function.lambda_handler" ]
