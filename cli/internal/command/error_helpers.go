// Where: cli/internal/command/error_helpers.go
// What: Shared CLI error env.
// Why: Keep build-only CLI output consistent without project/env commands.
package command

import (
	"fmt"
	"io"
)

// exitWithError prints an error message to the output writer and returns
// exit code 1 for CLI error handling.
func exitWithError(out io.Writer, err error) int {
	legacyUI(out).Warn(fmt.Sprintf("✗ %v", err))
	return 1
}

// exitWithSuggestion prints an error with suggested next steps.
func exitWithSuggestion(out io.Writer, message string, suggestions []string) int {
	ui := legacyUI(out)
	ui.Warn(fmt.Sprintf("⚠️  %s", message))
	if len(suggestions) > 0 {
		ui.Info("")
		ui.Info("💡 Next steps:")
		for _, s := range suggestions {
			ui.Info(fmt.Sprintf("   - %s", s))
		}
	}
	return 1
}

// exitWithSuggestionAndAvailable prints an error with suggestions and available options.
func exitWithSuggestionAndAvailable(out io.Writer, message string, suggestions, available []string) int {
	ui := legacyUI(out)
	ui.Warn(fmt.Sprintf("⚠️  %s", message))
	if len(suggestions) > 0 {
		ui.Info("")
		ui.Info("💡 Next steps:")
		for _, s := range suggestions {
			ui.Info(fmt.Sprintf("   - %s", s))
		}
	}
	if len(available) > 0 {
		ui.Info("")
		ui.Info("🛠️  Available:")
		for _, a := range available {
			ui.Info(fmt.Sprintf("   - %s", a))
		}
	}
	return 1
}
