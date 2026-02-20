# syntax=docker/dockerfile:1.7
# FunctionName: lambda-image
FROM 127.0.0.1:5010/esb-e2e-lambda-python:latest

# ESB Runtime
COPY functions/lambda-image/sitecustomize.py /opt/python/sitecustomize.py
ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}
