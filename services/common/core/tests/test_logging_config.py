import logging
import json
from services.common.core import logging_config, request_context


def test_custom_json_formatter_includes_request_id():
    """Ensure the formatter includes TraceID and RequestID from context."""
    # Clear context.
    request_context.clear_trace_id()

    # Arrange
    trace_id_str = "Root=1-abc-123;Sampled=1"

    # Set context.
    request_context.set_trace_id(trace_id_str)
    req_id_str = request_context.generate_request_id()

    # Create LogRecord.
    record = logging.LogRecord(
        name="test_logger",
        level=logging.INFO,
        pathname="test_path.py",
        lineno=10,
        msg="Test message",
        args=(),
        exc_info=None,
    )

    # Act.
    formatter = logging_config.CustomJsonFormatter()
    log_output = formatter.format(record)
    log_json = json.loads(log_output)

    # Assert.
    assert log_json["message"] == "Test message"
    assert log_json.get("trace_id") == trace_id_str

    # [Red] Current implementation should not include aws_request_id.
    assert log_json.get("aws_request_id") == req_id_str
