// Where: cli/internal/architecture/layering_test.go
// What: Layer dependency guard tests for CLI internal packages.
// Why: Prevent architectural regressions across domain/usecase/infra/command boundaries.
package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const internalImportPrefix = "github.com/poruru/edge-serverless-box/cli/internal/"

func TestLayeringRules(t *testing.T) {
	t.Parallel()

	internalRoot := resolveInternalRoot(t)
	fset := token.NewFileSet()
	violations := []string{}

	err := filepath.WalkDir(internalRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		sourceLayer := topLayer(rel)
		if sourceLayer == "" {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, "\"")
			importLayer := topLayerFromImport(importPath)
			if importLayer == "" {
				continue
			}
			if violatesRule(sourceLayer, importLayer) {
				violations = append(violations, rel+" -> "+importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan internal packages: %v", err)
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("layering rule violations:\n%s", strings.Join(violations, "\n"))
	}
}

func resolveInternalRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	return filepath.Join(root, "internal")
}

func topLayer(relPath string) string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func topLayerFromImport(importPath string) string {
	if !strings.HasPrefix(importPath, internalImportPrefix) {
		return ""
	}
	rest := strings.TrimPrefix(importPath, internalImportPrefix)
	parts := strings.Split(rest, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func violatesRule(sourceLayer, importLayer string) bool {
	switch sourceLayer {
	case "domain":
		return importLayer == "infra" || importLayer == "usecase" || importLayer == "command"
	case "usecase":
		return importLayer == "command"
	case "infra":
		return importLayer == "command"
	default:
		return false
	}
}
