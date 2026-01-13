// Where: cli/internal/app/config_cmd.go
// What: Configuration management commands.
// Why: Allow setting internal CLI params like repo path.
package app

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// ConfigCmd groups configuration subcommands.
type ConfigCmd struct {
	SetRepo ConfigSetRepoCmd `cmd:"" help:"Set ESB repository path"`
}

type ConfigSetRepoCmd struct {
	Path string `arg:"" help:"Path to ESB repository root"`
}

// runConfigSetRepo updates the global configuration with the ESB repo path.
func runConfigSetRepo(cli CLI, _ Dependencies, out io.Writer) int {
	repoPath := cli.Config.SetRepo.Path
	// Resolve true root (upward search) before saving
	absPath, err := config.ResolveRepoRoot(repoPath)
	if err != nil {
		fmt.Fprintf(out, "⚠️  Warning: %v\n", err)
		// Fallback to absolute path if resolution fails (though unlikely if it's a valid repo)
		absPath, err = filepath.Abs(repoPath)
		if err != nil {
			return exitWithError(out, err)
		}
	}

	path, cfg, err := loadGlobalConfigWithPath()
	if err != nil {
		return exitWithError(out, err)
	}

	cfg.RepoPath = absPath
	if err := config.SaveGlobalConfig(path, cfg); err != nil {
		return exitWithError(out, err)
	}

	fmt.Fprintf(out, "updated repo_path: %s\n", absPath)
	return 0
}
