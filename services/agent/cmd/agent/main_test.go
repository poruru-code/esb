// Where: services/agent/cmd/agent/main_test.go
// What: Unit tests for agent main helpers (TLS options / reflection flag).
// Why: Verify security-related gating behavior without starting a server.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsReflectionEnabled(t *testing.T) {
	t.Setenv("AGENT_GRPC_REFLECTION", "")
	assert.False(t, isReflectionEnabled())

	t.Setenv("AGENT_GRPC_REFLECTION", "1")
	assert.True(t, isReflectionEnabled())
}

func TestGrpcServerOptions_Disabled(t *testing.T) {
	t.Setenv("AGENT_GRPC_TLS_ENABLED", "")

	opts, err := grpcServerOptions()
	assert.NoError(t, err)
	assert.Nil(t, opts)
}

func TestGrpcServerOptions_Enabled(t *testing.T) {
	certPath, keyPath, caPath := writeTestCerts(t)

	t.Setenv("AGENT_GRPC_TLS_ENABLED", "1")
	t.Setenv("AGENT_GRPC_CERT_PATH", certPath)
	t.Setenv("AGENT_GRPC_KEY_PATH", keyPath)
	t.Setenv("AGENT_GRPC_CA_CERT_PATH", caPath)

	opts, err := grpcServerOptions()
	assert.NoError(t, err)
	assert.Len(t, opts, 1)
}

func writeTestCerts(t *testing.T) (string, string, string) {
	t.Helper()

	dir := t.TempDir()
	caCertPath := filepath.Join(dir, "ca.crt")
	serverCertPath := filepath.Join(dir, "server.crt")
	serverKeyPath := filepath.Join(dir, "server.key")

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "esb-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	if err := os.WriteFile(caCertPath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caDER,
	}), 0o600); err != nil {
		t.Fatalf("write ca cert: %v", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "esb-test-server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}
	if err := os.WriteFile(serverCertPath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverDER,
	}), 0o600); err != nil {
		t.Fatalf("write server cert: %v", err)
	}
	if err := os.WriteFile(serverKeyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverKey),
	}), 0o600); err != nil {
		t.Fatalf("write server key: %v", err)
	}

	return serverCertPath, serverKeyPath, caCertPath
}
