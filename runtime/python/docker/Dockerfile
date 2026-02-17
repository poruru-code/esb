# syntax=docker/dockerfile:1.7
# Lambda Base Image
# Base image containing the shared runtime.
FROM public.ecr.aws/lambda/python:3.12@sha256:43106d427ae78760aee589f55be1b1836b41363feaf0ec19ca27aeddde4cb46f

# sitecustomize.py is auto-loaded from /opt/python via PYTHONPATH.
COPY extensions/sitecustomize/site-packages/sitecustomize.py /opt/python/sitecustomize.py

# trace_bridge included in PYTHONPATH as a Lambda Layer.
COPY extensions/trace-bridge/layer/ /opt/python/

ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}
