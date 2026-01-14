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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

// EnsureCertificates checks if the certificates exist in the configured directory
// (ESB_CERT_DIR or ~/.esb/certs) and generates them if not.
func EnsureCertificates() error {
	var certDir string
	if envDir := os.Getenv("ESB_CERT_DIR"); envDir != "" {
		// Expand ~ if present (though shells usually handle this, env vars might not)
		if strings.HasPrefix(envDir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			certDir = filepath.Join(home, envDir[2:])
		} else {
			certDir = envDir
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		certDir = filepath.Join(home, ".esb", "certs")
	}

	if err := os.MkdirAll(certDir, 0o755); err != nil {
		return err
	}

	caCertPath := filepath.Join(certDir, "ca.crt")
	caKeyPath := filepath.Join(certDir, "ca.key")
	certPath := filepath.Join(certDir, "server.crt")
	keyPath := filepath.Join(certDir, "server.key")

	// 1. Ensure Root CA exists
	caCert, caKey, err := ensureRootCA(caCertPath, caKeyPath)
	if err != nil {
		return fmt.Errorf("failed to ensure Root CA: %w", err)
	}

	// 2. Ensure Server Certificate exists (signed by Root CA)
	if err := checkFile(certPath); err == nil {
		if err := checkFile(keyPath); err == nil {
			// Idempotency check matching Python: server cert must be newer than CA
			certInfo, _ := os.Stat(certPath)
			caInfo, _ := os.Stat(caCertPath)
			if certInfo.ModTime().After(caInfo.ModTime()) {
				// TODO: We could add SAN verification here like legacy code, but for now date check + existence is a good proxy
				return nil
			}
			fmt.Println("Existing server certificate is older than Root CA; regenerating.")
		}
	}

	fmt.Println("Generating server certificate signed by ESB Root CA in " + certDir + "...")
	// Force cleanup
	os.RemoveAll(certPath)
	os.RemoveAll(keyPath)

	return generateServerCert(certPath, keyPath, caCert, caKey)
}

// ensureRootCA loads or generates the Root CA.
func ensureRootCA(certPath, keyPath string) (*x509.Certificate, any, error) {
	// Try loading first
	if checkFile(certPath) == nil && checkFile(keyPath) == nil {
		certPEM, err := os.ReadFile(certPath)
		if err != nil {
			return nil, nil, err
		}
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, nil, err
		}

		block, _ := pem.Decode(certPEM)
		if block == nil {
			return nil, nil, fmt.Errorf("failed to decode CA cert PEM")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, err
		}

		keyBlock, _ := pem.Decode(keyPEM)
		if keyBlock == nil {
			return nil, nil, fmt.Errorf("failed to decode CA key PEM")
		}
		// Try PKCS8 first, then PKCS1
		key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err != nil {
			key, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse CA private key: %w", err)
			}
		}
		return cert, key, nil
	}

	fmt.Println("Generating ESB Root CA...")
	return generateRootCA(certPath, keyPath)
}

func generateRootCA(certPath, keyPath string) (*x509.Certificate, any, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 4096) // Match Python SSL_KEY_SIZE = 4096
	if err != nil {
		return nil, nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(3650 * 24 * time.Hour) // Match Python SSL_CA_VALIDITY_DAYS = 3650

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Country:      []string{"JP"},
			Province:     []string{"Tokyo"},
			Locality:     []string{"Minato"},
			Organization: []string{"Edge Serverless Box"},
			CommonName:   "ESB Root CA",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	if err := writePEM(certPath, "CERTIFICATE", derBytes); err != nil {
		return nil, nil, err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	if err := writePEM(keyPath, "PRIVATE KEY", privBytes); err != nil {
		return nil, nil, err
	}

	// Try to install Root CA to system trust store
	if err := installRootCA(certPath); err != nil {
		fmt.Printf("WARNING: Failed to install Root CA to system trust store: %v\n", err)
		fmt.Println("You may need to install it manually to prevent SSL errors.")
	}

	return &template, priv, nil
}

func installRootCA(certPath string) error {
	switch runtime.GOOS {
	case "windows":
		// check if already installed
		checkCmd := exec.Command("certutil", "-verifystore", "Root", "ESB Root CA")
		if err := checkCmd.Run(); err == nil {
			fmt.Println("ESB Root CA is already trusted.")
			return nil
		}
		// install
		fmt.Println("Installing Root CA to Windows trust store (requires Admin)...")
		cmd := exec.Command("certutil", "-addstore", "-f", "Root", certPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("certutil failed: %s, output: %s", err, string(output))
		}

	case "darwin":
		// check if already installed
		checkCmd := exec.Command("security", "find-certificate", "-c", "ESB Root CA")
		if err := checkCmd.Run(); err == nil {
			fmt.Println("ESB Root CA is already trusted.")
			return nil
		}
		fmt.Println("Installing Root CA to macOS Keychain (requires sudo)...")
		cmd := exec.Command("sudo", "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", certPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()

	case "linux":
		destDir := "/usr/local/share/ca-certificates"
		if _, err := os.Stat(destDir); os.IsNotExist(err) {
			return fmt.Errorf("certificate directory %s does not exist", destDir)
		}
		destPath := filepath.Join(destDir, "esb-rootCA.crt")
		fmt.Println("Installing Root CA to Linux trust store (requires sudo)...")

		cmdCp := exec.Command("sudo", "cp", certPath, destPath)
		cmdCp.Stdin = os.Stdin
		cmdCp.Stdout = os.Stdout
		cmdCp.Stderr = os.Stderr
		if err := cmdCp.Run(); err != nil {
			return fmt.Errorf("failed to copy cert: %w", err)
		}

		cmdUpdate := exec.Command("sudo", "update-ca-certificates")
		cmdUpdate.Stdin = os.Stdin
		cmdUpdate.Stdout = os.Stdout
		cmdUpdate.Stderr = os.Stderr
		return cmdUpdate.Run()

	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return nil
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func generateServerCert(certPath, keyPath string, caCert *x509.Certificate, caKey any) error {
	priv, err := rsa.GenerateKey(rand.Reader, 4096) // Match Python SSL_KEY_SIZE = 4096
	if err != nil {
		return err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // Match Python SSL_CERT_VALIDITY_DAYS = 365

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	hostname, _ := os.Hostname()
	localIP := getLocalIP()
	wgGatewayIP := net.ParseIP("10.99.0.1") // Match Python WG_GATEWAY_IP

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Country:      []string{"JP"},
			Province:     []string{"Tokyo"},
			Locality:     []string{"Minato"},
			Organization: []string{"Edge Serverless Box"},
			CommonName:   "localhost", // Match Python logic
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames: []string{
			"localhost",
			hostname,
			"registry", // For containerd mode
			"gateway",  // For containerd mode
			"host.docker.internal",
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			wgGatewayIP,
		},
	}

	if localIP != "127.0.0.1" {
		if ip := net.ParseIP(localIP); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return err
	}

	if err := writePEM(certPath, "CERTIFICATE", derBytes); err != nil {
		return err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	if err := writePEM(keyPath, "PRIVATE KEY", privBytes); err != nil {
		return err
	}

	return nil
}

func writePEM(path, blockType string, bytes []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: bytes})
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
