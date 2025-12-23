from unittest.mock import patch
import pytest

# Module to be tested (not implemented yet)
try:
    from tools.cli.core import cert
except ImportError:
    cert = None


class TestCertGeneration:
    @pytest.fixture
    def mock_project_root(self, tmp_path):
        """Mock PROJECT_ROOT to use a temp directory"""
        with patch("tools.cli.core.cert.PROJECT_ROOT", tmp_path):
            yield tmp_path

    def test_generate_ssl_certificate_creates_files(self, mock_project_root):
        """Test that certificates are generated in the correct location"""
        if cert is None:
            pytest.fail("tools.cli.core.cert module not found")

        # Patch the logger instance used in the module
        with patch("tools.cli.core.cert.logger") as mock_logger:
            cert.generate_ssl_certificate()

            certs_dir = mock_project_root / "certs"
            assert (certs_dir / "server.crt").exists()
            assert (certs_dir / "server.key").exists()

            # Verify logging was called
            mock_logger.info.assert_called()

    def test_generate_ssl_certificate_skips_if_exists(self, mock_project_root):
        """Test that generation is skipped if files already exist"""
        if cert is None:
            pytest.fail("tools.cli.core.cert module not found")

        certs_dir = mock_project_root / "certs"
        certs_dir.mkdir()
        (certs_dir / "server.crt").touch()
        (certs_dir / "server.key").touch()

        with patch("tools.cli.core.cert.logger") as mock_logger:
            # First call (already exists)
            cert.generate_ssl_certificate()

            # Check modification time or logging to ensure it skipped
            mock_logger.debug.assert_called_with("Using existing SSL certificates")
