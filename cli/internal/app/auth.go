// Where: cli/internal/app/auth.go
// What: Auto-generation of authentication credentials.
// Why: Provide secure defaults when credentials are not explicitly configured.
package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// AuthCredentials holds the authentication configuration that was either
// loaded from environment or auto-generated.
type AuthCredentials struct {
	AuthUser        string
	AuthPass        string
	JWTSecretKey    string
	XAPIKey         string
	RustfsAccessKey string
	RustfsSecretKey string
	Generated       bool // True if any credentials were auto-generated
}

// EnsureAuthCredentials checks required authentication environment variables
// and generates random values for any that are missing. Returns the credentials
// and whether any were auto-generated.
func EnsureAuthCredentials() AuthCredentials {
	creds := AuthCredentials{}
	generated := false

	// Fixed usernames
	if os.Getenv("AUTH_USER") == "" {
		os.Setenv("AUTH_USER", "esb")
		generated = true
	}
	creds.AuthUser = os.Getenv("AUTH_USER")

	if os.Getenv("RUSTFS_ACCESS_KEY") == "" {
		os.Setenv("RUSTFS_ACCESS_KEY", "esb")
		generated = true
	}
	creds.RustfsAccessKey = os.Getenv("RUSTFS_ACCESS_KEY")

	// Random passwords (32 characters each)
	if os.Getenv("AUTH_PASS") == "" {
		pass := generateSecureRandom(32)
		os.Setenv("AUTH_PASS", pass)
		generated = true
	}
	creds.AuthPass = os.Getenv("AUTH_PASS")

	if os.Getenv("JWT_SECRET_KEY") == "" {
		key := generateSecureRandom(32)
		os.Setenv("JWT_SECRET_KEY", key)
		generated = true
	}
	creds.JWTSecretKey = os.Getenv("JWT_SECRET_KEY")

	if os.Getenv("X_API_KEY") == "" {
		key := generateSecureRandom(32)
		os.Setenv("X_API_KEY", key)
		generated = true
	}
	creds.XAPIKey = os.Getenv("X_API_KEY")

	if os.Getenv("RUSTFS_SECRET_KEY") == "" {
		key := generateSecureRandom(32)
		os.Setenv("RUSTFS_SECRET_KEY", key)
		generated = true
	}
	creds.RustfsSecretKey = os.Getenv("RUSTFS_SECRET_KEY")

	creds.Generated = generated

	// Ensure certificates exist (fixes IsADirectoryError on first run)
	if err := EnsureCertificates(); err != nil {
		fmt.Printf("Warning: Failed to generate certificates: %v\n", err)
	}

	return creds
}

// EnsureCertificates checks if the certificates exist in ~/.esb/certs
// and generates them if not. This prevents docker from mounting a directory
// when the file is missing.
func EnsureCertificates() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	certDir := filepath.Join(home, ".esb", "certs")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		return err
	}

	certPath := filepath.Join(certDir, "server.crt")
	keyPath := filepath.Join(certDir, "server.key")

	errCert := checkFile(certPath)
	errKey := checkFile(keyPath)

	if errCert == nil && errKey == nil {
		return nil // Both exist and are files
	}

	fmt.Println("Generating self-signed certificates in ~/.esb/certs...")
	// Force cleanup if they exist but are invalid (e.g. directories)
	os.RemoveAll(certPath)
	os.RemoveAll(keyPath)
	return generateSelfSignedCert(certPath, keyPath)
}

// checkFile returns nil if path exists and is a file.
func checkFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	return nil
}

func generateSelfSignedCert(certPath, keyPath string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Edge Serverless Box"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "gateway"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("0.0.0.0")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return err
	}

	keyOut, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer keyOut.Close()

	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes}); err != nil {
		return err
	}

	return nil
}

// generateSecureRandom generates a cryptographically secure random hex string
// of the specified length (in characters, not bytes).
func generateSecureRandom(length int) string {
	bytes := make([]byte, (length+1)/2)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a less secure but functional default
		return "fallback-insecure-key-please-set-env"
	}
	return hex.EncodeToString(bytes)[:length]
}

// PrintGeneratedCredentials outputs the auto-generated credentials to the writer.
// Only called when credentials were actually generated.
func PrintGeneratedCredentials(out io.Writer, creds AuthCredentials) {
	fmt.Fprintln(out, "✓ Authentication credentials auto-generated:")
	fmt.Fprintf(out, "  AUTH_USER         = %s\n", creds.AuthUser)
	fmt.Fprintf(out, "  AUTH_PASS         = %s\n", creds.AuthPass)
	fmt.Fprintf(out, "  JWT_SECRET_KEY    = %s\n", creds.JWTSecretKey)
	fmt.Fprintf(out, "  X_API_KEY         = %s\n", creds.XAPIKey)
	fmt.Fprintf(out, "  RUSTFS_ACCESS_KEY = %s\n", creds.RustfsAccessKey)
	fmt.Fprintf(out, "  RUSTFS_SECRET_KEY = %s\n", creds.RustfsSecretKey)
	fmt.Fprintln(out)
}
