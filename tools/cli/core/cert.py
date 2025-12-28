import logging
from pathlib import Path
import socket
from datetime import datetime, timedelta, timezone

from cryptography import x509
from cryptography.x509.oid import NameOID, ExtendedKeyUsageOID
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
import ipaddress


logger = logging.getLogger(__name__)

SSL_CERT_VALIDITY_DAYS = 365
SSL_CA_VALIDITY_DAYS = 3650
SSL_KEY_SIZE = 4096


def get_local_ip() -> str:
    """Get the local IP address."""
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        return ip
    except Exception:
        return "127.0.0.1"


def generate_root_ca(cert_dir: Path):
    """Generate a Root CA."""
    ca_key_file = cert_dir / "rootCA.key"
    ca_cert_file = cert_dir / "rootCA.crt"

    if ca_key_file.exists() and ca_cert_file.exists():
        logger.debug("Using existing Root CA")
        return ca_cert_file, ca_key_file

    logger.info("Generating Private Root CA...")

    # Generate the Root CA private key.
    ca_key = rsa.generate_private_key(
        public_exponent=65537,
        key_size=SSL_KEY_SIZE,
    )

    # Generate the Root CA certificate.
    subject = issuer = x509.Name(
        [
            x509.NameAttribute(NameOID.COUNTRY_NAME, "JP"),
            x509.NameAttribute(NameOID.STATE_OR_PROVINCE_NAME, "Tokyo"),
            x509.NameAttribute(NameOID.LOCALITY_NAME, "Minato"),
            x509.NameAttribute(NameOID.ORGANIZATION_NAME, "Edge Serverless Box"),
            x509.NameAttribute(NameOID.COMMON_NAME, "ESB Root CA"),
        ]
    )

    ca_cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(ca_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.now(timezone.utc))
        .not_valid_after(datetime.now(timezone.utc) + timedelta(days=SSL_CA_VALIDITY_DAYS))
        .add_extension(
            x509.BasicConstraints(ca=True, path_length=None),
            critical=True,
        )
        .sign(ca_key, hashes.SHA256())
    )

    cert_dir.mkdir(parents=True, exist_ok=True)

    with open(ca_key_file, "wb") as f:
        f.write(
            ca_key.private_bytes(
                encoding=serialization.Encoding.PEM,
                format=serialization.PrivateFormat.TraditionalOpenSSL,
                encryption_algorithm=serialization.NoEncryption(),
            )
        )

    with open(ca_cert_file, "wb") as f:
        f.write(ca_cert.public_bytes(serialization.Encoding.PEM))

    logger.info(f"Root CA saved to: {ca_cert_file}")
    return ca_cert_file, ca_key_file


def generate_server_cert(cert_dir: Path, ca_key_path: Path, ca_cert_path: Path):
    """Generate a CA-signed server certificate."""
    server_key_file = cert_dir / "server.key"
    server_cert_file = cert_dir / "server.crt"

    # Idempotency check: skip if server cert/key exists and is newer than the CA cert.
    if server_key_file.exists() and server_cert_file.exists():
        server_cert_mtime = server_cert_file.stat().st_mtime
        ca_cert_mtime = ca_cert_path.stat().st_mtime
        if server_cert_mtime > ca_cert_mtime:
            logger.debug("Using existing server certificate")
            return server_cert_file, server_key_file

    logger.info("Generating Server Certificate signed by Private CA...")

    # Load the CA key and certificate.
    with open(ca_key_path, "rb") as f:
        ca_key = serialization.load_pem_private_key(f.read(), password=None)
    with open(ca_cert_path, "rb") as f:
        ca_cert = x509.load_pem_x509_certificate(f.read())

    # Generate the server private key.
    server_key = rsa.generate_private_key(
        public_exponent=65537,
        key_size=SSL_KEY_SIZE,
    )

    # Build the SAN (Subject Alternative Name) list.
    hostname = socket.gethostname()
    local_ip = get_local_ip()

    san_list = [
        x509.DNSName("localhost"),
        x509.DNSName(hostname),
        x509.DNSName("esb-registry"),
        x509.DNSName("esb-gateway"),
        x509.DNSName("host.docker.internal"),
        x509.IPAddress(ipaddress.IPv4Address("127.0.0.1")),
    ]

    if local_ip != "127.0.0.1":
        try:
            san_list.append(x509.IPAddress(ipaddress.IPv4Address(local_ip)))
        except ValueError:
            pass

    # Build the certificate.
    subject = x509.Name(
        [
            x509.NameAttribute(NameOID.COUNTRY_NAME, "JP"),
            x509.NameAttribute(NameOID.STATE_OR_PROVINCE_NAME, "Tokyo"),
            x509.NameAttribute(NameOID.LOCALITY_NAME, "Minato"),
            x509.NameAttribute(NameOID.ORGANIZATION_NAME, "Edge Serverless Box"),
            x509.NameAttribute(NameOID.COMMON_NAME, "localhost"),
        ]
    )

    builder = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(ca_cert.subject)
        .public_key(server_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.now(timezone.utc))
        .not_valid_after(datetime.now(timezone.utc) + timedelta(days=SSL_CERT_VALIDITY_DAYS))
        .add_extension(
            x509.SubjectAlternativeName(san_list),
            critical=False,
        )
        .add_extension(
            x509.BasicConstraints(ca=False, path_length=None),
            critical=True,
        )
        .add_extension(
            x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]),
            critical=False,
        )
    )

    cert = builder.sign(ca_key, hashes.SHA256())

    with open(server_key_file, "wb") as f:
        f.write(
            server_key.private_bytes(
                encoding=serialization.Encoding.PEM,
                format=serialization.PrivateFormat.TraditionalOpenSSL,
                encryption_algorithm=serialization.NoEncryption(),
            )
        )

    with open(server_cert_file, "wb") as f:
        f.write(cert.public_bytes(serialization.Encoding.PEM))

    logger.info(f"Server certificate saved to: {server_cert_file}")
    return server_cert_file, server_key_file


def ensure_certs(cert_dir: Path = None):
    """Prepare the full certificate set."""
    from tools.cli.config import DEFAULT_CERT_DIR

    if cert_dir is None:
        cert_dir = DEFAULT_CERT_DIR

    ca_cert, ca_key = generate_root_ca(cert_dir)
    generate_server_cert(cert_dir, ca_key, ca_cert)
