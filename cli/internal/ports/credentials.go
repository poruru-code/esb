// Where: cli/internal/ports/credentials.go
// What: Credential manager port definitions.
// Why: Allow workflows to ensure authentication credentials without knowing implementation details.
package ports

// AuthCredentials holds authentication secrets required by the environment.
type AuthCredentials struct {
	AuthUser        string
	AuthPass        string
	JWTSecretKey    string
	XAPIKey         string
	RustfsAccessKey string
	RustfsSecretKey string
	Generated       bool
}

// CredentialManager ensures authentication credentials are present.
type CredentialManager interface {
	Ensure() AuthCredentials
}
