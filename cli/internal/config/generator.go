// Where: cli/internal/config/generator.go
// What: Types shared between the CLI builder and generator.
// Why: Keep the build configuration shape centralized without relying on files.
package config

// GeneratorConfig captures the data produced at build time.
type GeneratorConfig struct {
	App        AppConfig      `yaml:"app"`
	Paths      PathsConfig    `yaml:"paths"`
	Parameters map[string]any `yaml:"parameters,omitempty"`
}

// AppConfig contains the application name and optional last-used environment.
type AppConfig struct {
	Name    string `yaml:"name"`
	LastEnv string `yaml:"last_env,omitempty"`
}

// PathsConfig describes the template and output directories for the build.
type PathsConfig struct {
	SamTemplate  string `yaml:"sam_template"`
	OutputDir    string `yaml:"output_dir"`
	FunctionsYml string `yaml:"functions_yml,omitempty"`
	RoutingYml   string `yaml:"routing_yml,omitempty"`
	ResourcesYml string `yaml:"resources_yml,omitempty"`
}
