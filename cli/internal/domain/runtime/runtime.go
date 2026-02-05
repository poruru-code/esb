// Where: cli/internal/domain/runtime/runtime.go
// What: Runtime profile registry for staging and rendering decisions.
// Why: Centralize runtime behavior to avoid scattered conditional logic.
package runtime

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Kind string

const (
	KindPython Kind = "python"
	KindJava   Kind = "java"
)

const defaultPythonRuntime = "python3.12"

type Profile struct {
	Name              string
	Kind              Kind
	UsesSitecustomize bool
	UsesPip           bool
	UsesJavaWrapper   bool
	NestPythonLayers  bool
	PythonVersion     string
	JavaBaseImage     string
}

func (p Profile) CodeUriTargetDir(sourcePath string) string {
	if p.Kind != KindJava {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(sourcePath), ".jar") {
		return filepath.Join("lib")
	}
	return ""
}

func Resolve(runtime string) (Profile, error) {
	normalized := strings.TrimSpace(strings.ToLower(runtime))
	if normalized == "" {
		normalized = defaultPythonRuntime
	}

	if strings.HasPrefix(normalized, "python") {
		version := strings.TrimPrefix(normalized, "python")
		if version == "" {
			version = "3.12"
		}
		return Profile{
			Name:              normalized,
			Kind:              KindPython,
			UsesSitecustomize: true,
			UsesPip:           true,
			NestPythonLayers:  true,
			PythonVersion:     version,
		}, nil
	}

	if strings.HasPrefix(normalized, "java") {
		switch normalized {
		case "java21":
			return Profile{
				Name:            normalized,
				Kind:            KindJava,
				UsesJavaWrapper: true,
				JavaBaseImage:   "public.ecr.aws/lambda/java:21",
			}, nil
		default:
			return Profile{}, fmt.Errorf("unsupported java runtime: %s", runtime)
		}
	}

	return Profile{}, fmt.Errorf("unsupported runtime: %s", runtime)
}
