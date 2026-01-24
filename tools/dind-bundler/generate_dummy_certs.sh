#!/bin/bash
set -e

CERT_DIR="$HOME/.esb/certs"
mkdir -p "$CERT_DIR"

if [ -f "$CERT_DIR/rootCA.crt" ]; then
    echo "Certs already exist."
    exit 0
fi

echo "Generating dummy certificates in $CERT_DIR..."

# Root CA
# Check if rootCA.key exists, otherwise generate
if [ ! -f "$CERT_DIR/rootCA.key" ]; then
    openssl req -x509 -new -nodes -keyout "$CERT_DIR/rootCA.key" -sha256 -days 3650 -out "$CERT_DIR/rootCA.crt" -subj "/CN=ESB-Root-CA"
    cp "$CERT_DIR/rootCA.crt" "$CERT_DIR/rootCA.pem"
fi

# Server Cert
openssl req -new -nodes -newkey rsa:2048 -keyout "$CERT_DIR/server.key" -out "$CERT_DIR/server.csr" -subj "/CN=localhost"
openssl x509 -req -in "$CERT_DIR/server.csr" -CA "$CERT_DIR/rootCA.crt" -CAkey "$CERT_DIR/rootCA.key" -CAcreateserial -out "$CERT_DIR/server.crt" -days 3650 -sha256

# Client Cert
openssl req -new -nodes -newkey rsa:2048 -keyout "$CERT_DIR/client.key" -out "$CERT_DIR/client.csr" -subj "/CN=client"
openssl x509 -req -in "$CERT_DIR/client.csr" -CA "$CERT_DIR/rootCA.crt" -CAkey "$CERT_DIR/rootCA.key" -CAcreateserial -out "$CERT_DIR/client.crt" -days 3650 -sha256

# Cleanup
rm -f "$CERT_DIR"/*.csr "$CERT_DIR"/*.srl

echo "Certificates generated."
