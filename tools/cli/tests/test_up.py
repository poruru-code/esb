from unittest.mock import MagicMock, patch
import pytest
from tools.cli.commands import up


class TestUpCommand:
    @pytest.fixture
    def mock_args(self):
        args = MagicMock()
        args.detach = True
        args.build = False
        args.wait = False
        return args

    @patch("tools.cli.commands.up.subprocess.check_call")
    @patch("tools.cli.commands.up.provisioner.main")
    @patch("tools.cli.commands.up.generate_ssl_certificate")
    def test_run_generates_certificates(self, mock_cert, mock_prov, mock_sub, mock_args):
        """Test that SSL certificates are generated before bringing up containers"""
        # Mocking generate_ssl_certificate locally in the up module context
        # Needs to be implemented in up.py first to be patched, or we can patch where it would be imported
        # But since it's TDD, we expect this to fail if we try to patch something that isn't imported yet.
        # Alternatively, we patch the source: tools.cli.core.cert.generate_ssl_certificate
        # which is what up.py SHOULD import.

        # However, for the test to even run without ImportErrors, up.py imports must succeed.
        # test_up.py imports up, so up.py must be importable.

        up.run(mock_args)

        # Expectation: generate_ssl_certificate is called
        mock_cert.assert_called_once()

    @patch("tools.cli.commands.up.subprocess.check_call")
    @patch("tools.cli.commands.up.provisioner.main")
    @patch("tools.cli.commands.up.generate_ssl_certificate")  # Expecting import in up.py
    @patch("requests.get")
    def test_run_waits_for_gateway_if_wait_arg_is_true(
        self, mock_get, mock_cert, mock_prov, mock_sub, mock_args
    ):
        """Test that --wait triggers health check logic"""
        mock_args.wait = True

        # Mock requests.get success
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_get.return_value = mock_response

        up.run(mock_args)

        # Expectation: requests.get is called (waiting logic)
        mock_get.assert_called()
        args, kwargs = mock_get.call_args
        assert (
            "https://localhost/health" in args[0] or kwargs.get("url") == "https://localhost/health"
        )
