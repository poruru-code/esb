# syntax=docker/dockerfile:1.7
# Lambda Base Image
# Base image containing the shared runtime.
FROM public.ecr.aws/lambda/python:3.12@sha256:43106d427ae78760aee589f55be1b1836b41363feaf0ec19ca27aeddde4cb46f

# Build context must be repository root because runtime hooks are split into runtime-hooks/.
# sitecustomize.py is auto-loaded from /opt/python via PYTHONPATH.
COPY runtime-hooks/python/sitecustomize/site-packages/sitecustomize.py /opt/python/sitecustomize.py

# trace_bridge included in PYTHONPATH as a Lambda Layer.
COPY runtime-hooks/python/trace-bridge/layer/ /opt/python/

ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}
