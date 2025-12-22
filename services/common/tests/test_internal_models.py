import pytest
from pydantic import ValidationError


def test_container_ensure_request_validation():
    from services.common.models.internal import ContainerEnsureRequest

    # Valid data
    req = ContainerEnsureRequest(
        function_name="test-func", image="test-image", env={"KEY": "VALUE"}
    )
    assert req.function_name == "test-func"
    assert req.image == "test-image"
    assert req.env == {"KEY": "VALUE"}

    # Missing required field
    with pytest.raises(ValidationError):
        ContainerEnsureRequest()

    # Default values
    req_min = ContainerEnsureRequest(function_name="test-min")
    assert req_min.image is None
    assert req_min.env == {}


def test_container_info_response_validation():
    from services.common.models.internal import ContainerInfoResponse

    # Valid data
    res = ContainerInfoResponse(host="1.2.3.4", port=8080)
    assert res.host == "1.2.3.4"
    assert res.port == 8080

    # Type coercion
    res_str_port = ContainerInfoResponse(host="1.2.3.4", port="8081")
    assert res_str_port.port == 8081

    # Missing field
    with pytest.raises(ValidationError):
        ContainerInfoResponse(host="1.2.3.4")
