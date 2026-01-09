// Where: cli/internal/app/env.go
// What: Environment management commands.
// Why: Provide env list/create/use/remove for generator.yml and global config.
package app

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

type EnvCmd struct {
	List   EnvListCmd   `cmd:"" help:"List environments"`
	Create EnvCreateCmd `cmd:"" help:"Create environment"`
	Use    EnvUseCmd    `cmd:"" help:"Switch environment"`
	Remove EnvRemoveCmd `cmd:"" help:"Remove environment"`
}

type (
	EnvListCmd   struct{}
	EnvCreateCmd struct {
		Name string `arg:"" help:"Environment name"`
	}
	EnvUseCmd struct {
		Name string `arg:"" help:"Environment name"`
	}
	EnvRemoveCmd struct {
		Name string `arg:"" help:"Environment name"`
	}
)

type envContext struct {
	Project    projectConfig
	Config     config.GlobalConfig
	ConfigPath string
}

func runEnvList(_ CLI, deps Dependencies, out io.Writer) int {
	ctx, err := resolveEnvContext(deps)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	activeEnv := strings.TrimSpace(ctx.Config.ActiveEnvironments[ctx.Project.Name])
	for _, env := range ctx.Project.Generator.Environments {
		name := strings.TrimSpace(env.Name)
		if name == "" {
			continue
		}
		if name == activeEnv {
			fmt.Fprintf(out, "* %s\n", name)
			continue
		}
		fmt.Fprintln(out, name)
	}
	return 0
}

func runEnvCreate(cli CLI, deps Dependencies, out io.Writer) int {
	rawName := strings.TrimSpace(cli.Env.Create.Name)
	if rawName == "" {
		fmt.Fprintln(out, "environment name is required")
		return 1
	}

	ctx, err := resolveEnvContext(deps)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	name, mode := splitEnvMode(rawName)
	if name == "" {
		fmt.Fprintln(out, "environment name is required")
		return 1
	}
	if mode == "" {
		mode = defaultMode()
	}
	if ctx.Project.Generator.Environments.Has(name) {
		fmt.Fprintln(out, "environment already exists")
		return 1
	}

	ctx.Project.Generator.Environments = append(ctx.Project.Generator.Environments, config.EnvironmentSpec{
		Name: name,
		Mode: mode,
	})
	if err := config.SaveGeneratorConfig(ctx.Project.GeneratorPath, ctx.Project.Generator); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintf(out, "Created environment '%s'\n", name)
	return 0
}

func runEnvUse(cli CLI, deps Dependencies, out io.Writer) int {
	name := strings.TrimSpace(cli.Env.Use.Name)
	if name == "" {
		fmt.Fprintln(out, "environment name is required")
		return 1
	}

	ctx, err := resolveEnvContext(deps)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if !ctx.Project.Generator.Environments.Has(name) {
		fmt.Fprintln(out, "environment not found")
		return 1
	}
	if ctx.ConfigPath == "" {
		fmt.Fprintln(out, "global config path not available")
		return 1
	}

	cfg := normalizeGlobalConfig(ctx.Config)
	cfg.ActiveProject = ctx.Project.Name
	cfg.ActiveEnvironments[ctx.Project.Name] = name
	cfg.Projects[ctx.Project.Name] = config.ProjectEntry{
		Path:     ctx.Project.Dir,
		LastUsed: now(deps).Format(time.RFC3339),
	}
	if err := saveGlobalConfig(ctx.ConfigPath, cfg); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintf(out, "Switched to '%s:%s'\n", ctx.Project.Name, name)
	return 0
}

func runEnvRemove(cli CLI, deps Dependencies, out io.Writer) int {
	name := strings.TrimSpace(cli.Env.Remove.Name)
	if name == "" {
		fmt.Fprintln(out, "environment name is required")
		return 1
	}

	ctx, err := resolveEnvContext(deps)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if !ctx.Project.Generator.Environments.Has(name) {
		fmt.Fprintln(out, "environment not found")
		return 1
	}
	if len(ctx.Project.Generator.Environments) <= 1 {
		fmt.Fprintln(out, "cannot remove the last environment")
		return 1
	}

	filtered := make(config.Environments, 0, len(ctx.Project.Generator.Environments)-1)
	for _, env := range ctx.Project.Generator.Environments {
		if strings.TrimSpace(env.Name) == name {
			continue
		}
		filtered = append(filtered, env)
	}
	ctx.Project.Generator.Environments = filtered
	if err := config.SaveGeneratorConfig(ctx.Project.GeneratorPath, ctx.Project.Generator); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	if ctx.ConfigPath != "" && ctx.Config.ActiveEnvironments[ctx.Project.Name] == name {
		cfg := normalizeGlobalConfig(ctx.Config)
		delete(cfg.ActiveEnvironments, ctx.Project.Name)
		if err := saveGlobalConfig(ctx.ConfigPath, cfg); err != nil {
			fmt.Fprintln(out, err)
			return 1
		}
	}

	fmt.Fprintf(out, "Removed environment '%s'\n", name)
	return 0
}

func resolveEnvContext(deps Dependencies) (envContext, error) {
	cfg := defaultGlobalConfig()
	path, err := config.GlobalConfigPath()
	if err == nil {
		loaded, err := loadGlobalConfig(path)
		if err != nil {
			return envContext{}, err
		}
		cfg = loaded
	}

	projectDir := deps.ProjectDir
	if cfg.ActiveProject != "" {
		if entry, ok := cfg.Projects[cfg.ActiveProject]; ok && strings.TrimSpace(entry.Path) != "" {
			projectDir = entry.Path
		}
	}
	project, err := loadProjectConfig(projectDir)
	if err != nil {
		return envContext{}, err
	}

	return envContext{
		Project:    project,
		Config:     cfg,
		ConfigPath: path,
	}, nil
}
