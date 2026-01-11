// Where: cli/internal/app/auth.go
// What: Auto-generation of authentication credentials.
// Why: Provide secure defaults when credentials are not explicitly configured.
package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
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
	return creds
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
