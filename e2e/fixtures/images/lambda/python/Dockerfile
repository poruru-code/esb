# Where: e2e/fixtures/images/lambda/python/Dockerfile
# What: Minimal AWS Lambda Python 3.12 container image for E2E tests.
# Why: Validate SAM PackageType=Image flow with a known-good runtime image.
FROM public.ecr.aws/lambda/python:3.12

COPY lambda_function.py ${LAMBDA_TASK_ROOT}/lambda_function.py

CMD ["lambda_function.lambda_handler"]
