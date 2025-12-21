import json
import io
import sys
from unittest.mock import patch
import os

# Set local env for testing
os.environ["LOCAL_LAMBDA_ENV"] = "true"

from lambda_lib.python.cloudwatch_logger import CloudWatchLogger, LocalLogClient

def test_local_log_client_uses_print_instead_of_urllib():
    client = LocalLogClient()
    
    # Track stdout
    captured_output = io.StringIO()
    sys.stdout = captured_output
    
    try:
        with patch("urllib.request.urlopen") as mock_url:
            client.put_log_events("test-group", "test-stream", [{"timestamp": 123000, "message": "hello test"}])
            
            # URLLIB should NOT be called in the new implementation
            mock_url.assert_not_called()
            
            # Check stdout
            output = captured_output.getvalue().strip()
            assert output != ""
            
            # Verify it is valid JSON
            log_data = json.loads(output)
            assert log_data["_msg"] == "hello test"
            assert log_data["log_group"] == "test-group"
    finally:
        sys.stdout = sys.__stdout__

def test_cloudwatch_logger_local_behavior():
    logger = CloudWatchLogger("test-user")
    
    captured_output = io.StringIO()
    sys.stdout = captured_output
    
    try:
        logger.info(lambda: "something happened")
        output = captured_output.getvalue().strip()
        assert "something happened" in output
        assert '"level": "INFO"' in output or '"_msg":' in output # depends on implementation
    finally:
        sys.stdout = sys.__stdout__
