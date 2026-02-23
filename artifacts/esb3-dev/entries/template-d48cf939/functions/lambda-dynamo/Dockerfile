# syntax=docker/dockerfile:1.7
# FunctionName: lambda-dynamo
FROM 127.0.0.1:5010/esb-lambda-base:latest

# ESB Runtime
COPY functions/lambda-dynamo/sitecustomize.py /opt/python/sitecustomize.py
ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}
# Layer: common-lib-e2e
COPY functions/lambda-dynamo/layers/common-lib-e2e/ /opt/

# Function code
COPY functions/lambda-dynamo/src/ ${LAMBDA_TASK_ROOT}/

# Handler
CMD [ "lambda_function.lambda_handler" ]
