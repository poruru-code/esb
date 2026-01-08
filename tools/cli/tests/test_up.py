import os
from unittest.mock import MagicMock, patch

import pytest
import yaml

from tools.cli import config as cli_config
from tools.cli.commands import up


class TestUpCommand:
    @pytest.fixture(autouse=True)
    def setup_env(self):
        # Mocking update of Os environ that enforce_env_arg usually does
        cli_config.setup_environment("default")
        with patch("tools.cli.commands.up.cli_config.TEMPLATE_YAML", "dummy_template.yaml"):
            yield
        # Teardown defined by yield, but os.environ persists? 
        # Pytest usually handles env if using monkeypatch, but here direct modification.
        # Ideally use monkeypatch/context, but setup_environment modifies global os.environ.
        # It's fine for these unit tests.

    @pytest.fixture
    def mock_args(self):
        args = MagicMock()
        args.detach = True
        args.build = False
        args.wait = False
        # Fix: missing string attributes causing TypeError
        args.env = "default"
        args.file = []
        return args

    @patch("tools.cli.commands.up.subprocess.check_call")
    @patch("tools.cli.commands.up.provisioner.main")
    @patch("tools.cli.commands.up.ensure_certs")
    @patch("tools.cli.commands.up.context.enforce_env_arg")
    def test_run_generates_certificates(self, mock_enforce, mock_cert, mock_prov, mock_sub, mock_args):
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
    @patch("tools.cli.commands.up.ensure_certs")  # Expecting import in up.py
    @patch("tools.cli.commands.up.cli_config.get_port_mapping", return_value={"ESB_PORT_GATEWAY_HTTPS": "443"})
    @patch("requests.get")
    @patch("tools.cli.commands.up.context.enforce_env_arg")
    def test_run_waits_for_gateway_if_wait_arg_is_true(
        self, mock_enforce, mock_get, mock_port_mapping, mock_cert, mock_prov, mock_sub, mock_args
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
        assert "/health" in args[0]

    @patch("tools.cli.commands.up.subprocess.check_call")
    @patch("tools.cli.commands.up.provisioner.main")
    @patch("tools.cli.commands.up.ensure_certs")
    @patch("tools.cli.commands.up.context.enforce_env_arg")
    def test_run_sets_agent_address_for_firecracker(
        self, mock_enforce, mock_cert, mock_prov, mock_sub, mock_args, tmp_path, monkeypatch
    ):
        nodes = {"version": 1, "nodes": [{"wg_compute_addr": "10.99.0.2/32"}]}
        nodes_path = tmp_path / "nodes.yaml"
        nodes_path.write_text(yaml.safe_dump(nodes))

        monkeypatch.setenv("ESB_HOME", str(tmp_path))
        monkeypatch.setenv("AGENT_GRPC_ADDRESS", "localhost:50051")
        monkeypatch.setenv("PORT", "50051")
        monkeypatch.setattr(up.runtime_mode, "get_mode", lambda: up.cli_config.ESB_MODE_FIRECRACKER)

        up.run(mock_args)

        assert os.environ["AGENT_GRPC_ADDRESS"] == "10.99.0.2:50051"
