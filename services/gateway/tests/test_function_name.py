from urllib.parse import quote

import pytest

from services.gateway.core.function_name import normalize_invoke_function_name

TEST_ACCOUNT_ID = "123456789012"
TEST_REGION = "us-east-1"
TEST_FUNCTION_NAME = "lambda-callback-sample"


def test_normalize_keeps_plain_function_name():
    resolved = normalize_invoke_function_name("lambda-echo")

    assert resolved.name == "lambda-echo"
    assert resolved.qualifier is None


def test_normalize_extracts_name_from_full_lambda_arn():
    resolved = normalize_invoke_function_name(
        f"arn:aws:lambda:{TEST_REGION}:{TEST_ACCOUNT_ID}:function:{TEST_FUNCTION_NAME}"
    )

    assert resolved.name == TEST_FUNCTION_NAME
    assert resolved.qualifier is None


def test_normalize_extracts_name_and_qualifier_from_partial_arn():
    resolved = normalize_invoke_function_name(
        f"{TEST_ACCOUNT_ID}:function:{TEST_FUNCTION_NAME}:prod"
    )

    assert resolved.name == TEST_FUNCTION_NAME
    assert resolved.qualifier == "prod"


def test_normalize_extracts_name_and_qualifier_from_bare_name():
    resolved = normalize_invoke_function_name("lambda-echo:v1")

    assert resolved.name == "lambda-echo"
    assert resolved.qualifier == "v1"


def test_normalize_accepts_url_encoded_arn():
    encoded = quote(
        f"arn:aws:lambda:{TEST_REGION}:{TEST_ACCOUNT_ID}:function:{TEST_FUNCTION_NAME}",
        safe="",
    )
    resolved = normalize_invoke_function_name(encoded)

    assert resolved.name == TEST_FUNCTION_NAME


def test_normalize_rejects_empty_name():
    with pytest.raises(ValueError, match="FunctionName is required"):
        normalize_invoke_function_name("   ")
