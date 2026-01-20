// Where: cli/internal/commands/config_cmd.go
// What: Configuration management commands.
// Why: Allow setting internal CLI params like repo path.
package commands

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// ConfigCmd groups configuration subcommands.
type ConfigCmd struct {
	SetRepo ConfigSetRepoCmd `cmd:"" help:"Set repository path"`
}

type ConfigSetRepoCmd struct {
	Path string `arg:"" help:"Path to repository root"`
}

// runConfigSetRepo updates the global configuration with the repo path.
func runConfigSetRepo(cli CLI, deps Dependencies, out io.Writer) int {
	repoPath := cli.Config.SetRepo.Path
	ui := legacyUI(out)
	// Resolve true root (upward search) before saving
	absPath, err := config.ResolveRepoRootFromPath(repoPath)
	if err != nil {
		ui.Warn(fmt.Sprintf("⚠️  Warning: %v", err))
		// Fallback to absolute path if resolution fails (though unlikely if it's a valid repo)
		absPath, err = filepath.Abs(repoPath)
		if err != nil {
			return exitWithError(out, err)
		}
	}

	path, cfg, err := loadGlobalConfigWithPath(deps)
	if err != nil {
		return exitWithError(out, err)
	}

	cfg.RepoPath = absPath
	if err := config.SaveGlobalConfig(path, cfg); err != nil {
		return exitWithError(out, err)
	}

	ui.Info(fmt.Sprintf("updated repo_path: %s", absPath))
	return 0
}
