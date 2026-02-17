// Where: cli/internal/command/branding.go
// What: User-facing CLI command name.
// Why: Keep the CLI binary name contract stable across environments.
package command

const fixedCLIName = "esb"

func cliName() string {
	return fixedCLIName
}
