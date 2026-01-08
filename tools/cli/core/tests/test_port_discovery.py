"""
Where: tools/cli/core/tests/test_port_discovery.py
What: Unit tests for dynamic port discovery module
Why: TDD - Tests first for port discovery functionality
"""
import json
import os
import subprocess
from unittest.mock import patch


class TestDiscoverPorts:
    """Tests for discover_ports function"""

    def test_discover_ports_returns_dict(self):
        """discover_ports should return a dictionary"""
        from tools.cli.core.port_discovery import discover_ports

        with patch("subprocess.check_output", return_value=""):
            result = discover_ports("test-project", ["/path/to/compose.yml"])
        
        assert isinstance(result, dict)

    def test_discover_ports_parses_docker_output(self):
        """discover_ports should parse 'host:port' format from docker compose port"""
        from tools.cli.core.port_discovery import discover_ports

        def mock_check_output(cmd, text=True, stderr=None):
            # Simulate docker compose port output
            if "gateway" in cmd and "443" in cmd:
                return "0.0.0.0:32768\n"
            elif "s3-storage" in cmd and "9000" in cmd:
                return "0.0.0.0:32769\n"
            return ""

        with patch("subprocess.check_output", side_effect=mock_check_output):
            result = discover_ports("test-project", ["/path/to/compose.yml"], mode="docker")
        
        assert result.get("ESB_PORT_GATEWAY_HTTPS") == 32768
        assert result.get("ESB_PORT_STORAGE") == 32769

    def test_discover_ports_handles_ipv6_format(self):
        """discover_ports should parse '[::]:port' format"""
        from tools.cli.core.port_discovery import discover_ports

        with patch("subprocess.check_output", return_value="[::]:32770\n"):
            result = discover_ports("test-project", ["/path/to/compose.yml"], mode="docker")
        
        assert result.get("ESB_PORT_GATEWAY_HTTPS") == 32770

    def test_discover_ports_skips_failed_services(self):
        """discover_ports should skip services that fail to return port"""
        from tools.cli.core.port_discovery import discover_ports

        def mock_check_output(cmd, text=True, stderr=None):
            if "gateway" in cmd:
                raise subprocess.CalledProcessError(1, cmd)
            return "0.0.0.0:32768\n"

        with patch("subprocess.check_output", side_effect=mock_check_output):
            # Test with docker mode where gateway is the service for GATEWAY_HTTPS
            result = discover_ports("test-project", ["/path/to/compose.yml"], mode="docker")
        
        assert "ESB_PORT_GATEWAY_HTTPS" not in result

    def test_discover_ports_respects_mode_filter(self):
        """discover_ports should skip registry/agent ports in docker mode"""
        from tools.cli.core.port_discovery import discover_ports

        call_log = []
        def mock_check_output(cmd, text=True, stderr=None):
            call_log.append(cmd)
            return "0.0.0.0:32768\n"

        with patch("subprocess.check_output", side_effect=mock_check_output):
            discover_ports("test-project", ["/path/to/compose.yml"], mode="docker")
        
        # registry and runtime-node should not be queried in docker mode
        call_strs = [" ".join(c) for c in call_log]
        assert not any("registry" in s for s in call_strs)
        assert not any("runtime-node" in s for s in call_strs)


class TestSavePorts:
    """Tests for save_ports function"""

    def test_save_ports_creates_json_file(self, tmp_path, monkeypatch):
        """save_ports should create a JSON file with port mapping"""
        from tools.cli.core.port_discovery import save_ports

        # Override home directory
        monkeypatch.setenv("HOME", str(tmp_path))
        
        ports = {"ESB_PORT_GATEWAY_HTTPS": 32768, "ESB_PORT_STORAGE": 32769}
        result_path = save_ports("test-env", ports)
        
        assert result_path.exists()
        content = json.loads(result_path.read_text())
        assert content == ports

    def test_save_ports_creates_parent_directories(self, tmp_path, monkeypatch):
        """save_ports should create ~/.esb/{env}/ if not exists"""
        from tools.cli.core.port_discovery import save_ports

        monkeypatch.setenv("HOME", str(tmp_path))
        
        save_ports("new-env", {"ESB_PORT_GATEWAY_HTTPS": 12345})
        
        assert (tmp_path / ".esb" / "new-env" / "ports.json").exists()


class TestLoadPorts:
    """Tests for load_ports function"""

    def test_load_ports_reads_json_file(self, tmp_path, monkeypatch):
        """load_ports should read and return port mapping from file"""
        from tools.cli.core.port_discovery import load_ports

        monkeypatch.setenv("HOME", str(tmp_path))
        
        # Create the file
        port_file = tmp_path / ".esb" / "test-env" / "ports.json"
        port_file.parent.mkdir(parents=True)
        port_file.write_text('{"ESB_PORT_GATEWAY_HTTPS": 32768}')
        
        result = load_ports("test-env")
        
        assert result == {"ESB_PORT_GATEWAY_HTTPS": 32768}

    def test_load_ports_returns_empty_if_no_file(self, tmp_path, monkeypatch):
        """load_ports should return empty dict if file doesn't exist"""
        from tools.cli.core.port_discovery import load_ports

        monkeypatch.setenv("HOME", str(tmp_path))
        
        result = load_ports("nonexistent-env")
        
        assert result == {}


class TestApplyPortsToEnv:
    """Tests for apply_ports_to_env function"""

    def test_apply_ports_sets_env_vars(self, monkeypatch):
        """apply_ports_to_env should set environment variables"""
        from tools.cli.core.port_discovery import apply_ports_to_env

        # Clear any existing vars
        monkeypatch.delenv("ESB_PORT_GATEWAY_HTTPS", raising=False)
        
        apply_ports_to_env({"ESB_PORT_GATEWAY_HTTPS": 32768})
        
        assert os.environ.get("ESB_PORT_GATEWAY_HTTPS") == "32768"

    def test_apply_ports_sets_derived_gateway_vars(self, monkeypatch):
        """apply_ports_to_env should set GATEWAY_PORT and GATEWAY_URL"""
        from tools.cli.core.port_discovery import apply_ports_to_env

        monkeypatch.delenv("GATEWAY_PORT", raising=False)
        monkeypatch.delenv("GATEWAY_URL", raising=False)
        
        apply_ports_to_env({"ESB_PORT_GATEWAY_HTTPS": 32768})
        
        assert os.environ.get("GATEWAY_PORT") == "32768"
        assert os.environ.get("GATEWAY_URL") == "https://localhost:32768"

    def test_apply_ports_sets_derived_victorialogs_vars(self, monkeypatch):
        """apply_ports_to_env should set VICTORIALOGS_* vars"""
        from tools.cli.core.port_discovery import apply_ports_to_env

        monkeypatch.delenv("VICTORIALOGS_PORT", raising=False)
        
        apply_ports_to_env({"ESB_PORT_VICTORIALOGS": 32769})
        
        assert os.environ.get("VICTORIALOGS_PORT") == "32769"
        assert os.environ.get("VICTORIALOGS_URL") == "http://localhost:32769"

    def test_apply_ports_sets_agent_grpc_address(self, monkeypatch):
        """apply_ports_to_env should set AGENT_GRPC_ADDRESS"""
        from tools.cli.core.port_discovery import apply_ports_to_env

        monkeypatch.delenv("AGENT_GRPC_ADDRESS", raising=False)
        
        apply_ports_to_env({"ESB_PORT_AGENT_GRPC": 50052})
        
        assert os.environ.get("AGENT_GRPC_ADDRESS") == "localhost:50052"
