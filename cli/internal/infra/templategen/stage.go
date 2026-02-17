// Where: cli/internal/infra/templategen/stage.go
// What: Core file staging helpers for generator output.
// Why: Keep GenerateFiles readable and testable while separating language-specific details.
package templategen

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/runtime"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
)

type stageContext struct {
	BaseDir           string
	OutputDir         string
	FunctionsDir      string
	ProjectRoot       string
	SitecustomizePath string
	LayerCacheDir     string
	DryRun            bool
	Verbose           bool
	Out               io.Writer
	JavaRuntimeBuild  *javaRuntimeBuildState
}

type javaRuntimeBuildState struct {
	Built bool
}

type stagedFunction struct {
	Function         template.FunctionSpec
	FunctionDir      string
	SitecustomizeRef string
}

func (ctx stageContext) verbosef(format string, args ...any) {
	if !ctx.Verbose {
		return
	}
	_, _ = fmt.Fprintf(resolveGenerateOutput(ctx.Out), format, args...)
}

// stageFunction prepares the function source, layers, and sitecustomize file
// under the output directory so downstream steps can render Dockerfiles.
func stageFunction(fn template.FunctionSpec, ctx stageContext) (stagedFunction, error) {
	if fn.Name == "" {
		return stagedFunction{}, fmt.Errorf("function name is required")
	}

	profile, err := runtime.Resolve(fn.Runtime)
	if err != nil {
		return stagedFunction{}, err
	}

	functionDir := filepath.Join(ctx.FunctionsDir, fn.Name)
	if !ctx.DryRun {
		if err := ensureDir(functionDir); err != nil {
			return stagedFunction{}, err
		}
	}

	stagingSrc := filepath.Join(functionDir, "src")
	if strings.TrimSpace(fn.CodeURI) != "" {
		sourcePath := resolveResourcePath(ctx.BaseDir, fn.CodeURI)
		if !ctx.DryRun {
			switch {
			case dirExists(sourcePath):
				if err := copyDir(sourcePath, stagingSrc); err != nil {
					return stagedFunction{}, err
				}
			case fileExists(sourcePath):
				if err := ensureDir(stagingSrc); err != nil {
					return stagedFunction{}, err
				}
				targetDir := stagingSrc
				if subDir := profile.CodeUriTargetDir(sourcePath); subDir != "" {
					targetDir = filepath.Join(stagingSrc, subDir)
					if err := ensureDir(targetDir); err != nil {
						return stagedFunction{}, err
					}
				}
				target := filepath.Join(targetDir, filepath.Base(sourcePath))
				if err := copyFile(sourcePath, target); err != nil {
					return stagedFunction{}, err
				}
			}
		}

		fn.CodeURI = ensureSlash(path.Join("functions", fn.Name, "src"))
		fn.HasRequirements = fileExists(filepath.Join(stagingSrc, "requirements.txt"))
	} else {
		fn.CodeURI = ""
		fn.HasRequirements = false
	}

	stagedLayers, err := stageLayers(fn.Layers, ctx, fn.Name, functionDir, profile)
	if err != nil {
		return stagedFunction{}, err
	}
	fn.Layers = stagedLayers

	siteRef := path.Join("functions", fn.Name, "sitecustomize.py")
	siteRef = filepath.ToSlash(siteRef)
	if !ctx.DryRun {
		if profile.UsesSitecustomize {
			siteSrc := resolveSitecustomizeSource(ctx)
			if siteSrc != "" {
				if info, err := os.Stat(siteSrc); err == nil {
					if err := linkOrCopyFile(siteSrc, filepath.Join(functionDir, "sitecustomize.py"), info.Mode()); err != nil {
						return stagedFunction{}, err
					}
				}
			}
		} else {
			sitePath := filepath.Join(functionDir, "sitecustomize.py")
			if fileExists(sitePath) {
				if err := os.Remove(sitePath); err != nil {
					return stagedFunction{}, err
				}
			}
		}
	}

	if !ctx.DryRun && profile.Kind == runtime.KindJava {
		needsBuild := ctx.JavaRuntimeBuild == nil || !ctx.JavaRuntimeBuild.Built
		if needsBuild {
			if err := buildJavaRuntimeJars(ctx); err != nil {
				return stagedFunction{}, err
			}
			if ctx.JavaRuntimeBuild != nil {
				ctx.JavaRuntimeBuild.Built = true
			}
		}

		wrapperSrc, err := ensureJavaWrapperSource(ctx)
		if err != nil {
			return stagedFunction{}, err
		}
		if _, err := os.Stat(wrapperSrc); err != nil {
			return stagedFunction{}, err
		}
		target := filepath.Join(functionDir, javaWrapperFileName)
		if err := copyFile(wrapperSrc, target); err != nil {
			return stagedFunction{}, err
		}

		agentSrc, err := ensureJavaAgentSource(ctx)
		if err != nil {
			return stagedFunction{}, err
		}
		if _, err := os.Stat(agentSrc); err != nil {
			return stagedFunction{}, err
		}
		target = filepath.Join(functionDir, javaAgentFileName)
		if err := copyFile(agentSrc, target); err != nil {
			return stagedFunction{}, err
		}
	}

	return stagedFunction{
		Function:         fn,
		FunctionDir:      functionDir,
		SitecustomizeRef: siteRef,
	}, nil
}
