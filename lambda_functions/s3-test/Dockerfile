FROM public.ecr.aws/lambda/python:3.12

# Lambda Layerをコピー（/opt配下に配置）
# ビルドコンテキストはプロジェクトルートから実行
# 注: boto3はベースイメージに含まれている
COPY lambda_functions/LayerLib/lambda_lib/python/ /opt/python/

# Lambda関数のコードをコピー
COPY lambda_functions/s3-test/lambda_function.py ${LAMBDA_TASK_ROOT}/

# PYTHONPATHにLayerのパスを追加
ENV PYTHONPATH="${PYTHONPATH}:/opt/python"

CMD [ "lambda_function.lambda_handler" ]
