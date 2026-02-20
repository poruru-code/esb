// Where: cli/internal/infra/templategen/git_helpers.go
// What: Minimal git command helper for templategen trace metadata.
// Why: Avoid dependency on infra/build internals while keeping errors explicit.
package templategen

import (
	"context"
	"fmt"
	"strings"

	"github.com/poruru-code/esb/cli/internal/infra/compose"
)

func runGit(ctx context.Context, runner compose.CommandRunner, root string, args ...string) (string, error) {
	out, err := runner.RunOutput(ctx, root, "git", args...)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, msg)
	}
	val := strings.TrimSpace(string(out))
	if val == "" {
		return "", fmt.Errorf("git %s returned empty output", strings.Join(args, " "))
	}
	return val, nil
}
