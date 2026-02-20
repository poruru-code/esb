# syntax=docker/dockerfile:1.7
# FunctionName: lambda-scaling
FROM 127.0.0.1:5010/esb-lambda-base:e2e-docker-latest

# ESB Runtime
COPY functions/lambda-scaling/sitecustomize.py /opt/python/sitecustomize.py
ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}
# Layer: common-lib-e2e
COPY functions/lambda-scaling/layers/common-lib-e2e/ /opt/

# Function code
COPY functions/lambda-scaling/src/ ${LAMBDA_TASK_ROOT}/

# Handler
CMD [ "lambda_function.lambda_handler" ]
