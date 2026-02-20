# syntax=docker/dockerfile:1.7
# FunctionName: lambda-echo
FROM 127.0.0.1:5010/esb-lambda-base:e2e-docker-latest

# ESB Runtime
COPY functions/lambda-echo/sitecustomize.py /opt/python/sitecustomize.py
ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}
# Layer: common-lib-e2e
COPY functions/lambda-echo/layers/common-lib-e2e/ /opt/

# Function code
COPY functions/lambda-echo/src/ ${LAMBDA_TASK_ROOT}/

# Handler
CMD [ "lambda_function.lambda_handler" ]
