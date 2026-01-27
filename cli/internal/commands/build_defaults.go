// Where: cli/internal/commands/build_defaults.go
// What: Load/store build defaults from global config.
// Why: Reuse recent build inputs per template for interactive defaults.
package commands

import "github.com/poruru/edge-serverless-box/cli/internal/config"

type storedBuildDefaults struct {
	Env       string
	Mode      string
	OutputDir string
	Params    map[string]string
}

func loadBuildDefaults(templatePath string) storedBuildDefaults {
	cfgPath, err := config.GlobalConfigPath()
	if err != nil || templatePath == "" {
		return storedBuildDefaults{}
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		return storedBuildDefaults{}
	}
	if cfg.BuildDefaults == nil {
		return storedBuildDefaults{}
	}
	entry, ok := cfg.BuildDefaults[templatePath]
	if !ok {
		return storedBuildDefaults{}
	}
	return storedBuildDefaults{
		Env:       entry.Env,
		Mode:      entry.Mode,
		OutputDir: entry.OutputDir,
		Params:    cloneParams(entry.Params),
	}
}

func saveBuildDefaults(templatePath string, inputs buildInputs) error {
	if templatePath == "" {
		return nil
	}
	cfgPath, err := config.GlobalConfigPath()
	if err != nil {
		return err
	}
	cfg, err := config.LoadGlobalConfig(cfgPath)
	if err != nil {
		cfg = config.DefaultGlobalConfig()
	}
	if cfg.BuildDefaults == nil {
		cfg.BuildDefaults = map[string]config.BuildDefaults{}
	}
	cfg.BuildDefaults[templatePath] = config.BuildDefaults{
		Env:       inputs.Env,
		Mode:      inputs.Mode,
		OutputDir: inputs.OutputDir,
		Params:    cloneParams(inputs.Parameters),
	}
	return config.SaveGlobalConfig(cfgPath, cfg)
}

func cloneParams(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
