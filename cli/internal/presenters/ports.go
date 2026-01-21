// Where: cli/internal/presenters/ports.go
// What: Shared UI presenters for workflows.
// Why: Avoid circular dependencies between workflows by centralizing output logic.
package presenters

import (
	"sort"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
)

// PrintCredentials displays the authentication credentials to the user.
func PrintCredentials(ui ports.UserInterface, creds ports.AuthCredentials) {
	if ui == nil {
		return
	}
	rows := []ports.KeyValue{
		{Key: "AUTH_USER", Value: creds.AuthUser},
		{Key: "AUTH_PASS", Value: creds.AuthPass},
		{Key: "JWT_SECRET_KEY", Value: creds.JWTSecretKey},
		{Key: "X_API_KEY", Value: creds.XAPIKey},
		{Key: "RUSTFS_ACCESS_KEY", Value: creds.RustfsAccessKey},
		{Key: "RUSTFS_SECRET_KEY", Value: creds.RustfsSecretKey},
	}
	ui.Block("🔑", "Authentication credentials:", rows)
}

// PrintDiscoveredPorts displays the mapped host ports for environment services.
func PrintDiscoveredPorts(ui ports.UserInterface, portsMap map[string]int) {
	if ui == nil || len(portsMap) == 0 {
		return
	}

	var rows []ports.KeyValue
	add := func(key string) {
		if val, ok := portsMap[key]; ok {
			rows = append(rows, ports.KeyValue{Key: key, Value: val})
		}
	}

	add(constants.EnvPortGatewayHTTPS)
	add(constants.EnvPortVictoriaLogs)
	add(constants.EnvPortDatabase)
	add(constants.EnvPortS3)
	add(constants.EnvPortS3Mgmt)
	add(constants.EnvPortRegistry)
	add(constants.EnvPortAgentMetrics)

	var unknown []string
	for key := range portsMap {
		if !IsKnownPort(key) {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	for _, key := range unknown {
		rows = append(rows, ports.KeyValue{Key: key, Value: portsMap[key]})
	}

	if len(rows) == 0 {
		return
	}
	ui.Block("🔌", "Discovered Ports:", rows)
}

// PrintResetWarning displays a warning about destructive reset operations.
func PrintResetWarning(ui ports.UserInterface) {
	if ui == nil {
		return
	}
	ui.Warn("WARNING! This will remove:")
	ui.Warn("  - all containers for the selected environment")
	ui.Warn("  - all volumes for the selected environment (DB/S3 data)")
	ui.Warn("  - rebuild images and restart services")
	ui.Warn("")
}

// IsKnownPort returns true if the environment variable key represents a known ESB port.
func IsKnownPort(key string) bool {
	switch key {
	case constants.EnvPortGatewayHTTPS,
		constants.EnvPortVictoriaLogs,
		constants.EnvPortDatabase,
		constants.EnvPortS3,
		constants.EnvPortS3Mgmt,
		constants.EnvPortRegistry,
		constants.EnvPortAgentGRPC,
		constants.EnvPortAgentMetrics:
		return true
	}
	return false
}
