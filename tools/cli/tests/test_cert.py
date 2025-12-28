import pytest
from cryptography import x509
from tools.cli.core import cert


class TestCertGeneration:
    @pytest.fixture
    def cert_dir(self, tmp_path):
        """Mock directory for certificates"""
        return tmp_path / "certs"

    def test_ensure_certs_creates_all_files(self, cert_dir):
        """Test that the Root CA and server certificates are generated."""
        cert.ensure_certs(cert_dir)

        assert (cert_dir / "rootCA.crt").exists()
        assert (cert_dir / "rootCA.key").exists()
        assert (cert_dir / "server.crt").exists()
        assert (cert_dir / "server.key").exists()

        # Verify certificate contents.
        with open(cert_dir / "server.crt", "rb") as f:
            server_cert = x509.load_pem_x509_certificate(f.read())
        with open(cert_dir / "rootCA.crt", "rb") as f:
            ca_cert = x509.load_pem_x509_certificate(f.read())

        # Verify Root CA name.
        assert "ESB Root CA" in str(ca_cert.subject)

        # Verify issuer relationship.
        assert server_cert.issuer == ca_cert.subject

        # Verify SAN.
        san = server_cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
        dns_names = san.get_values_for_type(x509.DNSName)
        assert "localhost" in dns_names
        assert "esb-registry" in dns_names

    def test_ensure_certs_skips_ca_if_exists(self, cert_dir):
        """Test that an existing Root CA is not regenerated."""
        cert_dir.mkdir(parents=True)
        ca_cert_path, ca_key_path = cert.generate_root_ca(cert_dir)

        import os

        orig_mtime = os.path.getmtime(ca_cert_path)

        # Wait briefly before rerunning.
        import time

        time.sleep(0.01)

        cert.ensure_certs(cert_dir)

        # Unchanged mtime means it was not regenerated.
        assert os.path.getmtime(ca_cert_path) == orig_mtime

    def test_ensure_certs_skips_server_cert_if_newer(self, cert_dir):
        """Test that a newer server cert is not regenerated."""
        cert.ensure_certs(cert_dir)
        server_cert_path = cert_dir / "server.crt"

        import os

        orig_mtime = os.path.getmtime(server_cert_path)

        import time

        time.sleep(0.01)

        cert.ensure_certs(cert_dir)
        assert os.path.getmtime(server_cert_path) == orig_mtime
