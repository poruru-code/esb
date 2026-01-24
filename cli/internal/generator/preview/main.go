package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
)

func main() {
	cfg := config.GeneratorConfig{
		App: config.AppConfig{},
		Paths: config.PathsConfig{
			SamTemplate: ".tmp/template.yml",
			OutputDir:   ".esb",
		},
		Parameters: map[string]any{},
	}
	wd, _ := os.Getwd()
	repoRoot := wd
	if filepath.Base(wd) == "cli" {
		repoRoot = filepath.Dir(wd)
	}
	opts := generator.GenerateOptions{
		ProjectRoot: repoRoot,
		DryRun:      true,
		Registry:    "",
		Tag:         "preview",
		Parameters:  map[string]string{},
	}
	functions, err := generator.GenerateFiles(cfg, opts)
	if err != nil {
		panic(err)
	}
	funcs, err := generator.RenderFunctionsYml(functions, "", "preview")
	if err != nil {
		panic(err)
	}
	routes, err := generator.RenderRoutingYml(functions)
	if err != nil {
		panic(err)
	}
	fmt.Printf("--- functions.yml ---\n%s\n--- routing.yml ---\n%s", funcs, routes)
}
