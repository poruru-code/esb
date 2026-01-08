
# Helper to import the target file since it's not in standard path
import importlib.util
import io
import json
import os
import sys
import unittest
from unittest.mock import MagicMock, patch

spec = importlib.util.spec_from_file_location("sitecustomize", "tools/generator/runtime/site-packages/sitecustomize.py")
sitecustomize = importlib.util.module_from_spec(spec)
sys.modules["sitecustomize"] = sitecustomize
spec.loader.exec_module(sitecustomize)

class TestSiteCustomize(unittest.TestCase):

    def setUp(self):
        self.original_stdout = sys.stdout
        self.original_stderr = sys.stderr
        # Reset trace context
        sitecustomize._trace_context.current_trace_id = None
        sitecustomize._trace_context.current_request_id = None

    def tearDown(self):
        sys.stdout = self.original_stdout
        sys.stderr = self.original_stderr

    @patch("urllib.request.urlopen")
    def test_log_shipping_success(self, mock_urlopen):
        """Verify that VictoriaLogsStdoutHook sends logs via HTTP."""
        mock_response = MagicMock()
        mock_response.__enter__.return_value = mock_response
        mock_urlopen.return_value = mock_response

        # Mock stdout buffer
        mock_stdout = io.StringIO()
        
        hook = sitecustomize.VictoriaLogsStdoutHook(
            mock_stdout, 
            container_name="test-container", 
            vl_url="http://localhost:9428"
        )

        log_msg = json.dumps({"level": "INFO", "message": "test log"}) + "\n"
        hook.write(log_msg)

        # Verify original stdout wrote it
        self.assertEqual(mock_stdout.getvalue(), log_msg)

        # Verify urllib was called
        mock_urlopen.assert_called_once()
        args, kwargs = mock_urlopen.call_args
        req = args[0]
        self.assertEqual(req.full_url, "http://localhost:9428/insert/jsonline?_stream_fields=container_name%2Cjob&_msg_field=message&_time_field=_time&container_name=test-container&job=lambda")
        
        # Verify payload
        sent_data = json.loads(req.data)
        self.assertEqual(sent_data["message"], "test log")
        self.assertEqual(sent_data["container_name"], "test-container")
        self.assertIn("_time", sent_data)

    @patch("urllib.request.urlopen")
    def test_log_shipping_timeout(self, mock_urlopen):
        """Verify that timeout/error is handled silently and does not crash."""
        mock_urlopen.side_effect = TimeoutError("Timed out")

        mock_stdout = io.StringIO()
        hook = sitecustomize.VictoriaLogsStdoutHook(
            mock_stdout, 
            container_name="test-container", 
            vl_url="http://localhost:9428"
        )

        try:
            hook.write("test message\n")
        except TimeoutError:
            self.fail("Hook raised TimeoutError instead of suppressing it")

        self.assertEqual(mock_stdout.getvalue(), "test message\n")

    def test_trace_id_injection(self):
        """Verify trace_id hydration logic."""
        params = {}
        # Simulate environment
        with patch.dict(os.environ, {"_X_AMZN_TRACE_ID": "Root=1-12345-abcdef"}):
             # Force reload/set since sitecustomize caches or reads env
             pass
        
        # Manually set the trace context as the hook reads from it
        sitecustomize._trace_context.current_trace_id = "Root=1-12345-abcdef"

        sitecustomize._inject_client_context_hook(params)

        self.assertIn("ClientContext", params)
        import base64
        ctx = json.loads(base64.b64decode(params["ClientContext"]))
        self.assertEqual(ctx["custom"]["trace_id"], "Root=1-12345-abcdef")

if __name__ == "__main__":
    unittest.main()
