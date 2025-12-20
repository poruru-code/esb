FROM public.ecr.aws/lambda/python:3.12

# Install requests
RUN pip install requests

# Copy function code
COPY lambda_functions/invoke-test/lambda_function.py ${LAMBDA_TASK_ROOT}/

CMD [ "lambda_function.lambda_handler" ]
